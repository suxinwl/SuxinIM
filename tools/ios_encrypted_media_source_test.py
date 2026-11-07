"""Offline contract/input tests. They do NOT claim Swift/Xcode runtime validation."""
import hashlib
import importlib.util
import json
from pathlib import Path
import plistlib
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
IOS = ROOT / 'sdks/flutter/ios'
spec = importlib.util.spec_from_file_location('ios_build', ROOT / 'clients/flutter/scripts/build-enterprise-core-ios.py')
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)

class FrameworkInputs(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name).resolve()
        self.cipher = self.root / 'SQLCipher.xcframework'
        self.cipher.mkdir()
        self.rows = []
        for key, architectures, variant in [('ios-arm64',['arm64'],None),('ios-arm64_x86_64-simulator',['arm64','x86_64'],'simulator')]:
            library=self.cipher/key/'SQLCipher.framework';(library/'Headers').mkdir(parents=True)
            (library/'Headers/sqlite3.h').write_text('/* approved synthetic input */')
            (library/'SQLCipher').write_bytes(b'not-a-real-dylib')
            row={'LibraryIdentifier':key,'LibraryPath':'SQLCipher.framework','SupportedPlatform':'ios','SupportedArchitectures':architectures}
            if variant:row['SupportedPlatformVariant']=variant
            self.rows.append(row)
        self.write_info()
    def tearDown(self):self.temporary.cleanup()
    def write_info(self):(self.cipher/'Info.plist').write_bytes(plistlib.dumps({'AvailableLibraries':self.rows}))
    def test_exact_hash_covers_all_header_and_binary_bytes(self):
        first=build.tree_sha(self.cipher)
        self.assertEqual(len(first),64);self.assertEqual(set(build.slices(self.cipher)),{'device','simulator'})
        (self.cipher/self.rows[0]['LibraryIdentifier']/'SQLCipher.framework/SQLCipher').write_bytes(b'changed')
        self.assertNotEqual(first,build.tree_sha(self.cipher))
    def test_escape_or_duplicate_slice_rejected(self):
        self.rows[0]['LibraryIdentifier']='../outside';self.write_info()
        with self.assertRaises(ValueError):build.slices(self.cipher)
        self.rows[0]['LibraryIdentifier']='ios-arm64';self.rows.append(dict(self.rows[0]));self.write_info()
        with self.assertRaises(ValueError):build.slices(self.cipher)
    def test_missing_simulator_arch_or_header_rejected(self):
        self.rows[1]['SupportedArchitectures']=['arm64'];self.write_info()
        with self.assertRaises(ValueError):build.slices(self.cipher)
        self.rows[1]['SupportedArchitectures']=['arm64','x86_64'];self.write_info()
        (self.cipher/'ios-arm64/SQLCipher.framework/Headers/sqlite3.h').unlink()
        with self.assertRaises(ValueError):build.slices(self.cipher)
    def test_build_copy_excludes_plain_test_databases_and_logs(self):
        source=self.root/'source';source.mkdir();(source/'go.mod').write_text('module example')
        (source/'a.go').write_text('package example');(source/'identifier.sqlite').write_bytes(b'private')
        (source/'server.log').write_bytes(b'private');(source/'.git').mkdir();(source/'.git/config').write_text('private')
        version=source/'version';version.mkdir();(version/'version').write_text('3.8.3')
        destination=self.root/'copy';build.copy_source(source,destination)
        self.assertEqual(sorted(p.relative_to(destination).as_posix() for p in destination.rglob('*') if p.is_file()),['a.go','go.mod','version/version'])
    def test_per_platform_link_overlay_does_not_modify_original_source(self):
        target=self.root/'fork';target.mkdir();(target/'go.mod').write_text('module example')
        original=(target/'go.mod').read_bytes();build.add_cipher_link(target,self.cipher/'ios-arm64/SQLCipher.framework')
        code=(target/'puty_ios_framework_link.go').read_text()
        self.assertIn('#cgo LDFLAGS:',code);self.assertIn('-framework SQLCipher',code)
        self.assertEqual(original,(target/'go.mod').read_bytes())
        self.assertNotIn('ios-arm64_x86_64-simulator',code)
    def test_framework_input_cannot_contain_links(self):
        target=self.cipher/'linked'
        try:target.symlink_to(self.cipher/'Info.plist')
        except OSError:self.skipTest('Host has no symlink privilege')
        with self.assertRaises(ValueError):build.inventory(self.cipher)

