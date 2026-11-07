import Foundation
import AVFoundation
import MobileCoreServices
import OpenIMCore

/// AVFoundation consumes only authenticated byte ranges; no decrypted file URL
/// or external-open capability is issued. The caller retains this object for the
/// lifetime of its AVAsset/AVPlayer and cancels the decoder on vault lock.
@available(iOS 13.0, *)
final class PMProtectedAsset:NSObject,AVAssetResourceLoaderDelegate {
    let asset:AVURLAsset,name:String
    private weak var authority:PMVaultAuthority?
    private let actor:PMVaultActor,size:Int64,type:String,reader:(Int64,Int)throws->Data
    private let queue=DispatchQueue(label:"com.szputy.im.asset-ranges"),state=NSLock()
    private var closed=false,registration:UUID?,requests:[ObjectIdentifier:AVAssetResourceLoadingRequest]=[:]
    init(authority:PMVaultAuthority,actor:PMVaultActor,name:String,size:Int64,reader:@escaping(Int64,Int)throws->Data)throws {
        guard authority.canPreview(actor),size>0,size<=Int64(2<<30) else{throw PMProtectedError.rejected}
        let ext=URL(fileURLWithPath:name).pathExtension.lowercased()
        self.type=(UTTypeCreatePreferredIdentifierForTag(kUTTagClassFilenameExtension,ext as CFString,nil)?.takeRetainedValue() as String?) ?? "public.data"
        self.authority=authority;self.actor=actor;self.size=size;self.reader=reader;self.name=name
        let suffix=["mp4","mov","m4a","aac","wav","mp3"].contains(ext) ? ext:"mp4"
        self.asset=AVURLAsset(url:URL(string:"puty-vault://media/"+UUID().uuidString+"."+suffix)!,options:[AVURLAssetPreferPreciseDurationAndTimingKey:true]);super.init()
        asset.resourceLoader.setDelegate(self,queue:queue)
        registration=try authority.registerCancellation(actor,{[weak self] in self?.cancel()})
    }
    deinit{cancel()}
    private func authorized()->Bool{state.lock();let value = !closed;state.unlock();return value&&authority?.canPreview(actor)==true}
    func isActive()->Bool{return authorized()}
    func imageBytes()throws->Data{guard authorized(),size<=32<<20 else{throw PMProtectedError.rejected};var data=Data();var offset:Int64=0;do{while offset<size{guard authorized() else{throw PMProtectedError.rejected};let count=Int(min(1<<20,size-offset));var chunk=try reader(offset,count);defer{chunk.resetBytes(in:0..<chunk.count)};guard chunk.count==count else{throw PMProtectedError.storage};data.append(chunk);offset+=Int64(count)};guard authorized() else{throw PMProtectedError.rejected};return data}catch{data.resetBytes(in:0..<data.count);throw error}}
    func cancel(){state.lock();if closed{state.unlock();return};closed=true;let registered=registration;registration=nil;state.unlock();if let registered=registered{authority?.unregisterCancellation(registered)};queue.async{[weak self] in guard let self=self else{return};for request in self.requests.values{request.finishLoading(with:PMProtectedError.cancelled)};self.requests.removeAll();self.asset.resourceLoader.setDelegate(nil,queue:nil)}}
    func resourceLoader(_ resourceLoader:AVAssetResourceLoader,shouldWaitForLoadingOfRequestedResource request:AVAssetResourceLoadingRequest)->Bool {
        let id=ObjectIdentifier(request);requests[id]=request
        defer{requests.removeValue(forKey:id)}
        do{guard authorized() else{throw PMProtectedError.rejected}
            if let info=request.contentInformationRequest{info.contentType=type;info.contentLength=size;info.isByteRangeAccessSupported=true}
            if let data=request.dataRequest {
                var offset=max(data.currentOffset,data.requestedOffset)
                guard offset>=0,data.requestedOffset>=0,data.requestedLength>=0,Int64(data.requestedLength)<=Int64.max-data.requestedOffset else{throw PMProtectedError.invalidInput}
                let end=data.requestsAllDataToEndOfResource ? size:min(size,data.requestedOffset+Int64(data.requestedLength))
                while offset<end{guard authorized(),!request.isCancelled else{throw PMProtectedError.cancelled};let count=Int(min(Int64(1<<20),end-offset));var bytes=try reader(offset,count);defer{bytes.resetBytes(in:0..<bytes.count)};guard bytes.count==count,authorized() else{throw PMProtectedError.rejected};data.respond(with:bytes);offset+=Int64(count)}
            }
            guard authorized(),!request.isCancelled else{throw PMProtectedError.cancelled};request.finishLoading()
        }catch{request.finishLoading(with:error)}
        return true
    }
    func resourceLoader(_ resourceLoader:AVAssetResourceLoader,didCancel loadingRequest:AVAssetResourceLoadingRequest){requests.removeValue(forKey:ObjectIdentifier(loadingRequest))}
    static func source(_ authority:PMVaultAuthority,_ actor:PMVaultActor,_ path:String)throws->PMProtectedAsset {
        let info=try pmSourceInfo(actor,path);guard let name=info["name"] as? String,let size=info["size"] as? NSNumber else{throw PMProtectedError.rejected}
        return try PMProtectedAsset(authority:authority,actor:actor,name:name,size:size.int64Value,reader:{offset,count in guard authority.check(actor),var bytes=Open_im_sdkReadNativeSourceRange(actor.userID,path,offset,count) as Data? else{throw PMProtectedError.rejected};guard authority.check(actor) else{bytes.resetBytes(in:0..<bytes.count);throw PMProtectedError.rejected};return bytes})
    }
    static func attachment(_ authority:PMVaultAuthority,_ actor:PMVaultActor,_ task:String)throws->PMProtectedAsset {
        guard let text=Open_im_sdkNativeAttachmentInfo(actor.userID,task) as String?,let bytes=text.data(using:.utf8),let info=try JSONSerialization.jsonObject(with:bytes) as? [String:Any],let name=info["name"] as? String,let size=info["size"] as? NSNumber else{throw PMProtectedError.rejected}
        return try PMProtectedAsset(authority:authority,actor:actor,name:name,size:size.int64Value,reader:{offset,count in guard authority.check(actor),var bytes=Open_im_sdkReadNativeAttachmentRange(actor.userID,task,offset,count) as Data? else{throw PMProtectedError.rejected};guard authority.check(actor) else{bytes.resetBytes(in:0..<bytes.count);throw PMProtectedError.rejected};return bytes})
    }
}
