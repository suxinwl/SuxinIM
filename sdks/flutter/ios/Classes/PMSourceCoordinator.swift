import Foundation
import UIKit
import AVFoundation
import AVKit
import Photos
import MobileCoreServices
import OpenIMCore
import Flutter
import ImageIO

@available(iOS 13.0, *)
final class PMSourceCoordinator:NSObject {
    typealias Completion=(Result<Any,Error>)->Void
    private weak var authority:PMVaultAuthority?
    private let state=NSRecursiveLock(),work=DispatchQueue(label:"com.szputy.im.producers",attributes:.concurrent)
    private var scratches:[String:(PMEncryptedScratch,UUID)]=[:],selections:[String:(PMVaultActor,URL,UUID)]=[:],exports:[String:PMOwnedBackupExport]=[:]
    private var operations:[String:()->Void]=[:],dialogs:[UUID:AnyObject]=[:]
    private var saveDialogs:[UUID:(PMVaultActor,()->Void)]=[:]
    private var previewRevision:UInt64=0
    init(authority:PMVaultAuthority){self.authority=authority;super.init()}
    private func require(_ actor:PMVaultActor)throws->PMVaultAuthority{guard let value=authority,value.check(actor) else{throw PMProtectedError.rejected};return value}
    private func cleanName(_ name:String)throws->String{guard !name.isEmpty,name.utf8.count<=255,!name.contains("/"),!name.contains("\\"),name != ".",name != "..",!name.unicodeScalars.contains(where:{CharacterSet.controlCharacters.contains($0)}) else{throw PMProtectedError.invalidInput};return name}
    private func scratch(_ actor:PMVaultActor,_ name:String)throws->PMEncryptedScratch{return try PMEncryptedScratch(authority:require(actor),actor:actor,name:cleanName(name))}
    private func ownedScratch(_ actor:PMVaultActor,_ path:String)throws->PMEncryptedScratch? {state.lock();defer{state.unlock()};if let value=scratches[path]{guard value.0.actor==actor else{throw PMProtectedError.rejected};return value.0};guard !path.hasPrefix("puty-scratch:") else{throw PMProtectedError.rejected};return nil}
    private func retainScratch(_ value:PMEncryptedScratch)throws->String {let authority=try require(value.actor),path="puty-scratch://"+UUID().uuidString.lowercased();let id=try authority.registerCancellation(value.actor,{[weak self,weak value] in self?.state.lock();self?.scratches.removeValue(forKey:path);self?.state.unlock();value?.cancel()});state.lock();scratches[path]=(value,id);state.unlock();guard authority.check(value.actor) else{discardScratch(path);throw PMProtectedError.rejected};return path}
    private func discardScratch(_ path:String){state.lock();let value=scratches.removeValue(forKey:path);state.unlock();if let value=value{authority?.unregisterCancellation(value.1);value.0.cancel()}}
    func sync(_ method:String,_ args:[String:Any],_ actor:PMVaultActor)throws->Any? {
        let authority=try require(actor)
        switch method {
        case "source/captureSupported":return true
        case "source/scratch":return try retainScratch(scratch(actor,(args["prefix"] as? String ?? "capture")+(args["suffix"] as? String ?? ".bin")))
        case "source/import":guard let path=args["path"] as? String else{throw PMProtectedError.invalidInput};if let value=try ownedScratch(actor,path){let result=try value.stage();discardScratch(path);return result};_ = try pmSourceInfo(actor,path);return path
        case "source/fromBytes":guard let bytes=args["bytes"] as? FlutterStandardTypedData,bytes.data.count<=32<<20,let name=args["name"] as? String else{throw PMProtectedError.invalidInput};let value=try scratch(actor,name);try value.append(bytes.data);return try value.stage()
        case "source/read":guard authority.canPreview(actor),let path=args["path"] as? String else{throw PMProtectedError.rejected};if let value=try ownedScratch(actor,path){let size=try value.size();guard size<=32<<20 else{throw PMProtectedError.invalidInput};var data=Data();var offset:Int64=0;while offset<size{let part=try value.read(offset:offset,count:Int(min(1<<20,size-offset)));guard !part.isEmpty else{throw PMProtectedError.storage};data.append(part);offset+=Int64(part.count)};return FlutterStandardTypedData(bytes:data)};guard let data=Open_im_sdkReadNativeSource(actor.userID,path,32<<20) as Data?,authority.canPreview(actor) else{throw PMProtectedError.rejected};return FlutterStandardTypedData(bytes:data)
        case "source/info":guard let path=args["path"] as? String else{throw PMProtectedError.invalidInput};if let value=try ownedScratch(actor,path){return ["name":value.name,"size":try value.size()]};return try pmSourceInfo(actor,path)
        case "source/discard":guard let path=args["path"] as? String else{throw PMProtectedError.invalidInput};if try ownedScratch(actor,path) != nil{discardScratch(path);return true};return Open_im_sdkDiscardNativeSource(actor.userID,path)
        case "source/previewUri":guard authority.canPreview(actor),let path=args["path"] as? String else{throw PMProtectedError.rejected};if try ownedScratch(actor,path)==nil{_ = try pmSourceInfo(actor,path)};return "puty-vault://source/"+Data(path.utf8).base64EncodedString().replacingOccurrences(of:"+",with:"-").replacingOccurrences(of:"/",with:"_").replacingOccurrences(of:"=",with:"")
        default:return nil
        }
    }
    func retainExport(_ path:String,_ actor:PMVaultActor,directory:URL)throws {let value=try PMOwnedBackupExport(path:path,actor:actor,directory:directory);do{let authority=try require(actor);guard authority.canSave(actor) else{throw PMProtectedError.rejected};state.lock();guard exports.count<32,exports[path]==nil else{state.unlock();throw PMProtectedError.rejected};exports[path]=value;state.unlock();value.registration=try authority.registerCancellation(actor,{[weak self] in self?.releaseExport(path)});guard authority.canSave(actor) else{throw PMProtectedError.rejected}}catch{releaseExport(path);value.removeIfUnchanged();throw error}}
    func releaseExport(_ path:String){state.lock();let value=exports.removeValue(forKey:path);state.unlock();if let value=value{if let token=value.registration{authority?.unregisterCancellation(token)};value.removeIfUnchanged()}}
    func invalidateExports(_ actor:PMVaultActor){state.lock();let cancel=saveDialogs.values.filter{$0.0==actor}.map{$0.1};state.unlock();cancel.forEach{$0()}}
    private func saveExport(_ path:String,_ folder:URL,_ actor:PMVaultActor)throws->PMBackupSavedCopy {
        let authority=try require(actor);guard authority.canSave(actor),folder.isFileURL else{throw PMProtectedError.rejected}
        state.lock();let owner=exports[path];state.unlock();guard let owner=owner,owner.actor==actor,owner.unchanged() else{throw PMProtectedError.rejected};defer{releaseExport(path)}
        let scoped=folder.startAccessingSecurityScopedResource();defer{if scoped{folder.stopAccessingSecurityScopedResource()}}
        var issue:NSError?,outcome:Result<PMBackupSavedCopy,Error>?
        NSFileCoordinator().coordinate(writingItemAt:folder,options:.forMerging,error:&issue){directory in do{outcome = .success(try owner.copy(to:directory,check:{authority.canSave(actor)}))}catch{outcome = .failure(error)}}
        if issue != nil{if case .success(let saved)?=outcome{saved.removeIfUnchanged()};throw PMProtectedError.storage}
        guard let outcome=outcome else{throw PMProtectedError.storage};let saved=try outcome.get();guard authority.canSave(actor) else{saved.removeIfUnchanged();throw PMProtectedError.rejected};return saved
    }
    func withBackupInput<T>(_ path:String,_ actor:PMVaultActor,_ action:(URL)throws->T)throws->T {
        _ = try require(actor);state.lock();let selected=selections.removeValue(forKey:path),export=exports[path];state.unlock()
        if let selected=selected {defer{authority?.unregisterCancellation(selected.2)};guard selected.0==actor else{throw PMProtectedError.rejected};let scoped=selected.1.startAccessingSecurityScopedResource();defer{if scoped{selected.1.stopAccessingSecurityScopedResource()}};var issue:NSError?,result:Result<T,Error>?
            NSFileCoordinator().coordinate(readingItemAt:selected.1,options:.withoutChanges,error:&issue){url in do{guard self.authority?.check(actor)==true else{throw PMProtectedError.rejected};result = .success(try action(url))}catch{result = .failure(error)}}
            guard issue==nil,let result=result else{throw PMProtectedError.storage};return try result.get()
        }
        guard export?.actor==actor else{throw PMProtectedError.rejected};defer{releaseExport(path)};return try action(URL(fileURLWithPath:path))
    }
    private func presenter()throws->UIViewController {guard Thread.isMainThread else{throw PMProtectedError.invalidInput};let scene=UIApplication.shared.connectedScenes.compactMap{$0 as? UIWindowScene}.first{$0.activationState == .foregroundActive};guard var controller=scene?.windows.first(where:{$0.isKeyWindow})?.rootViewController else{throw PMProtectedError.rejected};while let next=controller.presentedViewController{controller=next};guard !(controller is UIAlertController) else{throw PMProtectedError.rejected};return controller}
    private func retainDialog(_ dialog:AnyObject,_ id:UUID){state.lock();dialogs[id]=dialog;state.unlock()}
    private func releaseDialog(_ id:UUID){state.lock();dialogs.removeValue(forKey:id);state.unlock()}
    func invalidatePreviews(){state.lock();previewRevision &+= 1;state.unlock()}
    func handle(_ method:String,_ args:[String:Any],_ actor:PMVaultActor,authorize:@escaping(PMVaultActor,String,String)throws->[String:Any],complete:@escaping Completion)->Bool {
        if method=="media/cancel"{state.lock();let cancel=operations[args["operationID"] as? String ?? ""];state.unlock();cancel?();complete(.success(true));return true}
        switch method {
        case "picker/files","backup/pick","backup/save":DispatchQueue.main.async{self.documents(method,args,actor,complete)}
        case "picker/assets":DispatchQueue.main.async{self.photos(args,actor,complete)}
        case "media/capture":DispatchQueue.main.async{self.capture(args,actor,complete)}
        case "media/info","media/thumbnail","media/compress":work.async{self.process(method,args,actor,complete)}
        case "media/preview":state.lock();let revision=previewRevision,allowed=authority?.canPreview(actor)==true;state.unlock();guard allowed else{complete(.failure(PMProtectedError.rejected));return true};work.async{self.preview(args,actor,revision,authorize,complete)}
        default:return false
        };return true
    }
    private func documents(_ method:String,_ args:[String:Any],_ actor:PMVaultActor,_ complete:@escaping Completion){do {
        let authority=try require(actor),controller=try presenter();state.lock();let busy = !dialogs.isEmpty;state.unlock();guard !busy else{throw PMProtectedError.rejected}
        let dialog:UIDocumentPickerViewController
        if method=="backup/save"{guard authority.canSave(actor),let path=args["path"] as? String else{throw PMProtectedError.rejected};state.lock();let owner=exports[path];state.unlock();guard owner?.actor==actor,owner?.unchanged()==true else{throw PMProtectedError.rejected};dialog=UIDocumentPickerViewController(documentTypes:[kUTTypeFolder as String],in:.open);dialog.allowsMultipleSelection=false}
        else{dialog=UIDocumentPickerViewController(documentTypes:[kUTTypeData as String],in:.open);dialog.allowsMultipleSelection=method=="picker/files" && args["multiple"] as? Bool==true}
        let id=UUID(),delegate=PMDocumentSelection()
        let registration=try authority.registerCancellation(actor,{[weak dialog,weak delegate] in DispatchQueue.main.async{dialog?.dismiss(animated:false);delegate?.finish(nil)}})
        retainDialog(delegate,id)
        if method=="backup/save"{state.lock();saveDialogs[id]=(actor,{[weak dialog,weak delegate] in DispatchQueue.main.async{dialog?.dismiss(animated:false);delegate?.finish(nil)}});state.unlock()}
        delegate.completion={urls in self.releaseDialog(id);self.state.lock();self.saveDialogs.removeValue(forKey:id);self.state.unlock();authority.unregisterCancellation(registration);guard authority.check(actor),(method != "backup/save" || authority.canSave(actor)) else{if method=="backup/save",let path=args["path"] as? String{self.releaseExport(path)};complete(.failure(PMProtectedError.rejected));return};guard let urls=urls,!urls.isEmpty else{if method=="backup/save",let path=args["path"] as? String{self.releaseExport(path);complete(.success(false))}else{complete(.success(NSNull()))};return}
            if method=="backup/save"{guard urls.count==1,let path=args["path"] as? String else{complete(.failure(PMProtectedError.invalidInput));return};self.work.async{do{complete(.success(try self.saveExport(path,urls[0],actor)))}catch{complete(.failure(error))}};return}
            if method=="backup/pick"{guard urls.count==1 else{complete(.failure(PMProtectedError.invalidInput));return};let ticket="puty-backup:"+UUID().uuidString.lowercased();do{let token=try authority.registerCancellation(actor,{[weak self] in self?.state.lock();self?.selections.removeValue(forKey:ticket);self?.state.unlock()});self.state.lock();guard self.selections.count<16 else{self.state.unlock();authority.unregisterCancellation(token);throw PMProtectedError.rejected};self.selections[ticket]=(actor,urls[0],token);self.state.unlock();guard authority.check(actor) else{self.state.lock();self.selections.removeValue(forKey:ticket);self.state.unlock();authority.unregisterCancellation(token);throw PMProtectedError.rejected};complete(.success(ticket))}catch{complete(.failure(error))};return}
            self.work.async{do{guard urls.count<=20 else{throw PMProtectedError.invalidInput};var items=[[String:Any]]();for url in urls{items.append(try self.importDocument(url,actor))};complete(.success(items))}catch{complete(.failure(error))}}
        };dialog.delegate=delegate;if method=="backup/save",!authority.canSave(actor){delegate.finish(nil);return};controller.present(dialog,animated:true)
    }catch{complete(.failure(error))}}
    private func importDocument(_ url:URL,_ actor:PMVaultActor)throws->[String:Any]{guard url.isFileURL else{throw PMProtectedError.invalidInput};let authority=try require(actor),scoped=url.startAccessingSecurityScopedResource();defer{if scoped{url.stopAccessingSecurityScopedResource()}}
        var issue:NSError?,outcome:Result<[String:Any],Error>?
        NSFileCoordinator().coordinate(readingItemAt:url,options:.withoutChanges,error:&issue){readURL in do{let metadata=try readURL.resourceValues(forKeys:[.isRegularFileKey,.isSymbolicLinkKey,.fileSizeKey]);guard metadata.isRegularFile==true,metadata.isSymbolicLink != true,(metadata.fileSize ?? -1)>=0,(metadata.fileSize ?? Int.max)<=2<<30,let input=InputStream(url:readURL) else{throw PMProtectedError.invalidInput};let sink=try self.scratch(actor,readURL.lastPathComponent);let token=try authority.registerCancellation(actor,{sink.cancel();input.close()});defer{authority.unregisterCancellation(token);input.close()};input.open();var buffer=[UInt8](repeating:0,count:65536);defer{buffer.withUnsafeMutableBytes{$0.initializeMemory(as:UInt8.self,repeating:0)}};while true{guard authority.check(actor) else{throw PMProtectedError.rejected};let count=input.read(&buffer,maxLength:buffer.count);if count==0{break};guard count>0 else{throw PMProtectedError.storage};try sink.append(Data(buffer[0..<count]))};guard try sink.size()==Int64(metadata.fileSize!) else{throw PMProtectedError.storage};let path=try sink.stage();outcome = .success(try self.item(path,actor))}catch{outcome = .failure(error)}}
        guard issue==nil,let outcome=outcome else{throw PMProtectedError.storage};return try outcome.get()
    }
    private func item(_ path:String,_ actor:PMVaultActor,includeMedia:Bool=false)throws->[String:Any]{let data=try pmSourceInfo(actor,path),name=data["name"] as? String ?? "file",ext=URL(fileURLWithPath:name).pathExtension.lowercased();let kind=["png","jpg","jpeg","heic","gif","webp"].contains(ext) ? "image":(["mp4","mov","m4v"].contains(ext) ? "video":"file");let uti=UTTypeCreatePreferredIdentifierForTag(kUTTagClassFilenameExtension,ext as CFString,nil)?.takeRetainedValue();let mime=uti.flatMap{UTTypeCopyPreferredTagWithClass($0,kUTTagClassMIMEType)?.takeRetainedValue() as String?} ?? "application/octet-stream";var result:[String:Any]=["path":path,"name":name,"size":data["size"] ?? 0,"kind":kind,"mimeType":mime];if includeMedia&&kind=="video"{let authority=try require(actor),input=try PMProtectedAsset.source(authority,actor,path);defer{input.cancel()};result.merge(try PMMediaOperations.info(input,authority,actor),uniquingKeysWith:{$1})};return result}
    private func photos(_ args:[String:Any],_ actor:PMVaultActor,_ complete:@escaping Completion){let status=PHPhotoLibrary.authorizationStatus();if status == .notDetermined{PHPhotoLibrary.requestAuthorization{_ in DispatchQueue.main.async{self.photos(args,actor,complete)}};return};do{let authority=try require(actor);var allowed=status == .authorized;if #available(iOS 14.0,*){allowed=allowed||status == .limited};guard allowed else{throw PMProtectedError.rejected};let picker=PMPhotoSelection(images:args["images"] as? Bool ?? true,videos:args["videos"] as? Bool ?? true,multiple:args["multiple"] as? Bool ?? false);let navigation=UINavigationController(rootViewController:picker),id=UUID();retainDialog(picker,id);let token=try authority.registerCancellation(actor,{[weak picker] in DispatchQueue.main.async{picker?.finish(nil)}});picker.completion={assets in self.releaseDialog(id);authority.unregisterCancellation(token);guard let assets=assets else{complete(.success(NSNull()));return};self.work.async{do{var result=[[String:Any]]();for asset in assets{result.append(try self.importAsset(asset,actor))};complete(.success(result))}catch{complete(.failure(error))}}};try presenter().present(navigation,animated:true)}catch{complete(.failure(error))}}
    private func importAsset(_ asset:PHAsset,_ actor:PMVaultActor)throws->[String:Any]{let authority=try require(actor),resources=PHAssetResource.assetResources(for:asset);let type:PHAssetResourceType=asset.mediaType == .video ? .video:.photo;guard let resource=resources.first(where:{$0.type==type}) ?? resources.first(where:{$0.type == .fullSizeVideo||$0.type == .fullSizePhoto}) else{throw PMProtectedError.storage};let sink=try scratch(actor,resource.originalFilename),gate=DispatchSemaphore(value:0),guardLock=NSLock();var failure:Error?,requestID:PHAssetResourceDataRequestID=0,done=false;let manager=PHAssetResourceManager.default(),options=PHAssetResourceRequestOptions();options.isNetworkAccessAllowed=true
        requestID=manager.requestData(for:resource,options:options,dataReceivedHandler:{data in guardLock.lock();defer{guardLock.unlock()};guard !done,failure==nil else{return};do{guard authority.check(actor) else{throw PMProtectedError.rejected};try sink.append(data)}catch{failure=error;manager.cancelDataRequest(requestID)}},completionHandler:{error in guardLock.lock();if failure==nil{failure=error};done=true;guardLock.unlock();gate.signal()})
        let token=try authority.registerCancellation(actor,{manager.cancelDataRequest(requestID);sink.cancel();gate.signal()});defer{authority.unregisterCancellation(token)}
        guard gate.wait(timeout:.now()+300) == .success else{manager.cancelDataRequest(requestID);throw PMProtectedError.network};guardLock.lock();let issue=failure,finished=done;guardLock.unlock();if let issue=issue{throw issue};guard finished,authority.check(actor) else{throw PMProtectedError.rejected};let path=try sink.stage();return try item(asset.mediaType == .image ? normalizePhoto(path,actor):path,actor,includeMedia:true)
    }
    private func normalizePhoto(_ path:String,_ actor:PMVaultActor)throws->String{let info=try pmSourceInfo(actor,path);guard let name=info["name"] as? String else{throw PMProtectedError.storage};let ext=URL(fileURLWithPath:name).pathExtension.lowercased();if ["jpg","jpeg","png","gif"].contains(ext){return path};let authority=try require(actor);guard let size=info["size"] as? NSNumber,size.int64Value<=32<<20,var bytes=Open_im_sdkReadNativeSource(actor.userID,path,32<<20) as Data? else{throw PMProtectedError.invalidInput};defer{bytes.resetBytes(in:0..<bytes.count)}
        guard authority.check(actor),let source=CGImageSourceCreateWithData(bytes as CFData,[kCGImageSourceShouldCache:false] as CFDictionary),let image=CGImageSourceCreateThumbnailAtIndex(source,0,[kCGImageSourceCreateThumbnailFromImageAlways:true,kCGImageSourceCreateThumbnailWithTransform:true,kCGImageSourceThumbnailMaxPixelSize:4096,kCGImageSourceShouldCacheImmediately:true] as CFDictionary),var jpeg=UIImage(cgImage:image).jpegData(compressionQuality:0.9) else{throw PMProtectedError.storage};defer{jpeg.resetBytes(in:0..<jpeg.count)};let output=try scratch(actor,URL(fileURLWithPath:name).deletingPathExtension().lastPathComponent+".jpg");try output.append(jpeg);let result=try output.stage();guard authority.check(actor) else{throw PMProtectedError.rejected};guard Open_im_sdkDiscardNativeSource(actor.userID,path) else{throw PMProtectedError.storage};return result
    }
    private func capture(_ args:[String:Any],_ actor:PMVaultActor,_ complete:@escaping Completion){do{let authority=try require(actor),capture=PMCaptureController(authority:authority,actor:actor,kind:args["kind"] as? String ?? "both",maxDuration:min(300,max(1,(args["maxDuration"] as? NSNumber)?.doubleValue ?? 60))),id=UUID();retainDialog(capture,id);let token=try authority.registerCancellation(actor,{[weak capture] in capture?.cancel()});capture.completion={response in self.releaseDialog(id);authority.unregisterCancellation(token);complete(response)};try presenter().present(capture,animated:true)}catch{complete(.failure(error))}}
    private func process(_ method:String,_ args:[String:Any],_ actor:PMVaultActor,_ complete:@escaping Completion){do{let authority=try require(actor);guard let path=args["path"] as? String else{throw PMProtectedError.invalidInput};let input=try PMProtectedAsset.source(authority,actor,path);defer{input.cancel()}
        if method=="media/info"{complete(.success(try PMMediaOperations.info(input,authority,actor)));return}
        if method=="media/thumbnail"{let result=try PMMediaOperations.thumbnail(input,authority,actor,positionMS:(args["positionMS"] as? NSNumber)?.int64Value ?? 0,quality:(args["quality"] as? NSNumber)?.doubleValue ?? 80);complete(.success(result));return}
        guard let operationID=args["operationID"] as? String,!operationID.isEmpty,operationID.count<=128 else{throw PMProtectedError.invalidInput};let operation=PMCompressionOperation(authority:authority,actor:actor,input:input,quality:args["quality"] as? String ?? "medium",audio:args["includeAudio"] as? Bool ?? true);state.lock();guard operations[operationID]==nil else{state.unlock();throw PMProtectedError.rejected};operations[operationID]=operation.cancel;state.unlock();defer{state.lock();operations.removeValue(forKey:operationID);state.unlock()};let token=try authority.registerCancellation(actor,operation.cancel);defer{authority.unregisterCancellation(token)};let result=try operation.run();var item=try self.item(result,actor);let output=try PMProtectedAsset.source(authority,actor,result);defer{output.cancel()};item.merge(try PMMediaOperations.info(output,authority,actor),uniquingKeysWith:{$1});item["snapshotPath"]=try PMMediaOperations.thumbnail(output,authority,actor,positionMS:0,quality:80);complete(.success(item))
    }catch{complete(.failure(error))}}
    private func preview(_ args:[String:Any],_ actor:PMVaultActor,_ revision:UInt64,_ authorize:@escaping(PMVaultActor,String,String)throws->[String:Any],_ complete:@escaping Completion){do{let authority=try require(actor);guard authority.canPreview(actor),[args["path"],args["taskID"],args["url"]].compactMap({$0 as? String}).filter({!$0.isEmpty}).count==1 else{throw PMProtectedError.invalidInput};let asset:PMProtectedAsset;var remote:PMRemoteAsset?
        if let path=args["path"] as? String{asset=try PMProtectedAsset.source(authority,actor,path)}else if let task=args["taskID"] as? String{asset=try PMProtectedAsset.attachment(authority,actor,task)}else{let stream=try PMRemoteAsset(authority:authority,actor:actor,source:args["url"] as! String,authorize:authorize);remote=stream;let extensionByType=["image/jpeg":"jpg","image/png":"png","image/gif":"gif","image/heic":"heic","audio/wav":"wav","audio/mp4":"m4a","video/quicktime":"mov"];asset=try PMProtectedAsset(authority:authority,actor:actor,name:args["name"] as? String ?? "preview."+(extensionByType[stream.contentType] ?? "mp4"),size:stream.size,reader:stream.read)}
        let image:[String:Bool]=["jpg":true,"jpeg":true,"png":true,"gif":true,"heic":true,"webp":true];let isImage=image[URL(fileURLWithPath:asset.name).pathExtension.lowercased()]==true;var bitmap:UIImage?
        if isImage{var bytes=try asset.imageBytes();defer{bytes.resetBytes(in:0..<bytes.count)};guard let decoded=UIImage(data:bytes) else{throw PMProtectedError.storage};bitmap=decoded}
        DispatchQueue.main.async{self.state.lock();defer{self.state.unlock()};do{guard revision==self.previewRevision,asset.isActive(),authority.canPreview(actor) else{throw PMProtectedError.rejected};let id=UUID()
            if let bitmap=bitmap{let controller=PMImagePreviewController(image:bitmap,asset:asset);self.retainDialog(controller,id);let token=try authority.registerCancellation(actor,{[weak controller] in DispatchQueue.main.async{controller?.close()}});controller.completion={self.releaseDialog(id);authority.unregisterCancellation(token);complete(.success(true))};try self.presenter().present(controller,animated:true)}
            else{let controller=PMPreviewController(asset:asset,remote:remote);self.retainDialog(controller,id);let token=try authority.registerCancellation(actor,{[weak controller] in DispatchQueue.main.async{controller?.close()}});controller.completion={self.releaseDialog(id);authority.unregisterCancellation(token);complete(.success(true))};try self.presenter().present(controller,animated:true){controller.player?.play()}}
        }catch{asset.cancel();complete(.failure(error))}}
    }catch{complete(.failure(error))}}
}

@available(iOS 13.0, *)
private final class PMDocumentSelection:NSObject,UIDocumentPickerDelegate {var completion:(([URL]?)->Void)?;func finish(_ urls:[URL]?){let callback=completion;completion=nil;callback?(urls)};func documentPickerWasCancelled(_ controller:UIDocumentPickerViewController){finish(nil)};func documentPicker(_ controller:UIDocumentPickerViewController,didPickDocumentsAt urls:[URL]){finish(urls)}}

@available(iOS 13.0, *)
private final class PMPhotoSelection:UITableViewController {
    var completion:(([PHAsset]?)->Void)?;private let assets:PHFetchResult<PHAsset>,multiple:Bool;private var selected=[String:PHAsset]();private let manager=PHCachingImageManager()
    init(images:Bool,videos:Bool,multiple:Bool){self.multiple=multiple;let options=PHFetchOptions();options.sortDescriptors=[NSSortDescriptor(key:"creationDate",ascending:false)];options.predicate=NSPredicate(format:"mediaType IN %@",[images ? PHAssetMediaType.image.rawValue:-1,videos ? PHAssetMediaType.video.rawValue:-1]);assets=PHAsset.fetchAssets(with:options);super.init(style:.plain)}
    required init?(coder:NSCoder){fatalError("init(coder:) is unavailable")}
    override func viewDidLoad(){super.viewDidLoad();title="选择照片或视频";navigationItem.leftBarButtonItem=UIBarButtonItem(barButtonSystemItem:.cancel,target:self,action:#selector(cancel));navigationItem.rightBarButtonItem=UIBarButtonItem(barButtonSystemItem:.done,target:self,action:#selector(done));tableView.rowHeight=72}
    @objc private func cancel(){finish(nil)};@objc private func done(){finish(Array(selected.values))}
    func finish(_ value:[PHAsset]?){let callback=completion;completion=nil;manager.stopCachingImagesForAllAssets();dismiss(animated:true){callback?(value)}}
    override func tableView(_ tableView:UITableView,numberOfRowsInSection section:Int)->Int{return assets.count}
    override func tableView(_ tableView:UITableView,cellForRowAt indexPath:IndexPath)->UITableViewCell{let asset=assets.object(at:indexPath.row),cell=UITableViewCell(style:.subtitle,reuseIdentifier:nil);cell.textLabel?.text=asset.mediaType == .video ? "视频":"照片";cell.detailTextLabel?.text=asset.creationDate.map{DateFormatter.localizedString(from:$0,dateStyle:.medium,timeStyle:.short)} ?? "";cell.accessoryType=selected[asset.localIdentifier]==nil ? .none:.checkmark;let options=PHImageRequestOptions();options.deliveryMode = .fastFormat;manager.requestImage(for:asset,targetSize:CGSize(width:96,height:96),contentMode:.aspectFill,options:options){[weak cell] image,_ in cell?.imageView?.image=image;cell?.setNeedsLayout()};return cell}
    override func tableView(_ tableView:UITableView,didSelectRowAt indexPath:IndexPath){let asset=assets.object(at:indexPath.row);if !multiple{finish([asset]);return};if selected.removeValue(forKey:asset.localIdentifier)==nil&&selected.count<20{selected[asset.localIdentifier]=asset};tableView.reloadRows(at:[indexPath],with:.none)}
}

@available(iOS 13.0, *)
private final class PMPreviewController:AVPlayerViewController {
    var completion:(()->Void)?;private let protected:PMProtectedAsset;private var remote:PMRemoteAsset?,closing=false
    init(asset:PMProtectedAsset,remote:PMRemoteAsset?){self.protected=asset;self.remote=remote;super.init(nibName:nil,bundle:nil);player=AVPlayer(playerItem:AVPlayerItem(asset:asset.asset));player?.allowsExternalPlayback=false;allowsPictureInPicturePlayback=false}
    required init?(coder:NSCoder){fatalError("init(coder:) is unavailable")}
    func close(){guard !closing else{return};closing=true;player?.pause();player?.replaceCurrentItem(with:nil);protected.cancel();remote=nil;let callback=completion;completion=nil;dismiss(animated:false){callback?()}}
    override func viewDidDisappear(_ animated:Bool){super.viewDidDisappear(animated);if isBeingDismissed||presentingViewController==nil{close()}}
    deinit{protected.cancel()}
}

@available(iOS 13.0, *)
private final class PMImagePreviewController:UIViewController {
    var completion:(()->Void)?;private let imageView=UIImageView(),protected:PMProtectedAsset;private var closing=false
    init(image:UIImage,asset:PMProtectedAsset){self.protected=asset;super.init(nibName:nil,bundle:nil);imageView.image=image;modalPresentationStyle = .fullScreen}
    required init?(coder:NSCoder){fatalError("init(coder:) is unavailable")}
    override func viewDidLoad(){super.viewDidLoad();view.backgroundColor = .black;imageView.contentMode = .scaleAspectFit;imageView.frame=view.bounds;imageView.autoresizingMask=[.flexibleWidth,.flexibleHeight];view.addSubview(imageView);let button=UIButton(type:.system);button.setTitle("关闭",for:.normal);button.addTarget(self,action:#selector(close),for:.touchUpInside);button.translatesAutoresizingMaskIntoConstraints=false;view.addSubview(button);NSLayoutConstraint.activate([button.topAnchor.constraint(equalTo:view.safeAreaLayoutGuide.topAnchor,constant:12),button.trailingAnchor.constraint(equalTo:view.safeAreaLayoutGuide.trailingAnchor,constant:-16)])}
    @objc func close(){guard !closing else{return};closing=true;imageView.image=nil;protected.cancel();let callback=completion;completion=nil;dismiss(animated:false){callback?()}}
    override func viewDidDisappear(_ animated:Bool){super.viewDidDisappear(animated);if isBeingDismissed||presentingViewController==nil{close()}}
    deinit{protected.cancel()}
}
