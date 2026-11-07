# iOS encrypted media source integration

This implementation preserves iOS 13, the existing application/extension bundle
identities, entitlements and Keychain service. `platformID=1` and
`logicalClient=ios` remain distinct. There is no remote/plain SQLite framework
fallback in the podspec.

The native broker obtains an 8-second lease from the complete, current
`/enterprise/v1/session/state` decision. It renews every 3 seconds with a 2.5-second
deadline. Missing policy fields, mismatched epoch/client, logout, backgrounding,
or failed renewal invalidate the generation and cancel registered producers,
readers and preview controllers. Masters, resource proofs, selected security
scope URLs and restore objects remain native.
Same-user/SID/epoch/origin token refresh preserves the actor generation and open
SDK database; a separate authorization serial prevents late HTTP verification
from replacing newer credentials. Native key/DB transitions are serialized, and
an old queued lock notification cannot clear a newly authorized actor.
Backgrounding still closes native keys and consumers immediately, but reports
`BACKGROUND_LOCK` separately from server revocation. Shared Flutter code must
perform fresh online verification and SDK-only re-login on resume; the native
broker never reopens a locked store from an old timer or cached permission.

## Data paths

- `picker/files`: `UIDocumentPicker` open-in-place, security-scoped coordinated
  reads, 64 KiB blocks directly into SQLCipher scratch. It returns an encrypted
  outgoing path and the original source name. An arbitrary Dart filesystem path
  is never imported.
- `picker/assets`: native Photos selection and
  `PHAssetResourceManager.requestData`; there is no file-representation export.
  HEIC/TIFF and other native-only still formats are converted in bounded RAM to
  JPEG with the original basename, avoiding an unsupported Go image decoder.
- `media/capture`: `AVCapturePhotoOutput` JPEG bytes or video/audio sample
  outputs. VideoToolbox H.264 without B frames and AudioConverter AAC-LC feed
  `PMFragmentMuxer`, whose synchronous callback appends to encrypted scratch.
  The camera's independent closed gate rejects late permission callbacks and
  queued starts after cancellation.
- `media/compress`: encrypted byte-range resource loader → AVAssetReader → the
  same encoder/muxer → encrypted scratch. Thumbnail generation uses JPEG bytes
  in RAM. The Flutter crop and still-image compression flows also use RAM bytes
  and `source/fromBytes`, not a file output.
- `media/preview`: native image view or AVPlayer, no external playback/PiP/share.
  Local sources and saved attachments use authenticated native range APIs.
  Remote media first obtains the current viewer's `preview` grant; HEAD/Range
  use `X-Puty-Resource-Proof`, a strong ETag and If-Range. Only a single same-actor
  authorization refresh is permitted. No plaintext cache or redirect is used.
- `attachment/save`: independent `save` authorization before using even an
  already complete encrypted task. Resume uses Range/If-Range; a changed object
  restarts from zero once. Preview and save are separate permissions.
- `attachment/import`: only an explicitly selected, already encrypted native
  source may be associated with a selected conversation. The current actor and
  `saveAllowed` are checked before and after the native operation. It creates a
  local import reference without fabricating an SDK message; the external
  original is retained. `archive/previewClear` includes these local references
  when showing the proposed cleanup counts.
- Backups: export is a ciphertext ZIP (`.backup`), selected input is an opaque
  one-use security-scope handle, and a prepared native restore never exposes its
  master to Dart. The native OS wrapper is passed to the Go promotion protocol
  before replacement; `RecoverNativeStoreRoot` runs before reading `master.wrap`.
  SDK-only logout plus the native restore fence precede commit. Completion
  locks the store and requires a fresh login. Export files are retained only by
  exact native identity and removed on save/cancel/lock if unchanged.
