# Security policy release operator contract

This independent worker coordinates exactly 17 OpenIM/Chat/enterprise application
binaries, the HRM `main`, and HRM Vue assets. It does not deploy client installers,
edit Nginx, issue login credentials, switch the active plugin, rotate keys, or
stop/recreate the six storage services or LiveKit.

The operator must keep the external maintenance window closed until the release
receipt and normal business acceptance finish. SIGTERM/HUP/INT invoke compensation;
SIGKILL or host loss requires the explicit `recover` command before reopening ingress.

## Prepare the backup proof locally

Use the actual encrypted archive, its original manifest, and the successful isolated
restore-check output. The archive remains on the operator's machine. The recovery key
must not be copied to the server or added to the receipt.

```text
python deploy/production/publish_security_policy.py prepare-receipt \
  --archive <absolute-private-archive.putybak> \
  --manifest <absolute-original-manifest.json> \
  --restore-check <absolute-successful-restore-check.json> \
  --out <new-absolute-private-receipt.json>
```

The tool hashes the actual archive bytes and requires the fixed instance, a full
stopped-writer/storage backup, matching archive ID/SHA/size, `passed=true`, and
`cleanupVerified=true`. The receipt records the archive, original manifest, and
restore-check hashes and verification time; it contains no recovery key. Both server
plan and apply reject a receipt older than 24 hours. Preserve all three original
inputs separately; do not substitute a manually authored receipt.

## Stage the exact candidates

Upload the worker and these reviewed helpers into the same root-owned staging
directory. The worker pins both helper hashes:

- `publish_hrm_web.py`: `bb7c96958559b3bc3a12cb2e60673db99ad6839f04c9e922b32c7eae12198a6a`
- `update_backend.py`: `a4f5847386a92c31143a4b184b4691c9bffd1eab9a8b81d2c15f95fc4fee4eca`

All request paths below are relative to `/srv/puty-suite/staging`, must identify
canonical regular root-owned files, and must have no group/world write permission.
Use `0600` for the request and receipt. Populate the hashes and sizes from the final
reviewed candidate manifests; do not use executable names as a substitute for hashes.

```json
{
  "format": "puty-security-release-1",
  "backend": {
    "openim-api": {"path": "security/openim-api", "sha256": "<64 lowercase hex>", "bytes": 1}
  },
  "main": {"path": "security/main", "sha256": "<64 lowercase hex>", "bytes": 1},
  "web": {"path": "security/hrm-web.zip", "sha256": "<64 lowercase hex>", "indexSHA256": "<64 lowercase hex>"},
  "backupReceipt": {"path": "security/backup-proof.json", "sha256": "<64 lowercase hex>"}
}
```

The `backend` object must contain all 17 keys, not just the one abbreviated example:

```text
openim-api openim-crontask openim-msggateway openim-msgtransfer openim-push
openim-rpc-auth openim-rpc-conversation openim-rpc-friend openim-rpc-group
openim-rpc-msg openim-rpc-third openim-rpc-user
admin-api admin-rpc chat-api chat-rpc enterprise-service
```

`admin-rpc` maps to Compose service `chat-admin`; `chat-rpc` maps to `chat-business`.
All other keys map to the same service name. The worker rejects a missing/extra key,
non-amd64 ELF, wrong file hash/size, changed reviewed helper, or invalid web archive.

## Read-only plan and guarded application

The operator supplies the exact SHA of the staged request JSON. The plan binds the
worker source SHA, all candidate hashes, existing binaries/config/index metadata,
backend manifest, 24 container IDs/images/owner labels, actual process executable
hashes/start times, six exact data volume identities/destinations, and every active
biz-suite package file. A changed state requires a new plan before any stop/write.

```text
python3 /srv/puty-suite/staging/publish_security_policy.py plan \
  --request security/request.json --request-sha256 <request-sha> \
  --out security/plan.json

python3 /srv/puty-suite/staging/publish_security_policy.py apply \
  --plan security/plan.json --plan-sha256 <returned-plan-sha>
```

Both commands require root and the existing exclusive suite maintenance lock. The
plan writes only its new private plan file; it does not stop services or change the
production release. Apply first creates a unique `backups/security-policy-<uuid>`
transaction, saves originals with SHA/owner/mode, and durably journals before stopping
writers. Apply changes only these configuration values:

