"""Windows source-contract checks only; never a Swift compile/device claim."""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[4]
CLASSES = ROOT / 'sdks/flutter/ios/Classes'


class BackupSaveContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.broker = (CLASSES / 'NativeVaultBroker.swift').read_text(encoding='utf-8')
        cls.sources = (CLASSES / 'PMSourceCoordinator.swift').read_text(encoding='utf-8')
        cls.copy = (CLASSES / 'PMBackupExport.swift').read_text(encoding='utf-8')

    def test_every_authority_conformance_implements_save(self):
        protocol = (CLASSES / 'PMVaultContext.swift').read_text(encoding='utf-8')
        self.assertIn('func canSave(_ actor:PMVaultActor) -> Bool', protocol)
        paths = [p for p in (ROOT / 'sdks/flutter/ios').rglob('*.swift')
                 if re.search(r'class\s+\w+[^\n{]*:\s*[^\n{]*\bPMVaultAuthority\b', p.read_text(encoding='utf-8'))]
        self.assertEqual({p.name for p in paths}, {'NativeVaultBroker.swift', 'PMEncryptedMediaTests.swift'})
        for path in paths:
            self.assertIn('func canSave(_ actor:PMVaultActor)', path.read_text(encoding='utf-8'))

    def test_save_predicate_binds_complete_actor_and_original_lease(self):
        line = next(v for v in self.broker.splitlines() if 'func canSave(' in v)
        for required in ['stateLock.lock()', 'stateLock.unlock()', '!closed', '!clearing', 'saveAllowed',
                         'activeAccount==actor.userID', 'sessionID==actor.sessionID', 'epoch==actor.epoch',
                         'actor.origin', 'generation==actor.generation', 'systemUptime<leaseDeadline']:
            self.assertIn(required, line)

    def test_generation_before_after_and_final_response_gate(self):
        export = self.broker.split('case "backup/export":', 1)[1].split('case "backup/prepare":', 1)[0]
        self.assertEqual(export.count('self.canSave(actor)'), 2)
        self.assertLess(export.index('self.canSave(actor)'), export.index('Open_im_sdkExportNativeBackup'))
        self.assertGreater(export.rindex('self.canSave(actor)'), export.index('retainExport'))
        self.assertIn('saved.removeIfUnchanged()', self.broker)
        self.assertIn('call.method=="backup/export" || call.method=="backup/save"', self.broker)
        self.assertIn('discardBackupResult()', self.broker)

    def test_system_copier_is_not_used_and_folder_flow_checks_save(self):
        self.assertNotIn('.exportToService', self.sources)
        self.assertIn('kUTTypeFolder as String],in:.open', self.sources)
        self.assertIn('method != "backup/save" || authority.canSave(actor)', self.sources)
        self.assertIn('owner.copy(to:directory,check:{authority.canSave(actor)})', self.sources)
        self.assertIn('startAccessingSecurityScopedResource()', self.sources)
        self.assertIn('stopAccessingSecurityScopedResource()', self.sources)

    def test_copy_is_bounded_nonoverwriting_and_live_gated(self):
        for required in ['count:65536', 'O_RDONLY|O_NOFOLLOW', 'O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW',
                         'value.st_nlink==1', 'fsync(destination)==0,check()', 'Darwin.read(', 'Darwin.write(',
                         'total==identity.size', 'PMBackupFileIdentity.descriptor(input)==identity',
                         'current.device==initial.device,current.inode==initial.inode', 'removeIfUnchanged()']:
            self.assertIn(required, self.copy)
        self.assertGreaterEqual(self.copy.count('check()'), 8)
        self.assertIn('while offset<count', self.copy)
        self.assertNotIn('copyItem(', self.copy)

    def test_save_reduction_only_cancels_export_dialogs(self):
        invalidate = next(v for v in self.sources.splitlines() if 'func invalidateExports(' in v)
        self.assertIn('saveDialogs.values.filter{$0.0==actor}', invalidate)
        self.assertNotIn('lock(', invalidate.replace('state.lock()', '').replace('state.unlock()', ''))
        self.assertIn('if !verified.saveAllowed{self.sources.invalidateExports(', self.broker)

    def test_restore_and_input_picker_keep_login_only_authority(self):
        prepare = self.broker.split('case "backup/prepare":', 1)[1].split('case "backup/cancel":', 1)[0]
        self.assertNotIn('canSave', prepare)
        self.assertIn('self.check(actor)', prepare)
        input_flow = self.sources.split('func withBackupInput', 1)[1].split('private func presenter', 1)[0]
        self.assertNotIn('canSave', input_flow)
        self.assertIn('require(actor)', input_flow)

    def test_real_filesystem_tests_are_available_but_not_claimed_executed(self):
        tests = pathlib.Path(__file__).with_name('PMBackupSavePermissionTests.swift').read_text(encoding='utf-8')
        for case in ['testPermittedCopyMatchesBytesAndPreservesInternalBackup', 'testDeniedSaveCreatesNoCopy',
                     'testSaveRevokedDuringCopyDeletesOnlyOwnPartial', 'testExistingDestinationIsNeverOverwritten',
                     'testLateResultCleanupKeepsForeignReplacement', 'testLateResultCleanupRemovesExactOwnedCopy',
                     'testChangedSourceRejectsCopyAndPreservesReplacement']:
            self.assertIn('func ' + case, tests)
        self.assertIn('NOT', tests)
        self.assertIn('executed on Windows', tests)


if __name__ == '__main__':
    unittest.main()
