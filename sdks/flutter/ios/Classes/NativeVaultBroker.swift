import Foundation
import Flutter
import Security
import CryptoKit
import OpenIMCore
import Network
import UIKit

/// Source integration only until the pinned SQLCipher core XCFramework is built
/// on macOS. No remote/plaintext OpenIMCore fallback is permitted by the podspec.
@available(iOS 13.0, *)
final class NativeVaultBroker: NSObject, URLSessionDelegate, URLSessionTaskDelegate, PMVaultAuthority {
    private let queue = DispatchQueue(label: "com.szputy.im.local-vault")
    private let authQueue = DispatchQueue(label:"com.szputy.im.local-vault-auth",attributes:.concurrent)
    private let stateLock=NSLock()
    // Serializes native key/DB transitions while callbacks may briefly release
    // stateLock to cancel consumers. A late old lock cannot close a new store.
    private let nativeTransition=NSRecursiveLock()
    private var generation:UInt64=0
    private var authorizationSerial:UInt64=0
    private var credentials:[String:Any]?
    private var sessionID:String?
    private var epoch:Int64=0
    private var leaseDeadline:TimeInterval=0
    private var refreshing=false,authorizing=false
    private var closed=false
    private var timer:DispatchSourceTimer?
    private weak var channel:FlutterMethodChannel?
    private var activeAccount: String?
    private var previewAllowed=false,saveAllowed=false,clearing=false
    private var cancellations:[UUID:(PMVaultActor,()->Void)]=[:]
    private var restores:[String:(PMVaultActor,Open_im_sdkNativeRestore)]=[:]
    private lazy var sources=PMSourceCoordinator(authority:self)
    override init(){super.init();let timer=DispatchSource.makeTimerSource(queue:authQueue);timer.schedule(deadline:.now()+3,repeating:3);timer.setEventHandler{[weak self] in self?.renew()};self.timer=timer;timer.resume();NotificationCenter.default.addObserver(self,selector:#selector(background),name:UIApplication.didEnterBackgroundNotification,object:nil)}
    @objc private func background(){stateLock.lock();let expected=generation;stateLock.unlock();lock(expected,reason:"BACKGROUND_LOCK")}
    deinit{NotificationCenter.default.removeObserver(self);timer?.cancel();stateLock.lock();let pending=cancellations.values.map{$0.1},restoring=restores.values.map{$0.1},account=activeAccount;closed=true;cancellations.removeAll();restores.removeAll();stateLock.unlock();pending.forEach{$0()};restoring.forEach{$0.abort()};if let account=account{Open_im_sdkLockNativeLocalStore(account)}}
    func setChannel(_ channel:FlutterMethodChannel){self.channel=channel}
    private func isCurrent(_ expected:UInt64,requireLease:Bool=true)->Bool{stateLock.lock();defer{stateLock.unlock()};return !closed&&generation==expected&&(!requireLease||(activeAccount != nil&&ProcessInfo.processInfo.systemUptime<leaseDeadline))}
    private func lock(_ expected:UInt64,authorization expectedAuthorization:UInt64?=nil,notify:Bool=true,reason:String="ONLINE_AUTHORIZATION_REQUIRED"){nativeTransition.lock();defer{nativeTransition.unlock()};stateLock.lock();guard generation==expected else{stateLock.unlock();return};if let expectedAuthorization=expectedAuthorization,authorizationSerial != expectedAuthorization{stateLock.unlock();return};generation &+= 1;let lockedGeneration=generation;authorizationSerial &+= 1;authorizing=false;let account=activeAccount;activeAccount=nil;credentials=nil;sessionID=nil;epoch=0;leaseDeadline=0;previewAllowed=false;saveAllowed=false;clearing=false;let cancel=cancellations.values.map{$0.1},pending=restores.values.map{$0.1};cancellations.removeAll();restores.removeAll();stateLock.unlock();cancel.forEach{$0()};pending.forEach{$0.abort()};if let account=account{Open_im_sdkLockNativeLocalStore(account)};if notify{DispatchQueue.main.async{[weak self] in guard let self=self else{return};self.stateLock.lock();defer{self.stateLock.unlock()};if self.generation==lockedGeneration&&self.activeAccount==nil{self.channel?.invokeMethod("onVaultLocked",arguments:reason)}}}}
    func currentActor()->PMVaultActor?{stateLock.lock();defer{stateLock.unlock()};guard !closed,ProcessInfo.processInfo.systemUptime<leaseDeadline,let user=activeAccount,let sid=sessionID,let origin=credentials?["origin"] as? String else{return nil};return PMVaultActor(userID:user,sessionID:sid,origin:origin,epoch:epoch,generation:generation)}
    func check(_ actor:PMVaultActor)->Bool{return currentActor()==actor}
    func canPreview(_ actor:PMVaultActor)->Bool{guard check(actor) else{return false};stateLock.lock();defer{stateLock.unlock()};return previewAllowed && !clearing && generation==actor.generation}
    func canSave(_ actor:PMVaultActor)->Bool{stateLock.lock();defer{stateLock.unlock()};return !closed && !clearing && saveAllowed && activeAccount==actor.userID && sessionID==actor.sessionID && epoch==actor.epoch && (credentials?["origin"] as? String)==actor.origin && generation==actor.generation && ProcessInfo.processInfo.systemUptime<leaseDeadline}
    func registerCancellation(_ actor:PMVaultActor,_ cancel:@escaping()->Void)throws->UUID{guard check(actor) else{cancel();throw PMProtectedError.rejected};stateLock.lock();guard generation==actor.generation,!closed,!clearing,cancellations.count<128 else{stateLock.unlock();cancel();throw PMProtectedError.rejected};let id=UUID();cancellations[id]=(actor,cancel);stateLock.unlock();return id}
    func unregisterCancellation(_ id:UUID){stateLock.lock();cancellations.removeValue(forKey:id);stateLock.unlock()}
    // Clearing preserves the online actor, but cancels every reader/producer and
    // prepared restore before removing payload. It must not NativeLock the SDK.
    private func performClear(_ actor:PMVaultActor,_ operation:()->Bool)throws->Bool{
        nativeTransition.lock();defer{nativeTransition.unlock()}
        guard check(actor) else{throw VaultError.rejected}
        stateLock.lock();guard !clearing,generation==actor.generation else{stateLock.unlock();throw VaultError.rejected};clearing=true;stateLock.unlock()
        defer{stateLock.lock();clearing=false;stateLock.unlock()}
        // Invalidate queued main-thread previews before snapshotting callbacks.
        // Presentation holds the same coordinator lock, so it is either refused
        // or its close callback is included in this cancellation snapshot.
        sources.invalidatePreviews()
        stateLock.lock();let cancel=cancellations.values.map{$0.1},pending=restores.values.map{$0.1};cancellations.removeAll();restores.removeAll();stateLock.unlock()
        cancel.forEach{$0()};pending.forEach{$0.abort()}
        guard check(actor),operation(),check(actor) else{throw VaultError.rejected};return true
    }
    private func clearScopes(_ args:[String:Any])throws->(String,Int64,Int64){
        guard let values=args["conversationIDs"] as? [String],!values.isEmpty,values.count<=100,
          values.allSatisfy({!$0.isEmpty&&$0.utf8.count<=512 && !$0.unicodeScalars.contains(where:{CharacterSet.controlCharacters.contains($0)})}),
          let from=pmInteger(args["from"]),let to=pmInteger(args["toExclusive"]),from<to,
          let encoded=String(data:try JSONSerialization.data(withJSONObject:Array(Set(values)).sorted()),encoding:.utf8) else{throw VaultError.rejected}
        return(encoded,from,to)
    }
    private func renew(){stateLock.lock();guard !closed,!refreshing,!authorizing,let args=credentials else{stateLock.unlock();return};refreshing=true;let expected=generation,serial=authorizationSerial;stateLock.unlock();unlock(args,requested:expected,authorization:serial,renewal:true){[weak self] response in guard let self=self else{return};self.stateLock.lock();self.refreshing=false;self.stateLock.unlock();if case .failure=response{self.lock(expected,authorization:serial)}}}
    private let origins = ["https://192.168.10.11:9885", "https://im.szputy.com:9885"]
    private var root: URL {
        get throws {
            let base = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
            let value = base.appendingPathComponent("PUTY-Local", isDirectory: true)
            try FileManager.default.createDirectory(at: value, withIntermediateDirectories: true, attributes: [.protectionKey: FileProtectionType.complete])
            var mutable = value
            var attributes = URLResourceValues(); attributes.isExcludedFromBackup = true
            try mutable.setResourceValues(attributes)
            return value
        }
    }
    private func key(_ identity: String) throws -> Data {
        let query: [String: Any] = [kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:"com.szputy.im.local-vault.v1",kSecAttrAccount as String:identity]
        var read = query; read[kSecReturnData as String] = true
        var existing: CFTypeRef?
        let status = SecItemCopyMatching(read as CFDictionary, &existing)
        if status == errSecSuccess, let data = existing as? Data, data.count == 32 { return data }
        guard status == errSecItemNotFound else { throw VaultError.rejected }
        var data = Data(count:32)
        guard data.withUnsafeMutableBytes({SecRandomCopyBytes(kSecRandomDefault,32,$0.baseAddress!)}) == errSecSuccess else {throw VaultError.rejected}
        var add = query; add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        guard SecItemAdd(add as CFDictionary,nil) == errSecSuccess else {throw VaultError.rejected}
        return data
    }
    private func accountDirectory(_ user:String)throws->URL{let name=SHA256.hash(data:Data(user.utf8)).map{String(format:"%02x",$0)}.joined();return try root.appendingPathComponent("accounts/"+name,isDirectory:true)}
    private func wrapMaster(_ data:Data,_ user:String)throws->Data{let sealed=try AES.GCM.seal(data,using:SymmetricKey(data:try key("account-master-wrapper")),authenticating:Data(("master\u{0}"+user).utf8));guard let combined=sealed.combined else{throw VaultError.rejected};return Data("PUTYOS1\n".utf8)+combined}
    private func accountMaster(_ user:String,_ directory:URL)throws->Data{let file=directory.appendingPathComponent("master.wrap");if FileManager.default.fileExists(atPath:file.path){let envelope=try Data(contentsOf:file);guard envelope.count<4096,envelope.starts(with:Data("PUTYOS1\n".utf8)) else{throw VaultError.rejected};let box=try AES.GCM.SealedBox(combined:envelope.dropFirst(8));let result=try AES.GCM.open(box,using:SymmetricKey(data:try key("account-master-wrapper")),authenticating:Data(("master\u{0}"+user).utf8));guard result.count==32 else{throw VaultError.rejected};return result};let master=try key("account:"+user);try wrapMaster(master,user).write(to:file,options:[.atomic,.completeFileProtection]);return master}
    private func settings(read: Bool, value: String? = nil) throws -> Any {
        let file = try root.appendingPathComponent("settings.wrap")
        let secret = SymmetricKey(data:try key("device-settings"))
        if read {
            guard FileManager.default.fileExists(atPath:file.path) else {return "{}"}
            let envelope = try Data(contentsOf:file)
            guard envelope.starts(with:Data("PUTYOS1\n".utf8)), envelope.count <= 2<<20 else {throw VaultError.rejected}
            let box = try AES.GCM.SealedBox(combined:envelope.dropFirst(8))
            let plain = try AES.GCM.open(box,using:secret,authenticating:Data("settings".utf8))
            guard let result = String(data:plain,encoding:.utf8) else {throw VaultError.rejected};return result
        }
        guard let value = value, let bytes = value.data(using:.utf8), bytes.count <= 1<<20, (try JSONSerialization.jsonObject(with:bytes)) is [String:Any] else {throw VaultError.rejected}
        let sealed = try AES.GCM.seal(bytes,using:secret,authenticating:Data("settings".utf8))
        guard let combined = sealed.combined else {throw VaultError.rejected}
        try (Data("PUTYOS1\n".utf8)+combined).write(to:file,options:[.atomic,.completeFileProtection])
        return true
    }
    enum VaultError: Error {case rejected}
    private func unlock(_ args: [String:Any], requested:UInt64,authorization:UInt64,renewal:Bool=false,completion: @escaping (Result<Any,Error>)->Void) {
        guard let origin=args["origin"] as? String, origins.contains(origin), let user=args["userID"] as? String,
              user.range(of:"^emp_[0-9a-f]{32}$",options:.regularExpression) != nil,
              let token=args["imToken"] as? String, !token.isEmpty, let chat=args["chatToken"] as? String, !chat.isEmpty,
              let url=URL(string:origin+"/enterprise/v1/session/state") else {completion(.failure(VaultError.rejected));return}
        var request=URLRequest(url:url,cachePolicy:.reloadIgnoringLocalCacheData,timeoutInterval:2.5)
        request.setValue(token,forHTTPHeaderField:"token");request.setValue(chat,forHTTPHeaderField:"chatToken");request.setValue("1",forHTTPHeaderField:"platformID")
        let config=URLSessionConfiguration.ephemeral;config.urlCache=nil;config.httpCookieStorage=nil;config.httpShouldSetCookies=false;config.timeoutIntervalForRequest=2.5;config.timeoutIntervalForResource=2.5
        let session=URLSession(configuration:config,delegate:self,delegateQueue:nil)
        var deadline:DispatchWorkItem?
        let task=session.dataTask(with:request){data,response,error in
            deadline?.cancel()
            defer{session.finishTasksAndInvalidate()}
            guard error==nil, let http=response as? HTTPURLResponse,http.statusCode==200, let data=data,data.count<=65536 else {completion(.failure(VaultError.rejected));return}
            self.authQueue.async{
                do{
                    guard let envelope=try JSONSerialization.jsonObject(with:data) as? [String:Any],
                          pmInteger(envelope["errCode"] ?? envelope["code"])==0,let state=envelope["data"] as? [String:Any] else {throw VaultError.rejected}
                    let verified=try PMVerifiedSessionState.parse(state,userID:user)
                    let now=Date().timeIntervalSince1970*1000,lease=min(verified.expiresAt,now+8000)
                    guard lease>now else {throw VaultError.rejected}
                    let sid=verified.sessionID,currentEpoch=verified.epoch;guard self.isCurrent(requested,requireLease:false) else{throw VaultError.rejected}
                    let directory=try self.accountDirectory(user)
                    self.nativeTransition.lock();defer{self.nativeTransition.unlock()}
                    self.stateLock.lock();let authorized = !self.closed&&self.generation==requested&&self.authorizationSerial==authorization;self.stateLock.unlock();guard authorized else{throw VaultError.rejected}
                    guard Open_im_sdkRecoverNativeStoreRoot(directory.path) else{throw VaultError.rejected}
                    try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:true,attributes:[.protectionKey:FileProtectionType.complete])
                    var master=try self.accountMaster(user,directory);defer{master.resetBytes(in:0..<master.count)}
                    let home=URL(fileURLWithPath:NSHomeDirectory(),isDirectory:true).resolvingSymlinksInPath().path
                    // Old SDK databases/Hive/media were in Documents and old
                    // media pickers used Caches/tmp. Never scan Library as a
                    // whole: Preferences/Keychain/system runtime files are live.
                    let legacy=[FileManager.default.urls(for:.documentDirectory,in:.userDomainMask).first,FileManager.default.urls(for:.cachesDirectory,in:.userDomainMask).first,URL(fileURLWithPath:NSTemporaryDirectory(),isDirectory:true)].compactMap{$0}
                    guard legacy.allSatisfy({$0.resolvingSymlinksInPath().path.hasPrefix(home+"/")}),let encoded=String(data:try JSONSerialization.data(withJSONObject:legacy.map{$0.path}),encoding:.utf8) else{throw VaultError.rejected}
                    let quarantine=try self.root.appendingPathComponent("legacy-quarantine",isDirectory:true);try FileManager.default.createDirectory(at:quarantine,withIntermediateDirectories:true,attributes:[.protectionKey:FileProtectionType.complete]);var quarantineKey=try self.key("legacy-quarantine-master");defer{quarantineKey.resetBytes(in:0..<quarantineKey.count)}
                    self.stateLock.lock()
                    guard !self.closed,self.generation==requested,self.authorizationSerial==authorization else{self.stateLock.unlock();throw VaultError.rejected}
                    let changed=self.activeAccount != nil&&(self.activeAccount != user||self.sessionID != sid||self.epoch != currentEpoch||(self.credentials?["origin"] as? String) != origin)
                    if renewal&&changed{self.stateLock.unlock();throw VaultError.rejected}
                    if changed{let previous=self.activeAccount;self.generation &+= 1;let current=self.generation;self.leaseDeadline=0;self.previewAllowed=false;self.saveAllowed=false;self.clearing=false;self.sessionID=nil;self.credentials=nil;let cancelled=self.cancellations.values.map{$0.1},pending=self.restores.values.map{$0.1};self.cancellations.removeAll();self.restores.removeAll();self.stateLock.unlock();cancelled.forEach{$0()};pending.forEach{$0.abort()};if let previous=previous,previous != user{Open_im_sdkLockNativeLocalStore(previous)};self.stateLock.lock();guard !self.closed,self.generation==current,self.authorizationSerial==authorization else{self.stateLock.unlock();throw VaultError.rejected}}
                    guard Open_im_sdkConfigureNativeLocalStore(user,directory.path,master,Int64(lease)) else {self.stateLock.unlock();throw VaultError.rejected}
                    guard Open_im_sdkSetNativeLegacyRoots(user,encoded),Open_im_sdkConfigureNativeLegacyQuarantine(user,quarantine.path,quarantineKey) else{self.stateLock.unlock();Open_im_sdkLockNativeLocalStore(user);throw VaultError.rejected}
                    self.activeAccount=user;self.credentials=args;self.sessionID=sid;self.epoch=currentEpoch;self.leaseDeadline=ProcessInfo.processInfo.systemUptime+max(0,(lease-Date().timeIntervalSince1970*1000)/1000);self.previewAllowed=true;self.saveAllowed=verified.saveAllowed
                    let actorGeneration=self.generation;self.stateLock.unlock();if !verified.saveAllowed{self.sources.invalidateExports(PMVaultActor(userID:user,sessionID:sid,origin:origin,epoch:currentEpoch,generation:actorGeneration))};completion(.success(["userID":user,"sessionID":sid,"epoch":currentEpoch,"logicalClient":"ios","actorGeneration":NSNumber(value:actorGeneration),"directory":directory.path,"authorizedUntil":lease,"saveAllowed":verified.saveAllowed]))
                }catch{completion(.failure(VaultError.rejected))}
            }
        }
        let cancellation=DispatchWorkItem{task.cancel()};deadline=cancellation;authQueue.asyncAfter(deadline:.now()+2.5,execute:cancellation);task.resume()
    }
    private func requestJSON(_ actor:PMVaultActor,_ path:String,_ body:[String:Any])throws->[String:Any]{
        guard check(actor),path=="/enterprise/v1/attachment/authorize",let url=URL(string:actor.origin+path) else{throw VaultError.rejected}
        stateLock.lock();let args=credentials;stateLock.unlock();guard let im=args?["imToken"] as? String,let chat=args?["chatToken"] as? String else{throw VaultError.rejected}
        var request=URLRequest(url:url,cachePolicy:.reloadIgnoringLocalCacheData,timeoutInterval:8);request.httpMethod="POST";request.httpBody=try JSONSerialization.data(withJSONObject:body);request.setValue("application/json",forHTTPHeaderField:"Content-Type");request.setValue(im,forHTTPHeaderField:"token");request.setValue(chat,forHTTPHeaderField:"chatToken");request.setValue("1",forHTTPHeaderField:"platformID");request.setValue("no-store",forHTTPHeaderField:"Cache-Control")
        var result=Data();let transport=PMProtectedHTTP(timeout:8,maximum:65536,check:{[weak self] in self?.check(actor)==true},onResponse:{guard $0.statusCode==200 else{throw PMProtectedError.rejected}},onData:{result.append($0)});let id=try registerCancellation(actor,transport.cancel);defer{unregisterCancellation(id)};_ = try transport.run(request)
        guard check(actor),let envelope=try JSONSerialization.jsonObject(with:result) as? [String:Any],pmInteger(envelope["errCode"] ?? envelope["code"])==0,let value=envelope["data"] as? [String:Any] else{throw VaultError.rejected};return value
    }
    private func saveAttachment(_ actor:PMVaultActor,_ args:[String:Any])throws->[String:Any]{
        guard let source=args["url"] as? String,let name=args["sourceName"] as? String,!name.isEmpty else{throw VaultError.rejected}
        let grant=try requestJSON(actor,"/enterprise/v1/attachment/authorize",["url":source,"purpose":"save"])
        guard let address=grant["url"] as? String,let proof=grant["resourceProof"] as? String,proof.range(of:"^[0-9a-f]{64}$",options:.regularExpression) != nil,let object=grant["objectID"] as? String,!object.isEmpty,let url=URL(string:address),let parts=URLComponents(url:url,resolvingAgainstBaseURL:false),parts.user==nil,parts.password==nil,parts.fragment==nil,address.hasPrefix(actor.origin+"/attachments/"),url.scheme=="https",url.host==URL(string:actor.origin)?.host,url.port==URL(string:actor.origin)?.port else{throw VaultError.rejected}
        let task=SHA256.hash(data:Data((object+"\u{0}"+name).utf8)).map{String(format:"%02x",$0)}.joined(),conv=args["conversationID"] as? String ?? "",message=args["messageID"] as? String ?? ""
        guard var path=Open_im_sdkBeginNativeAttachment(actor.userID,task,object,name,conv,message) as String?,!path.isEmpty else{throw VaultError.rejected}
        if !Open_im_sdkNativeAttachmentComplete(actor.userID,task){
            var offset=Open_im_sdkNativeAttachmentOffset(actor.userID,task),tag=Open_im_sdkNativeAttachmentETag(actor.userID,task) as String? ?? ""
            guard offset>=0 else{throw VaultError.rejected}
            if offset>0&&tag.isEmpty{guard Open_im_sdkResetNativeAttachment(actor.userID,task),let reset=Open_im_sdkBeginNativeAttachment(actor.userID,task,object,name,conv,message) as String?,!reset.isEmpty else{throw VaultError.rejected};path=reset;offset=0}
            var complete=false;defer{if !complete{Open_im_sdkAbortNativeAttachment(actor.userID,task)}}
            for attempt in 0..<2 {
                guard check(actor) else{throw VaultError.rejected};var request=URLRequest(url:url,cachePolicy:.reloadIgnoringLocalCacheData,timeoutInterval:120);request.setValue(proof,forHTTPHeaderField:"X-Puty-Resource-Proof");request.setValue("no-store",forHTTPHeaderField:"Cache-Control");if offset>0{request.setValue("bytes=\(offset)-",forHTTPHeaderField:"Range");request.setValue(tag,forHTTPHeaderField:"If-Range")}
                var expected=(args["size"] as? NSNumber)?.int64Value ?? -1,actual=offset,restart=false
                let transport=PMProtectedHTTP(timeout:120,maximum:Int64(2<<30)-offset,check:{[weak self] in self?.check(actor)==true},onResponse:{http in
                    if offset>0&&http.statusCode==200&&attempt==0{restart=true;throw PMProtectedError.cancelled}
                    if offset>0{let text=http.value(forHTTPHeaderField:"Content-Range") ?? "";let tokens=text.replacingOccurrences(of:"bytes ",with:"").split(whereSeparator:{$0=="-"||$0=="/"});guard http.statusCode==206,text.hasPrefix("bytes "),tokens.count==3,Int64(tokens[0])==offset,let last=Int64(tokens[1]),last>=offset,let total=Int64(tokens[2]),last<total,http.value(forHTTPHeaderField:"ETag")==tag,expected<0||expected==total else{throw PMProtectedError.rejected};expected=total}
                    else{guard http.statusCode==200 else{throw PMProtectedError.rejected};if http.expectedContentLength>=0{guard expected<0||expected==http.expectedContentLength else{throw PMProtectedError.rejected};expected=http.expectedContentLength}}
                    if let validator=http.value(forHTTPHeaderField:"ETag"),validator.hasPrefix("\""),validator.hasSuffix("\""),validator.count<=512{guard Open_im_sdkSetNativeAttachmentETag(actor.userID,task,validator) else{throw PMProtectedError.storage}}
                },onData:{data in var cursor=0;while cursor<data.count{guard self.check(actor) else{throw PMProtectedError.rejected};let count=min(65536,data.count-cursor);var chunk=data.subdata(in:cursor..<cursor+count);defer{chunk.resetBytes(in:0..<chunk.count)};guard Open_im_sdkWriteNativeAttachment(actor.userID,task,chunk) else{throw PMProtectedError.storage};actual+=Int64(count);cursor+=count}})
                let cancellation=try registerCancellation(actor,transport.cancel)
                do{_ = try transport.run(request)}catch{unregisterCancellation(cancellation);if !restart{throw error}}
                unregisterCancellation(cancellation)
                if restart{guard Open_im_sdkResetNativeAttachment(actor.userID,task),let reset=Open_im_sdkBeginNativeAttachment(actor.userID,task,object,name,conv,message) as String?,!reset.isEmpty else{throw VaultError.rejected};path=reset;offset=0;tag="";continue}
                guard check(actor),Open_im_sdkFinishNativeAttachment(actor.userID,task,expected<0 ? actual:expected) else{throw VaultError.rejected};complete=true;break
            }
            guard complete else{throw VaultError.rejected}
        }
        guard check(actor) else{throw VaultError.rejected};return ["taskID":task,"name":name,"path":path,"encrypted":true]
    }
    func urlSession(_ session:URLSession,task:URLSessionTask,willPerformHTTPRedirection response:HTTPURLResponse,newRequest request:URLRequest,completionHandler:@escaping(URLRequest?)->Void){completionHandler(nil)}
    func urlSession(_ session:URLSession,didReceive challenge:URLAuthenticationChallenge,completionHandler:@escaping(URLSession.AuthChallengeDisposition,URLCredential?)->Void){
        guard challenge.protectionSpace.host=="192.168.10.11" else {completionHandler(.performDefaultHandling,nil);return}
        guard challenge.protectionSpace.port==9885,challenge.protectionSpace.authenticationMethod==NSURLAuthenticationMethodServerTrust,
              let trust=challenge.protectionSpace.serverTrust,
              let bundleURL=Bundle(for:NativeVaultBroker.self).url(forResource:"flutter_openim_sdk_privacy",withExtension:"bundle"),let bundle=Bundle(url:bundleURL),
              let path=bundle.url(forResource:"puty_enterprise_root_ca",withExtension:"der"),let der=try? Data(contentsOf:path),
              SHA256.hash(data:der).map({String(format:"%02x",$0)}).joined()=="836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69",
              let root=SecCertificateCreateWithData(nil,der as CFData) else {completionHandler(.cancelAuthenticationChallenge,nil);return}
        SecTrustSetAnchorCertificates(trust,[root] as CFArray);SecTrustSetAnchorCertificatesOnly(trust,true)
        SecTrustSetPolicies(trust,SecPolicyCreateSSL(true,"192.168.10.11" as CFString))
        guard SecTrustEvaluateWithError(trust,nil),SecTrustGetCertificateCount(trust)==2,let final=SecTrustGetCertificateAtIndex(trust,1),SecCertificateCopyData(final) as Data==der else {completionHandler(.cancelAuthenticationChallenge,nil);return}
        completionHandler(.useCredential,URLCredential(trust:trust))
    }
    func handle(_ call:FlutterMethodCall,result:@escaping FlutterResult){
        stateLock.lock();if call.method=="unlock"{authorizationSerial &+= 1;authorizing=true};let requested=generation,authorization=authorizationSerial;stateLock.unlock()
        if call.method=="lock"{lock(requested,notify:false);result(true);return}
        let complete:(Result<Any,Error>)->Void={response in DispatchQueue.main.async{
            let discardBackupResult={if case .success(let data)=response{if let saved=data as? PMBackupSavedCopy{saved.removeIfUnchanged()}else if call.method=="backup/export",let path=data as? String{self.sources.releaseExport(path)}}}
            if !call.method.hasPrefix("preferences/"),!self.isCurrent(requested){discardBackupResult();self.lock(requested,authorization:authorization);result(FlutterError(code:"LOCAL_VAULT_REJECTED",message:"GenerationOrLeaseChanged",details:nil));return}
            if call.method=="backup/export" || call.method=="backup/save"{guard let actor=self.currentActor(),self.canSave(actor) else{discardBackupResult();result(FlutterError(code:"LOCAL_VAULT_REJECTED",message:"Protected operation rejected",details:nil));return}}
            switch response{case .success(let data):if data is PMBackupSavedCopy{result(true)}else{result(data)};case .failure:result(FlutterError(code:"LOCAL_VAULT_REJECTED",message:"Protected operation rejected",details:nil))}
        }}
        let args=call.arguments as? [String:Any] ?? [:]
        if let actor=currentActor(),sources.handle(call.method,args,actor,authorize:{[weak self] actor,url,purpose in guard let self=self else{throw PMProtectedError.rejected};return try self.requestJSON(actor,"/enterprise/v1/attachment/authorize",["url":url,"purpose":purpose])},complete:complete){return}
        queue.async{
            do{
                if call.method != "unlock",!call.method.hasPrefix("preferences/"){guard self.isCurrent(requested) else{throw VaultError.rejected}}
                let actor=self.currentActor()
                if let actor=actor,let value=try self.sources.sync(call.method,args,actor){complete(.success(value));return}
                switch call.method {
                case "unlock":guard let args=call.arguments as? [String:Any] else {throw VaultError.rejected};self.unlock(args,requested:requested,authorization:authorization){response in DispatchQueue.main.async{self.stateLock.lock();let current=self.authorizationSerial==authorization;let latest=self.generation;if current{self.authorizing=false};self.stateLock.unlock();guard current else{result(FlutterError(code:"LOCAL_VAULT_REJECTED",message:"AuthorizationChanged",details:nil));return};switch response{case .success(let value):guard self.currentActor() != nil else{self.lock(latest,authorization:authorization);result(FlutterError(code:"LOCAL_VAULT_REJECTED",message:"AuthorizationExpired",details:nil));return};result(value);case .failure:self.lock(latest,authorization:authorization);result(FlutterError(code:"LOCAL_VAULT_REJECTED",message:"AuthorizationRejected",details:nil))}}}
                case "preferences/read":complete(.success(try self.settings(read:true)))
                case "preferences/write":complete(.success(try self.settings(read:false,value:call.arguments as? String)))
                case "archive/page":guard let actor=actor,let conv=args["conversationID"] as? String,let before=args["before"] as? NSNumber,let limit=args["limit"] as? NSNumber,limit.intValue>=1,limit.intValue<=1000,let page=Open_im_sdkGetNativeLocalArchivePage(actor.userID,conv,before.int64Value,args["beforeID"] as? String ?? "",limit.intValue) as String?,!page.isEmpty else{throw VaultError.rejected};complete(.success(page))
                case "archive/previewClear":guard let actor=actor,let conv=args["conversationID"] as? String,!conv.isEmpty,conv.utf8.count<=512,let from=pmInteger(args["from"]),let to=pmInteger(args["to"]),from<=to,let text=Open_im_sdkPreviewNativeClearRange(actor.userID,conv,from,to) as String?,!text.isEmpty,self.check(actor) else{throw VaultError.rejected};complete(.success(text))
                case "archive/clear":guard let actor=actor,let conv=args["conversationID"] as? String,!conv.isEmpty,conv.utf8.count<=512,let from=pmInteger(args["from"]),let to=pmInteger(args["to"]),from<=to else {throw VaultError.rejected};complete(.success(try self.performClear(actor){Open_im_sdkClearNativeLocalRange(actor.userID,conv,from,to)}))
                case "archive/previewClearScopes":guard let actor=actor else{throw VaultError.rejected};let (ids,from,to)=try self.clearScopes(args);guard let text=Open_im_sdkPreviewNativeClearScopes(actor.userID,ids,from,to) as String?,!text.isEmpty,self.check(actor) else{throw VaultError.rejected};complete(.success(text))
                case "archive/clearScopes":guard let actor=actor else{throw VaultError.rejected};let (ids,from,to)=try self.clearScopes(args);complete(.success(try self.performClear(actor){Open_im_sdkClearNativeLocalScopes(actor.userID,ids,from,to)}))
                case "legacy/finalize":guard let actor=actor,self.check(actor),Open_im_sdkFinalizeNativeLegacyFiles(actor.userID),self.check(actor) else{throw VaultError.rejected};complete(.success(true))
                case "attachment/import":guard let actor=actor,self.canSave(actor),let path=args["path"] as? String,let conv=args["conversationID"] as? String,!conv.isEmpty,conv.utf8.count<=512 else{throw VaultError.rejected};_ = try pmSourceInfo(actor,path);guard let text=Open_im_sdkImportNativeLocalAttachment(actor.userID,path,conv) as String?,!text.isEmpty,self.canSave(actor) else{throw VaultError.rejected};complete(.success(text))
                case "attachment/save":guard let actor=actor else{throw VaultError.rejected};complete(.success(try self.saveAttachment(actor,args)))
                case "attachment/list":guard let actor=actor,let limit=args["limit"] as? NSNumber,limit.intValue>=1,limit.intValue<=100,let text=Open_im_sdkListNativeAttachments(actor.userID,args["conversationID"] as? String ?? "",args["cursor"] as? String ?? "",limit.intValue) as String?,!text.isEmpty else{throw VaultError.rejected};complete(.success(text))
                case "attachment/read":guard let actor=actor,self.canPreview(actor),let task=args["taskID"] as? String,let bytes=Open_im_sdkReadNativeAttachment(actor.userID,task,32<<20) as Data?,self.canPreview(actor) else{throw VaultError.rejected};complete(.success(FlutterStandardTypedData(bytes:bytes)))
                case "attachment/previewUri":guard let actor=actor,self.canPreview(actor),let task=args["taskID"] as? String,let text=Open_im_sdkNativeAttachmentInfo(actor.userID,task) as String?,!text.isEmpty else{throw VaultError.rejected};complete(.success("puty-vault://attachment/"+task))
                case "backup/export":guard let actor=actor,self.canSave(actor),let passphrase=args["passphrase"] as? String,!passphrase.isEmpty,passphrase.utf8.count<=1024,let path=Open_im_sdkExportNativeBackup(actor.userID,passphrase) as String?,!path.isEmpty else{throw VaultError.rejected};try self.sources.retainExport(path,actor,directory:self.accountDirectory(actor.userID));guard self.canSave(actor) else{self.sources.releaseExport(path);throw VaultError.rejected};complete(.success(path))
                case "backup/prepare":guard let actor=actor,let path=args["path"] as? String,let passphrase=args["passphrase"] as? String,!passphrase.isEmpty,passphrase.utf8.count<=1024 else{throw VaultError.rejected};let restored=try self.sources.withBackupInput(path,actor){url -> Open_im_sdkNativeRestore in guard let value=Open_im_sdkPrepareNativeRestore(actor.userID,url.path,passphrase) else{throw VaultError.rejected};return value};guard self.check(actor) else{restored.abort();throw VaultError.rejected};let id=UUID().uuidString.lowercased();self.stateLock.lock();guard self.generation==actor.generation else{self.stateLock.unlock();restored.abort();throw VaultError.rejected};self.restores[id]=(actor,restored);self.stateLock.unlock();complete(.success(id))
                case "backup/cancel":guard let actor=actor,let id=args["restoreID"] as? String else{throw VaultError.rejected};self.stateLock.lock();let value=self.restores.removeValue(forKey:id);self.stateLock.unlock();if let value=value{guard value.0==actor else{throw VaultError.rejected};value.1.abort()};complete(.success(true))
                case "backup/commit":guard let actor=actor,let id=args["restoreID"] as? String else{throw VaultError.rejected};self.stateLock.lock();let value=self.restores.removeValue(forKey:id);self.stateLock.unlock();guard let value=value,value.0==actor,self.check(actor),var master=value.1.master(),master.count==32 else{throw VaultError.rejected};defer{master.resetBytes(in:0..<master.count);value.1.abort()};let envelope=try self.wrapMaster(master,actor.userID)
                    self.stateLock.lock();guard self.generation==actor.generation,!self.clearing else{self.stateLock.unlock();throw VaultError.rejected};self.clearing=true;self.stateLock.unlock();self.sources.invalidatePreviews()
                    self.stateLock.lock();guard self.generation==actor.generation else{self.stateLock.unlock();throw VaultError.rejected};let preparingCancel=self.cancellations.values.map{$0.1},otherRestores=self.restores.values.map{$0.1};self.cancellations.removeAll();self.restores.removeAll();self.stateLock.unlock();preparingCancel.forEach{$0()};otherRestores.forEach{$0.abort()}
                    // Hashing/merging a large restore stays outside the native
                    // transition. Credentials stay live for same-actor renewal;
                    // the consumer fence above prevents new plaintext readers.
                    guard value.1.prepareCommit(envelope),self.check(actor) else{self.lock(actor.generation);throw VaultError.rejected}
                    self.nativeTransition.lock();defer{self.nativeTransition.unlock()}
                    // Native commit writes master.wrap + promotion receipt before
                    // swapping same-parent ciphertext roots. No Keychain mutation
                    // can strand a promoted database with the previous master.
                    self.stateLock.lock();guard self.generation==actor.generation,ProcessInfo.processInfo.systemUptime<self.leaseDeadline else{self.stateLock.unlock();throw VaultError.rejected};self.generation &+= 1;let committedGeneration=self.generation;self.credentials=nil;self.sessionID=nil;self.leaseDeadline=0;self.previewAllowed=false;self.saveAllowed=false;let cancel=self.cancellations.values.map{$0.1};self.cancellations.removeAll();self.stateLock.unlock();cancel.forEach{$0()};guard value.1.commit(envelope) else{self.lock(committedGeneration);throw VaultError.rejected};DispatchQueue.main.async{result(true);self.lock(committedGeneration)}
                default:DispatchQueue.main.async{result(FlutterMethodNotImplemented)}
                }
            }catch{if call.method=="unlock"{self.lock(requested,authorization:authorization)}else if !self.isCurrent(requested){self.lock(requested,authorization:authorization)};complete(.failure(VaultError.rejected))}
        }
    }
}
