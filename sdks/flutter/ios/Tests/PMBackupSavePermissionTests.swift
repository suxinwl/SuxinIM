// Real filesystem tests for an iOS13+ XCTest plugin target. These have NOT
// executed on Windows; source-contract checks are not a Mac/device PASS.
import XCTest
@testable import flutter_openim_sdk

@available(iOS 13.0, *)
final class PMBackupSavePermissionTests:XCTestCase {
    private let actor=PMVaultActor(userID:"emp_00000000000000000000000000000000",sessionID:"offline-backup-test",origin:"https://im.szputy.com:9885",epoch:1,generation:1)
    private func fixture(_ run:(PMOwnedBackupExport,URL,URL,Data)throws->Void)throws {
        let root=FileManager.default.temporaryDirectory.appendingPathComponent("puty-ios-backup-save-"+UUID().uuidString,isDirectory:true)
        try FileManager.default.createDirectory(at:root,withIntermediateDirectories:false);defer{try? FileManager.default.removeItem(at:root)}
        let account=root.appendingPathComponent("account",isDirectory:true),backups=account.appendingPathComponent("backups",isDirectory:true),destination=root.appendingPathComponent("selected",isDirectory:true)
        try FileManager.default.createDirectory(at:backups,withIntermediateDirectories:true);try FileManager.default.createDirectory(at:destination,withIntermediateDirectories:false)
        let source=backups.appendingPathComponent("PUTY_IM-20261104-120000-aaaaaaaaaaaaaaaaaaaaaaaa.backup"),bytes=Data(repeating:0x6d,count:65536*3+17)
        try bytes.write(to:source);try run(PMOwnedBackupExport(path:source.path,actor:actor,directory:account),source,destination,bytes)
    }
    func testPermittedCopyMatchesBytesAndPreservesInternalBackup()throws {try fixture{export,source,destination,bytes in
        _ = try export.copy(to:destination,check:{true})
        XCTAssertEqual(try Data(contentsOf:destination.appendingPathComponent(source.lastPathComponent)),bytes)
        XCTAssertEqual(try Data(contentsOf:source),bytes)
    }}
    func testDeniedSaveCreatesNoCopy()throws {try fixture{export,source,destination,bytes in
        XCTAssertThrowsError(try export.copy(to:destination,check:{false}))
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath:destination.path).isEmpty)
        XCTAssertEqual(try Data(contentsOf:source),bytes)
    }}
    func testSaveRevokedDuringCopyDeletesOnlyOwnPartial()throws {try fixture{export,source,destination,bytes in
        var checks=0
        XCTAssertThrowsError(try export.copy(to:destination,check:{checks+=1;return checks<8}))
        XCTAssertGreaterThanOrEqual(checks,8)
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath:destination.path).isEmpty)
        XCTAssertEqual(try Data(contentsOf:source),bytes)
    }}
    func testExistingDestinationIsNeverOverwritten()throws {try fixture{export,source,destination,bytes in
        let output=destination.appendingPathComponent(source.lastPathComponent),foreign=Data("unrelated-existing-file".utf8);try foreign.write(to:output)
        XCTAssertThrowsError(try export.copy(to:destination,check:{true}))
        XCTAssertEqual(try Data(contentsOf:output),foreign);XCTAssertEqual(try Data(contentsOf:source),bytes)
    }}
    func testLateResultCleanupKeepsForeignReplacement()throws {try fixture{export,source,destination,_ in
        let saved=try export.copy(to:destination,check:{true}),output=destination.appendingPathComponent(source.lastPathComponent),foreign=Data("new-unrelated-file".utf8)
        try foreign.write(to:output,options:.atomic);saved.removeIfUnchanged()
        XCTAssertEqual(try Data(contentsOf:output),foreign)
    }}
    func testLateResultCleanupRemovesExactOwnedCopy()throws {try fixture{export,source,destination,bytes in
        let saved=try export.copy(to:destination,check:{true});saved.removeIfUnchanged()
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath:destination.path).isEmpty)
        XCTAssertEqual(try Data(contentsOf:source),bytes)
    }}
    func testChangedSourceRejectsCopyAndPreservesReplacement()throws {try fixture{export,source,destination,_ in
        let replacement=Data("changed-internal-backup".utf8);try replacement.write(to:source,options:.atomic)
        XCTAssertThrowsError(try export.copy(to:destination,check:{true}))
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath:destination.path).isEmpty)
        XCTAssertEqual(try Data(contentsOf:source),replacement)
    }}
}
