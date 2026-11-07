# Isolated account-security acceptance (operator run only)

This preparation has not created accounts, logged in, or contacted production.
Run it only after the HRM and OpenIM security candidates are deployed and the
operator has normally authenticated a fresh HRM administrator session.

Use the existing pinned Python/cryptography environment. Start with:

```powershell
python tools/production_security_acceptance.py dry-run
```

The default action is also `dry-run`: no private files are read and no request
is issued. Runtime state, random test passwords, opaque HRM/SSO sessions,
captcha images and evidence are restricted to
`.private/production/acceptance/security-20261104/`. Nothing overwrites the
shared `hrm-session.json`, any old fixture source, or old acceptance report.

The live command is intentionally separate:

```powershell
python tools/production_security_acceptance.py run --zone lan
```

It creates only `qa_261104_sec_i` and `qa_261104_sec_j`, plus two empty-ACL
policy roles. Every host/employee record and reply-lost recovery is bound to
a random prewritten run nonce, exact username/name/phone/UID/employee ID,
empty WeCom ID, and all six older fixture identities remaining disjoint.
Department 53 is read-only: its full expected identity, business scope,
OFF group switch and zero leader must match. No departments, groups,
messages, attachments, global network rules or global lock-policy settings
are changed.

Captchas are generated at the normal public endpoint when needed. The command
prints only the synthetic image path and phase name. Read that image normally
and enter its answer at the hidden prompt. It never guesses answers or
switches captcha off. Each answer is used once. Five wrong-password attempts
must finish within the configured ten-minute rolling window. The script reads
and asserts 600/5/1800, rather than changing the global policy.

Covered checks include:

- New account/reset login returns HTTP 428 and only an expiring change ticket;
  status remains restricted, a ticket cannot refresh, and replay after change
  fails. Password changes return no full session and require a fresh login.
- Configured-role allow union and personal inherit/allow/deny precedence for
  all ten zone/client cells, including preview allowed while save is denied.
- Captcha failures do not count as password failures; five real wrong
  passwords lock the fixture for thirty minutes and advance its security
  epoch. Admin unlock clears the rolling window but does not revive old
  sessions.
- Self-change checks the old password; independent admin reset forces another
  change. Existing HRM, web and PC SSO/IM session access is rejected after the
  relevant epoch changes.
- All five logical clients reject login when denied, then return policy and
  live `/enterprise/v1/session/state` matching the current source zone and
  login/save/epoch. Another test account is never altered by these mutations.

`--zone internet` selects both official public origins and requires the
server-reported source zone to actually be `internet`. A LAN workstation using
the public hostname is not an external-vantage test. A run covers its actual
zone; the admin matrix covers both zones. The report explicitly leaves
`resourceByteStreamTested=false`. Root performs GET/HEAD/Range resource checks
separately using the existing exact synthetic-object inventory/CAS workflow.
For the separate root-operated media checks, add `--operator-resource-pause`.
After security/state checks it normally authenticates two current synthetic I
sessions: `resource-preview-web` has preview but no save; `resource-save-pc`
allows save. Their proof remains only in the isolated private state. The runner
prints no tokens and waits for the operator to finish the existing exact
object inventory/CAS workflow, then continues its mandatory `finally` cleanup.
This optional pause itself creates no attachment and does not claim that
resource byte checks ran. Ctrl+C still enters cleanup.

A completed or partially used namespace is never recreated automatically.

Before creating a fixture, live preflight checks the new lock-policy and
restricted password/session routes, refusing an old or partial deployment.

The `run` command always attempts exact fixture quiescence in `finally`:
personal matrix and legacy five flags closed; accounts disabled; employees
inactive; only the two nonce-bound empty roles deleted after complete
membership/no-child checks; host accounts soft-deleted; disabled projections
confirmed. Reply-lost creation is recovered only by the exact random nonce.
A mismatched or partial record never produces a successful cleanup claim.
If the process is interrupted, inspect its private state before invoking:

```powershell
python tools/production_security_acceptance.py quiesce --zone lan
```

The runner never performs SQL employee deletion. It writes
`fixture-cleanup-plan.private.json` and `fixture-cleanup-progress.private.json`
in the isolated directory. The generic reviewed `fixtures_cleanup.py` worker
accepts their exact two-row predicates; `acceptanceNamespace` is supplementary
provenance and does not change its existing owner/instance/FK/business/outbox
checks. Upload only that private plan, review its SHA, and use the existing
root-operated `inspect` then `delete-employees --fixtures-released` workflow.
Retain the worker's successful stdout JSON as the isolated
`sql-cleanup-receipt.json`. Finally:

```powershell
python tools/production_security_acceptance.py verify-cleanup --zone lan --sql-receipt .private/production/acceptance/security-20261104/sql-cleanup-receipt.json
```

Verification requires the receipt to bind the current plan SHA, exactly two
deleted employee rows and zero business/FK references, then confirms normal
API absence plus disabled projections. Host soft-delete rows, epochs,
revocation tombstones, all audits and outbox history remain. Three exact
`gf_account_security`, `gf_account_failure` and `gf_account_change_ticket`
tables are retained authentication state. Similar names/business tables and
foreign keys are still counted and can block deletion.

Offline validation:

```powershell
python -m unittest tools/production_security_acceptance_test.py tools/production_fixture_cleanup_test.py tools/production_organization_fixture_cleanup_test.py tools/production_attachment_fixture_cleanup_test.py -q
```