- Existing SDK databases retain the old Documents dataDir as the migration
  input. Native-only sandbox roots cover Documents, Caches and tmp; the entire
  active Library/Preferences tree is not scanned. A separate device Keychain
  master protects other-account/unknown legacy quarantine. `legacy/finalize`
  runs only after the shared Flutter preference/Hive and SDK migrations finish;
  unrelated accounts are never imported using the current account master.

## Build and validation

Windows has executed the actual C muxer under WebAssembly (five tests) and the
Python input/source contracts. Additional Chrome tests encoded real H.264
samples, combined them with independent Android MediaCodec AAC-LC samples in
the same C muxer, and verified HTMLVideo playback, red/blue frames, seeking and
audio waveform decoding. The safe receipt is
`clients/web-desktop/build/acceptance/ios-muxer-playback.json`. These tests do
**not** prove VideoToolbox/AVFoundation behavior or Swift/Objective-C compilation.

On macOS, provide the approved SQLCipher XCFramework containing iOS arm64 and
simulator arm64/x86_64, then run the pinned builder:

```sh
python3 clients/flutter/scripts/build-enterprise-core-ios.py \
  --sqlcipher-xcframework /approved/SQLCipher.xcframework --describe-input
python3 clients/flutter/scripts/build-enterprise-core-ios.py \
  --sqlcipher-xcframework /approved/SQLCipher.xcframework \
  --sqlcipher-tree-sha256 APPROVED_COMPLETE_TREE_SHA \
  --go /approved/go1.26.9/bin/go --gomobile /approved/bin/gomobile \
  --output /new/candidate-directory
```

The builder works offline, verifies the fixed Go/mobile/base SDK and public CA,
and creates per-platform temporary SQLite forks containing only the required
SQLCipher link directives. It never edits the authoritative Go source, signs an
app, installs pods, or overwrites existing framework candidates. Review the
candidate manifest and then place its two frameworks in this plugin's
`Framework/` directory before CocoaPods/Xcode compilation.

Mac acceptance must compile the complete Flutter iOS app at the existing target
identities, link an XCTest target to `Tests/PMEncryptedMediaTests.swift`, and run
actual encrypted scratch/VideoToolbox→resource-loader decode tests. Also test
camera photo/video/audio, Photos/iCloud and Files selections, compression and
orientation, cancellation before/after permission, logout/background/lease loss
during recording or preview, large Range resume, encrypted backup export/import
and interrupted promotion recovery. Inspect the application container for
plaintext media/database residues. No Mac build, signing, or device result is
claimed by the current Windows evidence.

The multi-conversation cleanup routes are `archive/previewClearScopes` and
`archive/clearScopes`: 1–100 selected conversation IDs, deduplicated/sorted,
`from` inclusive and `toExclusive` exclusive in UTC milliseconds. The native
preview returns the union message/import/reference/file counts and actual
deduplicated ciphertext `estimatedReleaseBytes` (not SQLCipher compaction).
Clearing cancels all current previews/producers and prepared restores without
locking the still-authorized actor. A coordinator revision rejects pre-clear
queued image/video presentation even when decoding had already finished.
This new route and cancellation behavior still require Mac/device acceptance.

Backup promotion uses the current native `NativeRestore.prepareCommit` method
for long final merge/hash preparation outside the broker transition, with the
same actor's online renewal and credentials retained. New consumers are fenced
and existing ones canceled during this stage. Only the short durable promotion
then runs inside the native transition after the actor/lease is rechecked.

API references: [Photos streaming](https://developer.apple.com/documentation/photos/phassetresourcemanager/requestdata(for:options:datareceivedhandler:completionhandler:)),
[VideoToolbox encoder](https://developer.apple.com/documentation/videotoolbox/vtcompressionsessioncreate(allocator:width:height:codectype:encoderspecification:imagebufferattributes:compresseddataallocator:outputcallback:refcon:compressionsessionout:)),
[AudioConverter input](https://developer.apple.com/documentation/audiotoolbox/audioconvertercomplexinputdataproc).
