import Foundation
import OpenIMCore
import Network
import CoreFoundation

func pmBoolean(_ value:Any?)->Bool?{guard let number=value as? NSNumber,CFGetTypeID(number)==CFBooleanGetTypeID() else{return nil};return number.boolValue}
func pmInteger(_ value:Any?,minimum:Int64=0)->Int64?{guard let number=value as? NSNumber,CFGetTypeID(number) != CFBooleanGetTypeID() else{return nil};let double=number.doubleValue;guard double.isFinite,double.rounded(.towardZero)==double,double>=Double(minimum),double<=9007199254740991 else{return nil};return number.int64Value}

@available(iOS 13.0, *)
struct PMVerifiedSessionState {
    let sessionID:String,epoch:Int64,expiresAt:Double,saveAllowed:Bool
    static func parse(_ state:[String:Any],userID:String)throws->PMVerifiedSessionState{
        guard state["userID"] as? String==userID,state["logicalClient"] as? String=="ios",pmInteger(state["platform"])==1,pmBoolean(state["loginAllowed"])==true,
          let sid=state["sessionID"] as? String,!sid.isEmpty,sid.utf8.count<=512,let epoch=pmInteger(state["epoch"],minimum:1),let expiry=pmInteger(state["expiresAt"],minimum:1),let save=pmBoolean(state["saveAllowed"]),
          let policy=state["effectivePolicy"] as? [String:Any],policy["logicalClient"] as? String=="ios",pmBoolean(policy["login"])==true,pmBoolean(policy["preview"])==true,pmBoolean(policy["save"])==save,pmInteger(policy["securityVersion"],minimum:1)==epoch,
          let zone=policy["zone"] as? String,["lan","internet"].contains(zone),pmInteger(policy["policyRevision"]) != nil,pmInteger(policy["networkRevision"]) != nil,
          let sources=policy["sources"] as? [String:Any],let loginSource=sources["login"] as? String,!loginSource.isEmpty,loginSource.utf8.count<=128,let saveSource=sources["save"] as? String,!saveSource.isEmpty,saveSource.utf8.count<=128,
          let ip=policy["currentIP"] as? String,(IPv4Address(ip) != nil||IPv6Address(ip) != nil) else{throw PMProtectedError.rejected}
        let milliseconds=expiry<100000000000 ? Double(expiry)*1000:Double(expiry);guard milliseconds>Date().timeIntervalSince1970*1000 else{throw PMProtectedError.rejected};return PMVerifiedSessionState(sessionID:sid,epoch:epoch,expiresAt:milliseconds,saveAllowed:save)
    }
}

@available(iOS 13.0, *)
struct PMVaultActor: Equatable {
    let userID:String,sessionID:String,origin:String
    let epoch:Int64,generation:UInt64
}

@available(iOS 13.0, *)
protocol PMVaultAuthority: AnyObject {
    func currentActor() -> PMVaultActor?
    func check(_ actor:PMVaultActor) -> Bool
    func canPreview(_ actor:PMVaultActor) -> Bool
    func canSave(_ actor:PMVaultActor) -> Bool
    func registerCancellation(_ actor:PMVaultActor,_ cancel:@escaping()->Void)throws->UUID
    func unregisterCancellation(_ id:UUID)
}

enum PMProtectedError: Error {case rejected,cancelled,invalidInput,storage,unsupported,network}

/// A scratch never has a filesystem URL. All pages, including interrupted
/// writes, remain in SQLCipher; the SDK receives an authenticated outgoing file
/// only after stage succeeds for the exact original actor.
@available(iOS 13.0, *)
final class PMEncryptedScratch {
    private weak var authority:PMVaultAuthority?
    let actor:PMVaultActor,name:String,id:String
    private let guardLock=NSRecursiveLock()
    private var closed=false,offset:Int64=0
    init(authority:PMVaultAuthority,actor:PMVaultActor,name:String)throws {
        guard authority.check(actor),!name.isEmpty,name.utf8.count<=255,!name.contains("/"),!name.contains("\\"),!name.unicodeScalars.contains(where:{CharacterSet.controlCharacters.contains($0)}),
              let id=Open_im_sdkBeginNativeScratch(actor.userID,name) as String?,!id.isEmpty else{throw PMProtectedError.rejected}
        self.authority=authority;self.actor=actor;self.name=name;self.id=id
        guard authority.check(actor) else{Open_im_sdkCloseNativeScratch(actor.userID,id);throw PMProtectedError.rejected}
    }
    deinit{cancel()}
    private func authorized()throws{guard !closed,authority?.check(actor)==true else{throw PMProtectedError.rejected}}
    func append(_ data:Data)throws {guardLock.lock();defer{guardLock.unlock()};try authorized();guard offset<=Int64(2<<30)-Int64(data.count) else{throw PMProtectedError.invalidInput}
        var cursor=0;while cursor<data.count{try authorized();let count=min(65536,data.count-cursor);var chunk=data.subdata(in:cursor..<cursor+count);defer{chunk.resetBytes(in:0..<chunk.count)}
            guard Open_im_sdkWriteNativeScratch(actor.userID,id,offset,chunk) else{throw PMProtectedError.storage};try authorized();offset+=Int64(count);cursor+=count}
    }
    func read(offset:Int64,count:Int)throws->Data {guardLock.lock();defer{guardLock.unlock()};try authorized();guard offset>=0,count>=0,count<=1<<20 else{throw PMProtectedError.invalidInput};let size=Open_im_sdkNativeScratchSize(actor.userID,id);guard size>=0 else{throw PMProtectedError.storage};if count==0||offset>=size{return Data()}
        guard var result=Open_im_sdkReadNativeScratch(actor.userID,id,offset,min(count,Int(size-offset))) as Data? else{throw PMProtectedError.storage};do{try authorized();return result}catch{result.resetBytes(in:0..<result.count);throw error}
    }
    func size()throws->Int64{guardLock.lock();defer{guardLock.unlock()};try authorized();let value=Open_im_sdkNativeScratchSize(actor.userID,id);try authorized();guard value>=0 else{throw PMProtectedError.storage};return value}
    func stage()throws->String{guardLock.lock();defer{guardLock.unlock()};try authorized();guard let path=Open_im_sdkStageNativeScratch(actor.userID,id) as String?,!path.isEmpty else{throw PMProtectedError.storage};closed=true;guard authority?.check(actor)==true else{throw PMProtectedError.rejected};return path}
    func cancel(){guardLock.lock();defer{guardLock.unlock()};if !closed{closed=true;Open_im_sdkCloseNativeScratch(actor.userID,id)}}
}

@available(iOS 13.0, *)
func pmSourceInfo(_ actor:PMVaultActor,_ path:String)throws->[String:Any]{
    guard let text=Open_im_sdkNativeSourceInfo(actor.userID,path) as String?,let bytes=text.data(using:.utf8),let info=try JSONSerialization.jsonObject(with:bytes) as? [String:Any],let name=info["name"] as? String,!name.isEmpty,let size=info["size"] as? NSNumber,size.int64Value>=0 else{throw PMProtectedError.rejected};return info
}
