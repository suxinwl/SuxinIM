Android unknown-file quarantine integration
==========================================

`MobileLegacyQuarantine.java` is a native-only helper. It does not register any
MethodChannel, perform network I/O, or infer an owner from the currently logged-in
account. Do not add its device archive to an account's ordinary backup.

Broker integration order
------------------------

1. `MobileLegacyQuarantine.create(context)` opens/creates the independently
   OS-wrapped device master. It does **not** scan, move, or delete legacy files.
2. Give Go `directory().getCanonicalPath()` and `masterCopy()` through the
   existing native-only configuration method. Zero the returned key in `finally`.
3. Complete the signed-in actor's verified database/media migration and encrypted
   preference/Hive migration. Close every old producer. A Dart async `onInit`
   without an awaited completion gate is insufficient.
4. Let Go transactionally serialize/quarantine remaining SQLite databases. Java
   deliberately retains every `OpenIM_v3_*.db` and its WAL/SHM/journal siblings.
5. Call `sealRemaining()` on the native serial migration executor. Failure is
   explicit, with a fixed redacted message, and must not fall back to plaintext.
6. `close()` clears this helper's RAM master. It does not remove the envelope or
   encrypted evidence. Opening a fresh helper uses the same Keystore envelope.

Fixed roots and format
----------------------

The production factory uses only app `app_flutter`, `shared_prefs`, cache,
external-files and external-cache roots. It never creates an old `app_flutter`
through `Context.getDir`, touches public Downloads, traverses symlinks, or accepts
paths supplied by Dart. Platform root aliases are resolved once at the trusted
Context boundary; descendants must remain canonical. The new `puty-local`,
`no_backup`, `legacy-quarantine` and `puty-local-isolation` subtrees are excluded.

The internal directory is `no_backup/puty-local/legacy-quarantine`. Keystore alias
is `puty.legacy.quarantine.v1`; `master.wrap` uses PUTYOS1 with AAD
`master\0device-legacy`. No account master is reused. Android below API23 fails
explicitly because this feature requires OS-backed AES-GCM.

The shared Java/Go format is:

    objects/<SHA256(UTF8 exact declared source path))>/<payloadSHA>.data
    objects/<SHA256(UTF8 exact declared source path))>/<payloadSHA>.meta

Both files are PUTYLS1, account `device-legacy`, objectID
`sourceSHA-payloadSHA`, types `legacy-file` / `legacy-metadata`. Encrypted metadata
contains `source`, `kind:file`, `account:""`, `size`, `sha256`, `createdAt`.
Versions at one path never overwrite one another. Metadata is the durable
per-source receipt; the encrypted `migration.state` is informational only.

The helper syncs each newly created parent directory, each ciphertext, both
renamed directory entries, and each exact source unlink. It authenticates every
chunk and EOF, verifies plaintext size/SHA, checks the receipt identity, and
rehashes the still-identical source before unlink. Changed/replaced files stay
untouched. Unknown files are never assigned to a user. Package-private recovery
additionally requires an explicit native attribution callback and full-object
verification before returning an InputStream. Each read rechecks attribution and
the helper's open state before and after copying bytes; there is no generic
backup route.

Verification
------------

Run `clients/flutter/scripts/test-mobile-legacy-quarantine.ps1`. It uses pinned
JDK17, Android35 stubs only at the platform boundary, a dedicated synthetic D:
scratch leaf, and pinned Go1.27.3 with offline modules. Eighteen production-core
tests cover chunk boundaries, tampering/truncation/context binding, old-version
preservation, DB exclusions, ENOSPC, sync/unlink faults, source replacement,
restart after partial progress, nonregular/link refusal and master loss. The real
Go codec then verifies Java data+metadata and Java verifies Go's outputs.

These tests do not claim Android Keystore or physical filesystem/device E2E.
In particular external FUSE directory-fsync behavior must be checked on-device;
the current helper does not silently ignore a durability error or delete an
unverified source.