- cron `retainChatRecords=0`, `fileExpireTime=0`, `deleteObjectType=[]`;
- enterprise aliases `mongo-data:/capacity/mongo:ro` and
  `minio-data:/capacity/minio:ro`;
- enterprise `STORAGE_CAPACITY_DATA_VERIFIED=false`.

No environment credentials, other volume definitions, storage services, network,
ports, Nginx, or plugin package bytes are replaced. The capacity flag remains false
until a separate root-only audit verifies the actual owned mount sources.

The worker stops HRM and the 17 application services, installs verified files,
publishes Vue with index last, and restarts only those applications. Commit requires
readiness, all 18 new process executable hashes, actual read-only capacity aliases,
unchanged seven storage/LiveKit instances, and unchanged active plugin bytes.

## Failure or interrupted recovery

```text
python3 /srv/puty-suite/staging/publish_security_policy.py recover \
  --transaction /srv/puty-suite/backups/security-policy-<exact-transaction-uuid>
```

Use the exact transaction created by this attempt. Recovery rejects terminal
committed/rolled-back journals and validates the stored plan SHA. Before restoring
any file it rechecks plugin bytes, data volumes, and the original storage/LiveKit
instances. It stops writers, restores original file hashes/owners/modes and Vue,
starts the old applications, and verifies their actual executable hashes. Failed
recovery attempts stop application writers again and leave `recoveryRequired`;
never manually resume half-restored applications.

The file rollback does not remove newly added HRM SQL schema. This release's
migrations are additive and old binaries ignore the new security tables/columns;
the complete verified backup remains the disaster-recovery fallback. If recovery
reports a changed volume, instance, plugin, or missing original, stop and investigate
that exact guard instead of weakening it.

## Local verification

```text
python -m unittest tools.production_security_publish_test -v
python -m py_compile deploy/production/publish_security_policy.py tools/production_security_publish_test.py
```

The tests use synthetic temporary files and mock Docker/Supervisor only. Passing
them proves local behavior, not a completed production deployment.

## Enable verified capacity reporting after the release

Stage `publish_capacity_verification.py` beside the pinned security publisher and
`audit_security_retention.py` (SHA256
`0f9113ac26a82f2ee6ea2eb6c2e29eeb8df3e2241d55ddc83e27ab7bfb95be64`).
The capacity worker first verifies all six owned volumes and the actual MongoDB
and MinIO read-only aliases. Its plan binds the 24 running container identities,
plugin, enterprise binary and effective configuration. It changes only the quoted
`STORAGE_CAPACITY_DATA_VERIFIED` value from `false` to `true`, retaining every other
Compose byte. Only `enterprise-service` is recreated; the other 23 containers must
retain their exact identities and images.

```text
python3 /srv/puty-suite/staging/publish_capacity_verification.py plan \
  --out security/capacity-plan.json
python3 /srv/puty-suite/staging/publish_capacity_verification.py apply \
  --plan security/capacity-plan.json --plan-sha256 <returned-plan-sha>
```

The worker backs up the original configuration with owner/mode metadata before
stopping the service. A readiness or verification failure restores that exact
configuration and verifies the old process and read-only mounts. After host loss,
use the exact returned `backups/capacity-verification-<uuid>` transaction:

```text
python3 /srv/puty-suite/staging/publish_capacity_verification.py recover \
  --transaction /srv/puty-suite/backups/capacity-verification-<exact-uuid>
```

Recovery also handles an interrupted recreate where only `enterprise-service`
is missing. Any missing or changed unrelated container, changed binary, plugin,
volume or third-party Compose edit is rejected before restoring bytes.
Any unfinished capacity transaction must be recovered before attempting another
suite release or the main security rollback. Completed transactions retain their
backup and terminal journal for the operator's audit.
Local fault tests: `python -m unittest tools.production_capacity_publish_test -v`.

## Keep a coordinated release private while testing

Only enter the maintenance window after the signed Android candidate is ready
for its real application tests. `security_maintenance.py` uses the pinned CORS
worker as its filesystem and Nginx dependency. It binds the reviewed current vhost
hash and the operator's actual `SSH_CONNECTION` peer; forwarded headers cannot
select the allowed address. Its `map $remote_addr` and four server-level 503
checks cover only public HRM/IM IP and domain entry points. Both private bridge
server blocks remain byte-identical and reachable under their existing ACLs.

