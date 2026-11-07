import Foundation
import AVFoundation
import UIKit
import OpenIMCore

@available(iOS 13.0, *)
enum PMMediaOperations {
    static func load(_ source:PMProtectedAsset,_ authority:PMVaultAuthority,_ actor:PMVaultActor)throws {
        guard authority.canPreview(actor) else{throw PMProtectedError.rejected};let gate=DispatchSemaphore(value:0);source.asset.loadValuesAsynchronously(forKeys:["tracks","duration"]){gate.signal()}
        guard gate.wait(timeout:.now()+20) == .success,authority.canPreview(actor) else{source.asset.cancelLoading();throw PMProtectedError.rejected}
        for key in ["tracks","duration"]{var error:NSError?;guard source.asset.statusOfValue(forKey:key,error:&error) == .loaded else{throw PMProtectedError.storage}}
    }
    static func info(_ source:PMProtectedAsset,_ authority:PMVaultAuthority,_ actor:PMVaultActor)throws->[String:Any]{try load(source,authority,actor);let duration=CMTimeGetSeconds(source.asset.duration);guard duration.isFinite,duration>=0 else{throw PMProtectedError.storage};let track=source.asset.tracks(withMediaType:.video).first;let size=track.map{CGRect(origin:.zero,size:$0.naturalSize).applying($0.preferredTransform).size} ?? .zero;return ["duration":duration,"width":Int(abs(size.width)),"height":Int(abs(size.height))]}
    static func thumbnail(_ source:PMProtectedAsset,_ authority:PMVaultAuthority,_ actor:PMVaultActor,positionMS:Int64,quality:Double)throws->String {
        try load(source,authority,actor);guard positionMS>=0,quality>=1,quality<=100 else{throw PMProtectedError.invalidInput}
        let generator=AVAssetImageGenerator(asset:source.asset);generator.appliesPreferredTrackTransform=true;generator.maximumSize=CGSize(width:1920,height:1920);let token=try authority.registerCancellation(actor,generator.cancelAllCGImageGeneration);defer{authority.unregisterCancellation(token);generator.cancelAllCGImageGeneration()}
        let time=CMTimeMake(value:positionMS,timescale:1000),image=try generator.copyCGImage(at:time,actualTime:nil);guard authority.canPreview(actor),var data=UIImage(cgImage:image).jpegData(compressionQuality:quality/100) else{throw PMProtectedError.rejected};defer{data.resetBytes(in:0..<data.count)}
        let sink=try PMEncryptedScratch(authority:authority,actor:actor,name:"snapshot-"+UUID().uuidString.lowercased()+".jpg");try sink.append(data);return try sink.stage()
    }
}

