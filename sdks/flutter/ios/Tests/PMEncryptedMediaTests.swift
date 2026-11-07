// Add to an iOS13+ XCTest target that links the local plugin and its two pinned
// frameworks. This source has NOT run on Windows; no device PASS is implied.
import XCTest
import AVFoundation
import CoreVideo
import OpenIMCore
@testable import flutter_openim_sdk

@available(iOS 13.0, *)
private final class SyntheticAuthority:PMVaultAuthority {
    let actor=PMVaultActor(userID:"emp_00000000000000000000000000000000",sessionID:"unregistered-ios-offline-test",origin:"https://im.szputy.com:9885",epoch:1,generation:1)
    let directory:URL
    private let state=NSRecursiveLock()
    private var key=Data(repeating:17,count:32),active=true,callbacks=[UUID:()->Void]()
    init()throws{directory=FileManager.default.temporaryDirectory.appendingPathComponent("puty-ios-encrypted-test-"+UUID().uuidString,isDirectory:true);try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:false);guard Open_im_sdkConfigureNativeLocalStore(actor.userID,directory.path,key,Int64(Date().timeIntervalSince1970*1000)+8000),Open_im_sdkOpenConfiguredNativeWorkingStore(actor.userID) else{throw PMProtectedError.storage}}
    func currentActor()->PMVaultActor?{state.lock();defer{state.unlock()};return active ? actor:nil}
    func check(_ actor:PMVaultActor)->Bool{state.lock();defer{state.unlock()};guard active,self.actor==actor else{return false};return Open_im_sdkConfigureNativeLocalStore(actor.userID,directory.path,key,Int64(Date().timeIntervalSince1970*1000)+8000)}
    func canPreview(_ actor:PMVaultActor)->Bool{return check(actor)}
    func canSave(_ actor:PMVaultActor)->Bool{return check(actor)}
    func registerCancellation(_ actor:PMVaultActor,_ cancel:@escaping()->Void)throws->UUID{state.lock();guard check(actor) else{state.unlock();cancel();throw PMProtectedError.rejected};let id=UUID();callbacks[id]=cancel;state.unlock();return id}
    func unregisterCancellation(_ id:UUID){state.lock();callbacks.removeValue(forKey:id);state.unlock()}
    func revoke(){state.lock();active=false;let old=Array(callbacks.values);callbacks.removeAll();state.unlock();old.forEach{$0()};Open_im_sdkLockNativeLocalStore(actor.userID)}
    deinit{revoke();key.resetBytes(in:0..<key.count);try? FileManager.default.removeItem(at:directory)}
}

@available(iOS 13.0, *)
final class PMEncryptedMediaTests:XCTestCase {
    func testSessionShapeRejectsFractionalVersionsNumericBooleansAndPartialPolicy()throws {
        let user="emp_00000000000000000000000000000000",future=Int64(Date().timeIntervalSince1970*1000)+60000
        let policy:[String:Any]=["zone":"lan","logicalClient":"ios","login":true,"preview":true,"save":false,"securityVersion":1,"policyRevision":0,"networkRevision":0,"currentIP":"192.168.10.99","sources":["login":"personal","save":"role"]]
        let base:[String:Any]=["userID":user,"sessionID":"unregistered-test","platform":1,"logicalClient":"ios","epoch":1,"expiresAt":future,"loginAllowed":true,"saveAllowed":false,"effectivePolicy":policy]
        XCTAssertEqual(try PMVerifiedSessionState.parse(base,userID:user).epoch,1)
        for (key,value) in [("epoch",0 as Any),("epoch",1.5 as Any),("epoch",true as Any),("platform",2 as Any),("loginAllowed",1 as Any),("saveAllowed","false" as Any),("expiresAt",0 as Any)]{var bad=base;bad[key]=value;XCTAssertThrowsError(try PMVerifiedSessionState.parse(bad,userID:user))}
        var partial=base;partial.removeValue(forKey:"effectivePolicy");XCTAssertThrowsError(try PMVerifiedSessionState.parse(partial,userID:user))
        for (key,value) in [("securityVersion",2 as Any),("preview",1 as Any),("currentIP","192.168.10.99:9885" as Any),("logicalClient","android" as Any),("policyRevision",-1 as Any)]{var bad=base,changed=policy;changed[key]=value;bad["effectivePolicy"]=changed;XCTAssertThrowsError(try PMVerifiedSessionState.parse(bad,userID:user))}
    }
    func testScratchIsSeekableAndRevocationBlocksNewReads()throws {
        let authority=try SyntheticAuthority(),sink=try PMEncryptedScratch(authority:authority,actor:authority.actor,name:"synthetic.txt")
        let body=Data((0..<131173).map{UInt8($0%251)});try sink.append(body)
        XCTAssertEqual(try sink.read(offset:65531,count:1000),body.subdata(in:65531..<66531))
        authority.revoke();XCTAssertThrowsError(try sink.read(offset:0,count:1));XCTAssertThrowsError(try sink.stage())
    }
    func testActualVideoToolboxFragmentCanDecodeThroughEncryptedResourceLoader()throws {
        let completed=expectation(description:"actual H264 encode and decode")
        DispatchQueue.global(qos:.userInitiated).async{do{
            let authority=try SyntheticAuthority(),scratch=try PMEncryptedScratch(authority:authority,actor:authority.actor,name:"synthetic.mp4")
            guard let encoder=PMEncryptedEncoder(width:64,height:64,audio:false,bitrate:256000,emit:{data in do{try scratch.append(data);return true}catch{return false}}) else{throw PMProtectedError.storage}
            var buffer:CVPixelBuffer?;guard CVPixelBufferCreate(kCFAllocatorDefault,64,64,kCVPixelFormatType_32BGRA,[kCVPixelBufferIOSurfacePropertiesKey as String:[:]] as CFDictionary,&buffer)==kCVReturnSuccess,let pixel=buffer else{throw PMProtectedError.storage}
            CVPixelBufferLockBaseAddress(pixel,[]);if let address=CVPixelBufferGetBaseAddress(pixel){memset(address,127,CVPixelBufferGetDataSize(pixel))};CVPixelBufferUnlockBaseAddress(pixel,[])
            for index in 0..<30{XCTAssertTrue(encoder.appendVideo(pixel,time:CMTimeMake(value:Int64(index),timescale:30),duration:CMTimeMake(value:1,timescale:30)))}
            XCTAssertTrue(encoder.finish());let path=try scratch.stage(),source=try PMProtectedAsset.source(authority,authority.actor,path);defer{source.cancel()}
            try PMMediaOperations.load(source,authority,authority.actor);let reader=try AVAssetReader(asset:source.asset)
            guard let track=source.asset.tracks(withMediaType:.video).first else{throw PMProtectedError.storage};let output=AVAssetReaderTrackOutput(track:track,outputSettings:[kCVPixelBufferPixelFormatTypeKey as String:kCVPixelFormatType_32BGRA]);reader.add(output);XCTAssertTrue(reader.startReading());let sample=output.copyNextSampleBuffer();XCTAssertNotNil(sample);if let sample=sample{XCTAssertNotNil(CMSampleBufferGetImageBuffer(sample))};reader.cancelReading();authority.revoke();XCTAssertThrowsError(try source.imageBytes())
        }catch{XCTFail("Encrypted native codec test failed; no source bytes are logged")};completed.fulfill()}
        wait(for:[completed],timeout:30)
    }
}
