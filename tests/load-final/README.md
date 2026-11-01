# Final SDK load window

`start-final-load.py` runs only on `/srv/openim38-upgrade`. Upload it and this
directory's `launcher-lock.json`, then run from that root with root privileges:

```sh
python3 tests/start-final-load.py --check-only
python3 tests/start-final-load.py --run-id final-100-20261101a
```

The first command performs read-only ownership, fixed-loader and minimal-session
validation. The second additionally verifies all18 mounted/running business
binary hashes, starts only the previously inspected stopped load container, and
checks its existing system/NSS trust against the isolated public CA. It does not
install packages, replace certificates, restart business services, or create a
container. The host port7778 must be free.

The minimal private fixture must contain only users001–100, in order, each with
`userID`, `imToken` and `platformID:5`. Accounts101–110 are not used. No token is
printed. Existing barrier, exclusive-launch lock, workdir or evidence directory
causes refusal; preserve failed-run evidence before deliberately preparing any
new run. The tool never silently deletes a previous barrier.

Two real-WASM SDK groups50+50 must both report all50 connected and zero failures,
disconnects or runtime errors before the shared start file is created15seconds
in the future. Each group then holds for1800seconds, followed by the loader's
5second receive drain. The aggregate requires the full common30minute overlap,
each group's complete status, zero failures/disconnects/errors, and total sent
equal total received. Partition percentiles are retained separately.

All supervisor logs, copied original SDK reports, the shared barrier copy,
resource JSONL and business-binary before/after evidence go beneath
`tests/load-final/<run-id>/`. Raw loader workdirs are new
`load/<run-id>/linux50a` and `linux50b`; old `tests/load`, old loader logs and
old summary files are untouched. The exact owned load container is stopped at
the end, with report-copy failures unable to skip cleanup. It is never removed.

This preparation and the offline fixture tests are not a completed load result.
Only the subsequent run's `summary.json` and `run.json` determine acceptance.

After the root agent downloads the complete final evidence, independently
recompute it locally with:

```sh
python tests/summarize-final-load.py tests/load-final/final-100-20261101a
```

This writes `analysis.json` and `resource-summary.json` in that run only; the
original coordinator/partition summaries remain unchanged. It rechecks the
common window and zero-disconnection requirement, before/after final executable
and loader hashes, and separates all24 business containers from the generator.
Resource coverage/gaps are reported explicitly. Do not average the two partition
percentiles or treat synthetic SDK messages as media-capacity/device evidence.

For the fixed `final-100-20261101a` run, the root agent may run
`python3 tests/collect-final-load.py` on the server **after** the coordinator
reaches `complete` or `failed`. It reads only13 explicitly named result files,
requires all of them, rejects symlinks/escapes/non-ordinary or changing files,
and rejects private-key PEM, signed-token material and JSON credential fields.
It does not read `.private` or recurse into `tests`. A successful publication is
the suxin-readable `/tmp/openim38-final-load-results.tar.gz`, with a fixed run
directory prefix and a printed archive/member hash list. An existing archive
is refused rather than overwritten. Download to a cache, validate the exact
member allowlist and ordinary file types before extracting into `tests/load-final`,
then use the independent summarizer above. An early failed run with missing
required reports is refused and needs explicit diagnostic review.