@available(iOS 13.0, *)
final class PMCompressionOperation {
    private weak var authority:PMVaultAuthority?
    private let actor:PMVaultActor,input:PMProtectedAsset,quality:String,audio:Bool,state=NSLock()
    private var closed=false,reader:AVAssetReader?,encoder:PMEncryptedEncoder?,sink:PMEncryptedScratch?
    init(authority:PMVaultAuthority,actor:PMVaultActor,input:PMProtectedAsset,quality:String,audio:Bool){self.authority=authority;self.actor=actor;self.input=input;self.quality=quality;self.audio=audio}
    private func require()throws {state.lock();let active = !closed;state.unlock();guard active,authority?.canPreview(actor)==true else{throw PMProtectedError.cancelled}}
    func cancel(){state.lock();closed=true;let reader=reader,encoder=encoder,sink=sink;state.unlock();reader?.cancelReading();encoder?.cancel();input.asset.cancelLoading();input.cancel();sink?.cancel()}
    func run()throws->String {
        guard let authority=authority else{throw PMProtectedError.rejected};try require();try PMMediaOperations.load(input,authority,actor)
        guard ["low","medium","high"].contains(quality),let track=input.asset.tracks(withMediaType:.video).first else{throw PMProtectedError.invalidInput}
        let bounds=CGRect(origin:.zero,size:track.naturalSize).applying(track.preferredTransform),longest=max(abs(bounds.width),abs(bounds.height)),limit:CGFloat=quality=="high" ? 1920:(quality=="low" ? 640:1280)
        guard longest>0,longest<=16384 else{throw PMProtectedError.invalidInput};let scale=min(1,limit/longest),width=max(2,Int(abs(bounds.width)*scale)/2*2),height=max(2,Int(abs(bounds.height)*scale)/2*2)
        let sink=try PMEncryptedScratch(authority:authority,actor:actor,name:"compressed-"+UUID().uuidString.lowercased()+".mp4"),reader=try AVAssetReader(asset:input.asset)
        let composition=AVMutableVideoComposition(),instruction=AVMutableVideoCompositionInstruction(),layer=AVMutableVideoCompositionLayerInstruction(assetTrack:track)
        let transform=track.preferredTransform.concatenating(CGAffineTransform(translationX:-bounds.minX,y:-bounds.minY)).concatenating(CGAffineTransform(scaleX:scale,y:scale));layer.setTransform(transform,at:.zero);instruction.timeRange=CMTimeRange(start:.zero,duration:input.asset.duration);instruction.layerInstructions=[layer];composition.instructions=[instruction];composition.renderSize=CGSize(width:width,height:height);composition.frameDuration=CMTimeMake(value:1,timescale:30)
        let video=AVAssetReaderVideoCompositionOutput(videoTracks:[track],videoSettings:[kCVPixelBufferPixelFormatTypeKey as String:kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange]);video.videoComposition=composition;video.alwaysCopiesSampleData=false;guard reader.canAdd(video) else{throw PMProtectedError.storage};reader.add(video)
        var audioOutput:AVAssetReaderTrackOutput?
        if audio,let audioTrack=input.asset.tracks(withMediaType:.audio).first{let output=AVAssetReaderTrackOutput(track:audioTrack,outputSettings:[AVFormatIDKey:kAudioFormatLinearPCM,AVSampleRateKey:44100,AVNumberOfChannelsKey:1,AVLinearPCMBitDepthKey:16,AVLinearPCMIsFloatKey:false,AVLinearPCMIsBigEndianKey:false,AVLinearPCMIsNonInterleaved:false]);output.alwaysCopiesSampleData=false;guard reader.canAdd(output) else{throw PMProtectedError.storage};reader.add(output);audioOutput=output}
        guard let encoder=PMEncryptedEncoder(width:Int32(width),height:Int32(height),audio:audioOutput != nil,bitrate:Int32(width*height*5),emit:{data in do{try self.require();try sink.append(data);return true}catch{return false}}) else{throw PMProtectedError.storage}
        state.lock();self.sink=sink;self.reader=reader;self.encoder=encoder;let active = !closed;state.unlock();guard active else{encoder.cancel();throw PMProtectedError.cancelled}
        var finished=false;defer{if !finished{cancel()};state.lock();self.reader=nil;self.encoder=nil;self.sink=nil;state.unlock()}
        guard reader.startReading() else{throw PMProtectedError.storage};var v=video.copyNextSampleBuffer(),a=audioOutput?.copyNextSampleBuffer()
        while v != nil||a != nil {try require();if let frame=v,(a==nil||CMTimeCompare(CMSampleBufferGetPresentationTimeStamp(frame),CMSampleBufferGetPresentationTimeStamp(a!))<=0){guard let image=CMSampleBufferGetImageBuffer(frame),encoder.appendVideo(image,time:CMSampleBufferGetPresentationTimeStamp(frame),duration:CMSampleBufferGetDuration(frame)) else{throw PMProtectedError.storage};v=video.copyNextSampleBuffer()}
            else if let sample=a{guard encoder.appendAudio(sample) else{throw PMProtectedError.storage};a=audioOutput?.copyNextSampleBuffer()}}
        try require();guard reader.status == .completed,encoder.finish() else{throw PMProtectedError.storage};let path=try sink.stage();finished=true;return path
    }
}
