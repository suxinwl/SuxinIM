# Production backup and isolated recovery check

These tools target only `/srv/puty-suite` and `puty-suite-prod`. They do not
implement an in-place production restore. Run the local operator from the 3.8
checkout on Windows; it prompts for SSH password without storing it. SSH uses
the pinned production host fingerprint, not automatic host-key acceptance.

## Dependencies and commands

The existing `.venv-tools` runtime has `cryptography` and `paramiko` installed.
Server requirements are Python 3.9+, Docker Compose v2, local Docker volumes,
and `/www/server/panel/pyenv/bin/supervisorctl`. No server encryption package or
recovery key is required. Commands below run from the 3.8 repository root.

```powershell
# Local only. Fails if a recovery key already exists; never silently rotates it.
.venv-tools\Scripts\python.exe tools/production_backup.py keygen

# Stops this suite's running writers/storage, then restores their original state.
# Do not run concurrently with deployment, provisioning, imports or sync jobs.
.venv-tools\Scripts\python.exe tools/production_backup.py create --max-stop-seconds 1800

# Local authenticated decryption + complete archive inventory check; no SSH.
.venv-tools\Scripts\python.exe tools/production_backup.py inspect --archive <absolute-.private-backup-path>

# Authenticates locally first, then restores into isolated server resources.
.venv-tools\Scripts\python.exe tools/production_backup.py restore-check --archive <absolute-.private-backup-path>

# Offline tests: no SSH or Docker access.
.venv-tools\Scripts\python.exe tools/production_backup_test.py -v
```

Global options such as `--key`, `--ssh-key`, `--host`, `--port` and `--fingerprint`
precede the command. Default SSH is `root@192.168.10.11:2223`. A changed host
fingerprint must first be verified independently by the operator.

## Coverage and consistency

The worker verifies the exact root, regular `openim/instance.json` ownership
marker, project, instance ID, every source container label and all six local
volume labels/mountpoints before stopping anything. Only original running
container IDs and the original running `puty-hrm` program are resumed; no absent
service is created. Source services must not be concurrently operated by another
operator during a backup.

`/srv/puty-suite/activation.json` is mandatory for an activated production
backup. Its `instanceID` must match the ownership marker, its data policy must
be `fresh-business-data`, and its timezone-qualified `startAt` must exactly
equal the HRM runtime's `GOSUXIN_SYNC_START_AT` with rehearsal disabled. This
check runs before stopping services and is repeated after acquiring the shared
maintenance lock. The original marker is archived as `runtime/activation.json`
and recorded in the encrypted manifest. Local inspection and isolated recovery
both verify the restored marker, owner and original cutoff together; missing or
divergent activation data fails closed. Recovery never generates a new cutoff.

Source filesystem metadata is checked before downtime, and the same checks
run again while capturing. The sole allowed transient socket exclusion is the
symlink `volumes/mysql-data/mysql.sock` with the exact target
`/var/run/mysqld/mysqld.sock`. Other absolute or escaping links still fail closed.

1. Stop HRM through its Supervisor program and every running non-storage suite
   container, including ingress APIs, workers and LiveKit.
2. Export `gosuxin_prod` with routines/events/triggers and each of the configured
   Mongo databases (`openim38`, `chat38`, `enterprise38`). Capture table/collection
   counts before and after, and require equality.
3. Stop MySQL, Mongo, Minio, Redis, Kafka and etcd. Stream their complete cold
   volume contents, the logical exports, complete `gosuxin` and `openim`
   directories, the original suite activation marker, exact HRM Supervisor configuration, Supervisor include config,
   and both PUTY Nginx configuration files. This includes uploads, Minio object
   bytes/metadata, HRM plugin data, WeCom credential key, bridge/JWT/session keys,
   IM secrets, certificates/private keys, frontend files and executable binaries.
   Only the known `openim/logs` and `gosuxin/runtime/log` trees are omitted. A
   business directory or object named `logs` remains included. The exact transient
   MySQL socket link described above is omitted; all persistent database data remains included.
4. Always resume original storage first (wait for its configured health checks),
   original application containers next and HRM last. An independent forked
   watchdog also attempts recovery if the SSH worker dies or the maximum stop
   interval expires. No script can recover services from host power loss or a
   broken Docker daemon; failed recovery leaves a restricted `recovery.json`
   descriptor under `/srv/puty-suite/.backup-stage/` for the operator.

The archive contains per-file SHA256, sizes, owner/permissions and an encrypted
manifest. Logical dump plaintext exists only in a mode-0700 temporary server
stage while capturing. The final gzip tar is streamed over SSH and encrypted on
the local operator machine without a final plaintext server archive. Failed or
interrupted captures never commit a `.putybak` file. Bounded service interruption
includes dump time, compression and network transfer: choose an adequate limit
and maintenance window for the current dataset size. The default is 30 minutes.
Gzip level 1 is used to reduce the service interruption spent compressing executables.