```text
python3 /srv/puty-suite/staging/security_maintenance.py plan \
  --operator-ip <actual-operator-SSH-IP> --out security/maintenance-plan.json
python3 /srv/puty-suite/staging/security_maintenance.py close \
  --plan security/maintenance-plan.json --plan-sha256 <returned-plan-sha>
```

The plan is read-only. Closing creates a durable original-file and metadata
backup first, tests and reloads Nginx, and restores the exact original on failure.
Verify the operator can still reach both entries and an independent disallowed
source receives 503. This gate controls new HTTP requests: it does not prove
existing WebSockets or transfers have drained. Writer isolation is performed by
the suite publisher's explicit stops; backup consistency comes from the complete
stopped-writer backup, not from this gate.

While the gate is closed, publish the application backends and HRM, publish the
verified IM Web candidate, run normal administrator authentication and the new
synthetic-account acceptance, and test the real Android Main/picker/camera paths.
Publish both validated native packages and mandatory upgrade records together.
Complete exact fixture/object cleanup, retention and capacity checks before
opening the user entries. If an application release fails, recover its exact
transaction to a coherent tested state before opening the gate.

```text
python3 /srv/puty-suite/staging/security_maintenance.py open \
  --transaction /srv/puty-suite/backups/security-maintenance-<exact-uuid> \
  --plan-sha256 <original-plan-sha>
```

Open/recovery accepts only this transaction's original or gated bytes and exact
owner/mode; a third-party vhost edit is never overwritten. An interrupted prepared
transaction can use the same open command. An open test/reload failure attempts to
restore the gate and reports failure, rather than announcing a successful public
release. There is no timed automatic opening during a partial deployment.
Local fault tests: `python -m unittest tools.production_security_maintenance_test -v`.

## Explicit rollback after startup checks but before business acceptance

The suite publisher commits when process hashes and startup checks pass. Its
ordinary recovery command deliberately rejects a committed release. If the later
business acceptance fails while the maintenance gate is still closed, use the
separate `rollback_security_release.py` worker, staged beside the frozen publisher
and maintenance worker. Do this before enabling the separate capacity flag or
making another backend/HRM asset release.

```text
python3 /srv/puty-suite/staging/rollback_security_release.py plan \
  --transaction /srv/puty-suite/backups/security-policy-<exact-uuid> \
  --maintenance-transaction /srv/puty-suite/backups/security-maintenance-<exact-uuid> \
  --maintenance-plan-sha256 <maintenance-plan-sha> --out security/rollback-plan.json
python3 /srv/puty-suite/staging/rollback_security_release.py apply \
  --plan security/rollback-plan.json --plan-sha256 <returned-rollback-plan-sha>
```

The read-only plan checks all installed application bytes and owners/modes, their
actual processes, both configuration files, the backend manifest, every installed
HRM Vue member, original backups, the closed gate, container identities, plugin,
and owned data volumes. An external edit or subsequent deployment is rejected
before stopping anything. Apply repeats those checks, durably records the explicit
rollback intent, and calls the original publisher's tested file compensation. It
does not remove additive SQL schema, clear data volumes, modify the IM Web root,
or open the maintenance gate. Recover IM Web separately before reopening users.
For this release the retained previous `PUTY_IM-3.8.10-web.zip` was hashed and all
3,763 members were compared to the actual old production Web root before staging.
It can be republished with the frozen `publish_hrm_web.py --application im`
command, guarded by the exact current new index SHA, previous archive SHA and
previous index SHA. This creates its own static publication backup and retains
unreferenced assets. Preserve the exact previous archive and proof instead of
reconstructing a Web release from a mutable build directory.

If interrupted after `rollbackIntent` or `recoveryRequired`, continue with the
original `publish_security_policy.py recover --transaction <same-security-uuid>`
command while keeping the gate closed. Do not create a new committed rollback
plan for a partially restored transaction. A successful compensation rechecks the
gate's actual file and journal before reporting it remains closed. Fault tests:
`python -m unittest tools.production_committed_rollback_test -v`.
