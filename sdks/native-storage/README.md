# Native encrypted SQLite dependency

The Go adapter is a controlled source fork of mattn/go-sqlite3 v1.14.22. Its
unencrypted SQLite amalgamation is intentionally absent. Android release builds
must use `puty_sqlcipher,sqlite_omit_load_extension` and link the pinned official
SQLCipher 4.19.1 Android native library. The source and artifact hashes are in
`go-sqlite3/UPSTREAM.json` and `sqlcipher-android-4.19.1/source.json`.

Keys enter a connection through `sqlite3_key` before any database query, never
through a DSN or a logged PRAGMA statement. Database opening must additionally
verify `PRAGMA cipher_version` and a real encrypted read; a successful plain
SQLite open does not satisfy the storage contract.

iOS needs the same adapter linked against a freshly built SQLCipher static
library in the local core XCFramework. It must not use the former remote
unencrypted OpenIMSDKCore Pod as a fallback. iOS artifacts are not yet built or
device accepted on this Windows host.

The 3.8.18+118 Android candidate is being rebuilt with SQLCipher for its working database and
PUTY-LOCAL-STORE-1 authenticated monthly records and explicitly saved files.
Online native authorization renews every three seconds with an eight-second
monotonic lease. A socket timeout may retry once only while the same verified
actor/proof and original lease remain current, within the original 1500ms
socket and 2500ms request limits. Initial unlock and authorization, identity,
certificate or response validation failures do not retry. A final failed renewal closes readers and producers; it does not
enable an offline plaintext fallback. Final release and normal Main acceptance
are still pending; the API30 x86_64 storage tests are not ARM64 device evidence.
The user has confirmed that no connected ARM64 physical device is currently
available. Physical-device acceptance remains incomplete while the authorized
emulator acceptance and candidate publication continue. Successful ARM64
compilation, signing and static package checks do not replace that acceptance.

Current-account legacy SQLite migration streams through SQLCipher
ATTACH/sqlcipher_export and does **not** have a 256 MiB database limit. The
256 MiB bound applies only to RAM snapshots of unknown/other-account legacy
databases in the device quarantine and their later account-bound recovery.
Exceeding that bound fails closed and retains the original; it is never reported
as encrypted or deleted. Such unknown ownership requires explicit review.
Public Downloads and other externally exported copies are never automatically
claimed or deleted. Explicit SAF import keeps the external original and creates
an independent conversation/localImportID/firstImportedAt reference, without
inventing an SDK message.

Cleanup accepts a union of conversations and a half-open UTC millisecond range;
the persistent suppression range stops at the confirmation time. Already selected
messages with future-skewed timestamps retain exact deletion IDs, while later new
messages are not swallowed by a request extending through the end of a day.
The 43 top-level Android SQLCipher tests were covered across a full run and a
corrected resync-fixture rerun; this is not described as a single green full run.

the calendar interpretation is fixed Asia/Shanghai. Physical ciphertext paths
shared by unselected references are retained and estimated release bytes count
only the deduplicated files eligible for GC, excluding SQLCipher compaction.
Restoring an old backup re-merges the latest exact deletion, local tombstone and
ephemeral state after the SDK logout fence. Long verification runs outside the
native actor transition so normal short leases can renew; the promotion is a
short journaled, directory-fsynced operation. Startup reconstructs interrupted
rename states before master unwrap or creating a missing account directory.

The previous application-owned encrypted vault remains until fresh SDK sync.
GC then requires an authenticated receipt and a pinned SQLCipher ledger binding
every original path, ciphertext SHA, size, device and inode. Retry verifies the
entire remaining subset and rejects unknown files or a replaced root; receipt,
ledger and parent durability are re-established before the first unlink.
External backup copies are outside this GC scope.
