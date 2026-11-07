import Foundation
import Darwin

// The system export picker copies outside application authority. Choose a
// directory instead, then keep the ciphertext copy under the live save gate.
@available(iOS 13.0, *)
fileprivate struct PMBackupFileIdentity:Equatable {
    let device:dev_t,inode:ino_t,size:off_t,seconds:time_t,nanoseconds:Int
    init(_ value:stat)throws {
        guard value.st_mode & S_IFMT == S_IFREG,value.st_nlink==1 else{throw PMProtectedError.rejected}
        device=value.st_dev;inode=value.st_ino;size=value.st_size;seconds=value.st_mtimespec.tv_sec;nanoseconds=value.st_mtimespec.tv_nsec
    }
    static func at(_ url:URL)throws->PMBackupFileIdentity{var value=stat();guard lstat(url.path,&value)==0 else{throw PMProtectedError.storage};return try PMBackupFileIdentity(value)}
    static func descriptor(_ fd:Int32)throws->PMBackupFileIdentity{var value=stat();guard fstat(fd,&value)==0 else{throw PMProtectedError.storage};return try PMBackupFileIdentity(value)}
}

@available(iOS 13.0, *)
final class PMBackupSavedCopy {
    private let url:URL,directory:URL,identity:PMBackupFileIdentity
    fileprivate init(_ url:URL,_ directory:URL,_ identity:PMBackupFileIdentity){self.url=url;self.directory=directory;self.identity=identity}
    func removeIfUnchanged(){let scoped=directory.startAccessingSecurityScopedResource();defer{if scoped{directory.stopAccessingSecurityScopedResource()}};if (try? PMBackupFileIdentity.at(url))==identity{try? FileManager.default.removeItem(at:url)}}
}

@available(iOS 13.0, *)
final class PMOwnedBackupExport {
    let actor:PMVaultActor,url:URL;var registration:UUID?
    private let identity:PMBackupFileIdentity
    init(path:String,actor:PMVaultActor,directory:URL)throws {
        let url=URL(fileURLWithPath:path),expected=directory.resolvingSymlinksInPath().appendingPathComponent("backups",isDirectory:true)
        guard url.lastPathComponent.range(of:"^PUTY_IM-[0-9]{8}-[0-9]{6}-[0-9a-f]{24}\\.backup$",options:.regularExpression) != nil,url.deletingLastPathComponent().resolvingSymlinksInPath().standardizedFileURL==expected.standardizedFileURL else{throw PMProtectedError.rejected}
        let identity=try PMBackupFileIdentity.at(url);guard identity.size>0 else{throw PMProtectedError.rejected};self.url=url;self.actor=actor;self.identity=identity
    }
    func unchanged()->Bool{return (try? PMBackupFileIdentity.at(url))==identity}
    func removeIfUnchanged(){if unchanged(){try? FileManager.default.removeItem(at:url)}}
    func copy(to directory:URL,check:()->Bool)throws->PMBackupSavedCopy {
        guard check(),unchanged(),directory.isFileURL else{throw PMProtectedError.rejected}
        let output=directory.appendingPathComponent(url.lastPathComponent),input=Darwin.open(url.path,O_RDONLY|O_NOFOLLOW)
        guard input>=0 else{throw PMProtectedError.storage};defer{Darwin.close(input)}
        guard try PMBackupFileIdentity.descriptor(input)==identity,check() else{throw PMProtectedError.rejected}
        let destination=Darwin.open(output.path,O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,S_IRUSR|S_IWUSR)
        guard destination>=0 else{throw PMProtectedError.storage}
        var complete=false,initial:PMBackupFileIdentity?
        defer{
            Darwin.close(destination)
            if !complete,let initial=initial,let current=try? PMBackupFileIdentity.at(output),current.device==initial.device,current.inode==initial.inode{try? FileManager.default.removeItem(at:output)}
        }
        initial=try PMBackupFileIdentity.descriptor(destination)
        var buffer=[UInt8](repeating:0,count:65536),total:Int64=0
        defer{buffer.withUnsafeMutableBytes{$0.initializeMemory(as:UInt8.self,repeating:0)}}
        while true {
            guard check() else{throw PMProtectedError.rejected}
            let count=buffer.withUnsafeMutableBytes{Darwin.read(input,$0.baseAddress,$0.count)}
            if count<0{if errno==EINTR{continue};throw PMProtectedError.storage}
            guard check() else{throw PMProtectedError.rejected};if count==0{break}
            var offset=0
            while offset<count {
                guard check() else{throw PMProtectedError.rejected}
                let written=buffer.withUnsafeBytes{Darwin.write(destination,$0.baseAddress!.advanced(by:offset),count-offset)}
                if written<0{if errno==EINTR{continue};throw PMProtectedError.storage}
                guard written>0,check() else{throw PMProtectedError.rejected};offset+=written
            }
            total+=Int64(count)
        }
        guard check(),total==identity.size,try PMBackupFileIdentity.descriptor(input)==identity,unchanged(),fsync(destination)==0,check() else{throw PMProtectedError.rejected}
        let saved=try PMBackupFileIdentity.descriptor(destination)
        guard saved.size==identity.size,try PMBackupFileIdentity.at(output)==saved,check() else{throw PMProtectedError.rejected}
        complete=true;return PMBackupSavedCopy(output,directory,saved)
    }
}