## Encryption and key custody

Each backup derives a unique AES-256 key from the independent recovery key and
a random 256-bit salt using HKDF-SHA256. Each 4 MiB chunk uses AES-GCM with a
distinct nonce; associated data authenticates the envelope, chunk index, length
and final-record flag. An authenticated final record binds total bytes, chunk
count and payload SHA256. Missing/reordered chunks, changed headers, truncation,
appended bytes, incorrect keys and ciphertext modifications fail verification.

The independent key is generated only at
`.private/production-backup-keys/recovery.key`; Windows ACLs (or POSIX mode 0600)
restrict it to the operator. Archives and public manifest/hash sidecars default
to the separate `.private/production-backups/` directory. The recovery key never
enters the server, SSH arguments, environment, archive, sidecar or log. Preserve
an additional encrypted/offline copy of the recovery key separately from backup
media; losing it makes the backups unrecoverable. The sidecar contains only safe
summary metadata and checksums; connection credentials remain inside encryption.

The local post-capture roundtrip/inspect/restore check uses a restricted temporary
plaintext archive under `.private/production-backup-scratch/`, removed on normal
completion or exceptions. Disk encryption and restricted operator accounts are
appropriate because abrupt machine termination can leave temporary plaintext.
Available local disk must hold the encrypted archive plus one plaintext payload.
Server staging needs logical-dump space; restore validation additionally needs
the full unpacked cold data plus disposable restored MySQL/Mongo databases.

## Isolated restore verification

The operator authenticates **every chunk and the final record locally** and checks
the complete file manifest before sending any plaintext to the remote worker.
The worker independently verifies the transfer SHA256 and every archive member,
rejecting path traversal, device files, hardlink tricks, duplicate entries,
escaping symlinks and children beneath symlinks.

Cold volumes and runtime files restore only beneath a fresh random private
directory. MySQL and Mongo logical dumps restore into new randomly named,
labelled Docker volumes and containers, using the exact captured local image
content IDs. These containers have `--network none`, no published port, fresh
random credentials and no production bind mount. Database row/document counts,
MySQL table checks and Mongo collection validation must pass. Every Minio/cold
volume/runtime file is restored and verified against its original hash. Key
presence is checked without printing the material. The HRM plugin, IM services,
WeCom synchronizer, Redis, Kafka and etcd are never started from restored data.

Database readiness requires PID 1 to be the final server process, followed by an
authenticated query. MySQL uses TCP `SELECT 1`; `mysqladmin ping` can succeed
during initialization or authentication failure and is not used. The captured
image does not provide `mysqlcheck`, so the available MySQL client runs `CHECK
TABLE` for the exact captured base-table set; every table must return one
`check/status/OK` row. Additional, missing, duplicate or non-OK rows fail closed.

Cleanup can remove only resources bearing both the unique restore prefix and
the matching `com.puty.backup.restore` label. A private JSON report records all
checks and cleanup status. The report must show `passed: true` and
`cleanupVerified: true`. This is a restore exercise, not a claim of completed
production disaster recovery; actual cutover requires a separate approved
procedure, service identity/session invalidation and external connectivity checks.

The tools have offline tests for encryption tampering, truncation, reordering,
wrong keys, archive roundtrip/hash mismatch, ownership/path guards and recovery
state, plus activation capture and missing/wrong-owner/cutoff/policy/timezone
rejection. On 2026-11-03 the first real encrypted backup captured 5,385 entries and
all six owned cold volumes. Its isolated restore passed all 71 MySQL base-table
checks, MySQL row counts, all three Mongo database document counts/collection
validation, cold-file/Minio hashes, original activation cutoff, and cleanup. The
report has `passed: true`, `cleanupVerified: true`, `productionModified: false`,
`network: none`, and no published port. Earlier failed capture/restore evidence
is retained separately. The final delivery archive and its matching restore
report after subsequent release/maintenance changes supersede this early test.

The final delivery backup is
`.private/production-backups/puty-20261102T202034Z-0dd1e269.putybak`,
1,279,659,504 bytes, SHA256
`e97b7ba38ec1cc9cf0792ae9f81b57344d50a7959b75d6a7bac400d32edd7c48`.
It captures 5,389 entries after the eight runtime keys were rotated, both final
installers/version records were published, and the exact QA cleanup completed.
Its matching `.restore-check.json` passed on 2026-11-03: all 71 MySQL base tables,
three Mongo databases, original activation, cold-volume/Minio hashes and runtime
key material. It used network `none` with no published ports; cleanup was
verified and production was not modified. After the cold capture resumed the
suite, normal captcha login and native HRM communication/group reads passed;
the final operator session was then logged out and rejected on reuse.

Runtime credentials rotated after an earlier backup must not be reused during
production recovery. Use the final archive and its recorded credential state;
an older archive requires a new scoped credential rotation before reopening
production access. Recovery encryption keys remain separate from archive media.