class IOSSourceContracts(unittest.TestCase):
    def code(self,name):return (IOS/'Classes'/name).read_text(encoding='utf-8')
    def test_only_encrypted_sink_for_media_and_file_selection(self):
        names=['PMSourceCoordinator.swift','PMCaptureController.swift','PMMediaOperations.swift','PMEncryptedEncoder.m']
        for name in names:
            code=self.code(name)
            for forbidden in ['AVCaptureMovieFileOutput','AVAssetExportSession','AVAssetWriter(','UIImagePickerController','NSTemporaryDirectory','temporaryDirectory','loadFileRepresentation','writeData(for:','AVAudioFile(forWriting','startAccessingSecurityScopedResource() == false']:
                self.assertNotIn(forbidden,code,name)
        source=self.code('PMSourceCoordinator.swift')
        self.assertIn('in:.open',source);self.assertIn('coordinate(readingItemAt:',source)
        self.assertIn('manager.requestData(for:',source);self.assertIn('manager.cancelDataRequest',source)
        self.assertNotIn('Open_im_sdkStageNativeSource',source) # no arbitrary plaintext Dart path importer
    def test_backup_master_remains_native_and_promotion_precedes_relock(self):
        code=self.code('NativeVaultBroker.swift')
        self.assertIn('RecoverNativeStoreRoot(directory.path)',code)
        self.assertLess(code.index('RecoverNativeStoreRoot(directory.path)'),code.index('accountMaster(user,directory)'))
        self.assertIn('self.wrapMaster(master,actor.userID)',code)
        commit=code.split('case "backup/commit":',1)[1]
        self.assertLess(commit.index('value.1.commit(envelope)'),commit.index('result(true)'))
        self.assertNotIn('master":',code);self.assertNotIn('"masterKey"',code)
        for verb in ['backup/export','backup/prepare','backup/cancel','backup/commit','attachment/list','attachment/save','attachment/read','archive/page']:
            self.assertIn('case "'+verb+'"',code)
    def test_ios_identity_and_existing_application_identity_unchanged(self):
        code=self.code('NativeVaultBroker.swift')
        self.assertIn('PMVerifiedSessionState.parse(state,userID:user)',code)
        contract=self.code('PMVaultContext.swift');self.assertIn('state["logicalClient"] as? String=="ios"',contract)
        self.assertIn('minimum:1',contract);self.assertIn('CFBooleanGetTypeID()',contract)
        self.assertIn('request.setValue("1",forHTTPHeaderField:"platformID")',code)
        plist=plistlib.loads((ROOT/'clients/flutter/ios/Runner/Info.plist').read_bytes())
        self.assertEqual(plist['CFBundleIdentifier'],'$(PRODUCT_BUNDLE_IDENTIFIER)')
        self.assertEqual(plist['RTCAppGroupIdentifier'],'group.io.openim.upgrade38.test.rtc')
        self.assertFalse(plist['NSAppTransportSecurity']['NSAllowsArbitraryLoads'])
        self.assertIn("s.platform = :ios, '13.0'",(IOS/'flutter_openim_sdk.podspec').read_text())
    def test_proof_is_header_bound_and_remote_ranges_version_checked(self):
        remote=self.code('PMRemoteAsset.swift');broker=self.code('NativeVaultBroker.swift');http=self.code('PMProtectedHTTP.swift')
        self.assertIn('forHTTPHeaderField:"X-Puty-Resource-Proof"',remote)
        self.assertIn('"/enterprise/v1/attachment/authorize",["url":url,"purpose":purpose]',broker)
        self.assertIn('http.statusCode==206',remote);self.assertIn('forHTTPHeaderField:"If-Range"',remote)
        self.assertIn('validator==self.etag',remote);self.assertIn('denied && !refreshed',remote)
        self.assertIn('completionHandler(nil)',http);self.assertIn('httpCookieStorage=nil',http)
        self.assertNotIn('resourceProof=',remote);self.assertNotIn('URLSession.shared',remote)
    def test_lease_cancels_all_media_consumers_and_stale_callbacks_cannot_reunlock(self):
        broker=self.code('NativeVaultBroker.swift');asset=self.code('PMProtectedAsset.swift');source=self.code('PMSourceCoordinator.swift')
        self.assertIn('cancellations.removeAll()',broker);self.assertIn('cancel.forEach{$0()}',broker)
        self.assertIn('generation==requested',broker);self.assertIn('self.generation &+= 1',broker)
        self.assertIn('authority?.canPreview(actor)==true',asset)
        self.assertIn('player?.replaceCurrentItem(with:nil)',source);self.assertIn('imageView.image=nil',source)
        self.assertIn('reader?.cancelReading()',self.code('PMMediaOperations.swift'))
        self.assertIn('self.session.stopRunning()',self.code('PMCaptureController.swift'))
    def test_new_native_framework_has_no_remote_plaintext_fallback(self):
        spec=(IOS/'flutter_openim_sdk.podspec').read_text()
        self.assertIn("'Framework/OpenIMCore.xcframework', 'Framework/SQLCipher.xcframework'",spec)
        self.assertNotIn("dependency 'OpenIMCore'",spec)
        for symbol in ['ReadNativeSourceRange','ReadNativeAttachmentRange','ListNativeAttachments','GetNativeLocalArchivePage']:
            self.assertIn(symbol,'\n'.join(self.code(name) for name in ['NativeVaultBroker.swift','PMProtectedAsset.swift']))
    def test_cancel_before_permission_and_before_queued_camera_start_have_closed_gate(self):
        code=self.code('PMCaptureController.swift')
        self.assertIn('if self.valid(){completion(allowed)}',code)
        self.assertIn('self.lifecycle.lock();guard !self.closed',code)
        start=code.split('self.lifecycle.lock();guard !self.closed',1)[1]
        self.assertIn('self.session.startRunning();self.lifecycle.unlock()',start)
        finish=code.split('private func finish(',1)[1]
        self.assertLess(finish.index('closed=true'),finish.index('samples.async'))
        self.assertIn('self.video.setSampleBufferDelegate(nil,queue:nil)',finish)
    def test_same_sid_refresh_keeps_store_and_old_transition_cannot_close_new_actor(self):
        code=self.code('NativeVaultBroker.swift')
        self.assertIn('authorizationSerial==authorization',code)
        self.assertIn('private let nativeTransition=NSRecursiveLock()',code)
        self.assertIn('previous != user{Open_im_sdkLockNativeLocalStore(previous)}',code)
        self.assertIn('self.generation==lockedGeneration&&self.activeAccount==nil',code)
        handle=code.split('func handle(',1)[1].split('queue.async',1)[0]
        self.assertNotIn('Open_im_sdkLockNativeLocalStore',handle)
        self.assertIn('"actorGeneration":NSNumber(value:actorGeneration)',code)
    def test_cipher_export_cleanup_is_exact_and_unregisters_its_capability(self):
        code=self.code('PMSourceCoordinator.swift')
        self.assertIn('exports:[String:PMOwnedBackupExport]',code)
        self.assertIn('value.registration=try authority.registerCancellation',code)
        cleanup=code.split('func releaseExport(',1)[1].split('func invalidateExports',1)[0]
        self.assertIn('authority?.unregisterCancellation(token)',cleanup);self.assertIn('value.removeIfUnchanged()',cleanup)
        owner=self.code('PMBackupExport.swift')
        self.assertIn('url.deletingLastPathComponent().resolvingSymlinksInPath().standardizedFileURL==expected.standardizedFileURL',owner)
        self.assertIn('value.st_ino',owner);self.assertIn('value.st_mtimespec.tv_sec',owner);self.assertIn('value.st_mtimespec.tv_nsec',owner)
        self.assertIn('value.st_nlink==1',owner);self.assertIn('PMBackupFileIdentity.at(url))==identity',owner)
        self.assertNotIn('removeItem(at:directory)',code)
        self.assertNotIn('removeItem(at:directory)',owner)

    def test_explicit_local_import_requires_current_save_actor_and_encrypted_source(self):
        code=self.code('NativeVaultBroker.swift')
        operation=code.split('case "attachment/import":',1)[1].split('case "attachment/save":',1)[0]
        self.assertIn('self.canSave(actor)',operation)
        self.assertLess(operation.index('pmSourceInfo(actor,path)'),operation.index('Open_im_sdkImportNativeLocalAttachment'))
        self.assertEqual(operation.count('self.canSave(actor)'),2)
        self.assertNotIn('removeItem',operation)
        self.assertIn('self.saveAllowed=verified.saveAllowed',code)
        clear=code.split('case "archive/previewClear":',1)[1].split('case "archive/clear":',1)[0]
        self.assertIn('Open_im_sdkPreviewNativeClearRange',clear)
        self.assertIn('from<=to',clear)
        self.assertIn('ImportNativeLocalAttachment',build.REQUIRED_SYMBOLS)
        self.assertIn('PreviewNativeClearRange',build.REQUIRED_SYMBOLS)

    def test_background_locks_native_store_without_claiming_server_revocation(self):
        code=self.code('NativeVaultBroker.swift')
        self.assertIn('lock(expected,reason:"BACKGROUND_LOCK")',code)
        self.assertIn('reason:String="ONLINE_AUTHORIZATION_REQUIRED"',code)
        self.assertIn('Open_im_sdkLockNativeLocalStore(account)',code)
        self.assertIn('invokeMethod("onVaultLocked",arguments:reason)',code)

    def test_union_clear_uses_bounded_half_open_scope_and_keeps_online_actor(self):
        code=self.code('NativeVaultBroker.swift')
        parser=code.split('private func clearScopes(',1)[1].split('private func renew(',1)[0]
        self.assertIn('values.count<=100',parser)
        self.assertIn('Array(Set(values)).sorted()',parser)
        self.assertIn('pmInteger(args["toExclusive"])',parser)
        self.assertIn('from<to',parser)
        for route,symbol in [('archive/previewClearScopes','PreviewNativeClearScopes'),('archive/clearScopes','ClearNativeLocalScopes')]:
            self.assertIn('case "'+route+'"',code)
            self.assertIn('Open_im_sdk'+symbol,code)
            self.assertIn(symbol,build.REQUIRED_SYMBOLS)
        clear=code.split('private func performClear(',1)[1].split('private func clearScopes(',1)[0]
        self.assertNotIn('Open_im_sdkLockNativeLocalStore',clear)
        self.assertLess(clear.index('sources.invalidatePreviews()'),clear.index('cancellations.values'))
        self.assertLess(clear.index('cancel.forEach{$0()}'),clear.index('operation()'))
        self.assertIn('clearing=true',clear);self.assertIn('clearing=false',clear)
        self.assertIn('!clearing,cancellations.count<128',code)

    def test_clear_rejects_queued_or_already_decoded_preview_before_presentation(self):
        code=self.code('PMSourceCoordinator.swift')
        self.assertIn('let revision=previewRevision',code)
        self.assertIn('func invalidatePreviews(){state.lock();previewRevision &+= 1;state.unlock()}',code)
        presentation=code.split('DispatchQueue.main.async{self.state.lock();defer{self.state.unlock()}',1)[1]
        self.assertLess(presentation.index('revision==self.previewRevision,asset.isActive()'),presentation.index('self.presenter().present('))
        self.assertIn('func isActive()->Bool{return authorized()}',self.code('PMProtectedAsset.swift'))

    def test_large_restore_prepares_with_live_renewal_before_short_atomic_transition(self):
        code=self.code('NativeVaultBroker.swift').split('case "backup/commit":',1)[1].split('default:',1)[0]
        prepare=code.index('value.1.prepareCommit(envelope)')
        self.assertLess(code.index('self.clearing=true'),prepare)
        self.assertLess(code.index('preparingCancel.forEach{$0()}'),prepare)
        self.assertNotIn('self.credentials=nil',code[:prepare])
        self.assertLess(prepare,code.index('self.nativeTransition.lock()'))
        self.assertLess(code.index('self.nativeTransition.lock()'),code.index('value.1.commit(envelope)'))
        self.assertIn('self.lock(actor.generation)',code)
        self.assertIn('prepareCommit:',build.REQUIRED_SYMBOLS)

if __name__=='__main__':unittest.main()
