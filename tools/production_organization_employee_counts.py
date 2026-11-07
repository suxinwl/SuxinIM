"""Count real employees excluding exact old A/B and organization C/D fixtures.

Default is saved private snapshots. --fetch makes only three authenticated GETs.
Neither this wrapper nor imports alter the legacy counter or its evidence.
"""
import argparse
import collections
import copy
import datetime
import hashlib
import importlib.util
import json
import pathlib
import secrets
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
PRIVATE = ROOT / '.private/production/acceptance'
OLD_PLAN = PRIVATE / 'fixture-cleanup-plan.private.json'
ORG_PLAN = PRIVATE / 'organization-20261103-cleanup-plan.private.json'
ATTACHMENT_PLAN = PRIVATE / 'attachment-20261103/cleanup-plan.private.json'
HISTORICAL_PROOF_SHA256 = 'bb71e6ad44b914a310947d923e71630d4436705cdd4a0121ef7b0b832e3ed01d'
HISTORICAL_INPUT = PRIVATE / 'organization-count-probe.private.json'
HISTORICAL_BASELINE = PRIVATE / 'final-counts-after-cleanup-20261102T201807Z-6fcd07/hrmEmployees.private.json'
HISTORICAL_CAPTURE = PRIVATE / 'organization-counts-diagnostic-20261103T095940Z-617feb/imProjections.private.json'
_spec = importlib.util.spec_from_file_location('organization_legacy_counts', ROOT / 'tools/production_employee_counts.py')
base = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(base)
require = base.require
read = base.read


def fixture_bindings(old, organization):
    require(old.get('format') == organization.get('format') == 'puty-fixture-cleanup-v1'
            and old.get('namespace') is None and organization.get('namespace') == 'organization-20261103'
            and organization.get('departmentID') == 53
            and old.get('instanceID') == organization.get('instanceID') and old.get('instanceID'),
            'Two exact independently scoped fixture plans required')
    previous = [row['expected'] for row in old.get('fixtures', [])]
    current = [row['expected'] for row in organization.get('fixtures', [])]
    require(len(previous) == len(current) == 2
            and len({row['uid'] for row in previous}) == len({row['employeeID'] for row in previous}) == 2
            and {row['uid']: row['username'] for row in current} == {306: 'qa_261103_c', 307: 'qa_261103_d'}
            and len({row['employeeID'] for row in current}) == 2,
            'Exact A/B and C/D pairs required')
    for row in previous + current:
        require(type(row['uid']) is int and row['uid'] > 1 and row.get('employeeID') and row.get('name'),
                'Exact nonadministrator fixture binding required')
    old_uids, old_ids = {r['uid'] for r in previous}, {r['employeeID'] for r in previous}
    require(not old_uids.intersection({306, 307}) and not old_ids.intersection({r['employeeID'] for r in current})
            and set(organization.get('excludedOriginalUIDs', [])) == old_uids
            and set(organization.get('excludedOriginalEmployeeIDs', [])) == old_ids,
            'New and original fixture scopes must remain disjoint')
    require(all(r.get('departmentID') == 53 and isinstance(r.get('mobile'), str)
                and isinstance(r.get('hostMobile'), str) for r in current),
            'Exact original C/D mobile and department bindings required')
    return previous, current


def load_historical_proof(path, digest):
    require(path.resolve().is_relative_to(PRIVATE.resolve()) and not path.is_symlink()
            and digest == HISTORICAL_PROOF_SHA256 == hashlib.sha256(path.read_bytes()).hexdigest(),
            'Only the independently reviewed exact SQL proof is permitted')
    proof, request = read(path), read(HISTORICAL_INPUT)
    require(proof['inputSHA256'] == hashlib.sha256(HISTORICAL_INPUT.read_bytes()).hexdigest()
            and proof['baselineResponseSHA256'] == request['baselineResponseSHA256'] == hashlib.sha256(HISTORICAL_BASELINE.read_bytes()).hexdigest()
            and proof['captureResponseSHA256'] == request['captureResponseSHA256'] == hashlib.sha256(HISTORICAL_CAPTURE.read_bytes()).hexdigest(),
            'Historical proof, baseline, capture and exact private input must remain bound')
    baseline, captured = read(HISTORICAL_BASELINE), read(HISTORICAL_CAPTURE)
    require(baseline.get('code') == captured.get('code') == 0, 'Historical evidence response unavailable')
    original = request['baselineEmployee']
    baseline_rows = [r for r in baseline['data'] if r['id'] == original['id']]
    captured_rows = [r for r in captured['data'] if r['id'] == original['id']]
    require(len(baseline_rows) == len(captured_rows) == 1
            and {k: baseline_rows[0].get(k) for k in original} == original
            and proof['privateEvidence']['baselineEmployee'] == original,
            'Exact old baseline employee binding changed')
    return {'proof': proof, 'request': request, 'baseline': baseline['data'], 'captured': captured_rows[0],
            'proofSHA256': digest}


def historical_identity(evidence, employees, projections, fixture_ids, owner):
    proof, request = evidence['proof'], evidence['request']
    safe, detail = proof['safe'], proof['privateEvidence']
    original, current, sql = detail['baselineEmployee'], detail['currentEmployee'], detail['retainedProjection']
    identity = original['id']
    require(proof.get('format') == 'puty-organization-count-proof-v1' and proof.get('instanceID') == owner
            and identity not in fixture_ids and original == request['baselineEmployee']
            and original['status'] == 'active' and not original.get('employee_no')
            and original.get('wecom_userid') and int(original.get('user_id') or 0) == 0,
            'One separately audited real historical identity required')
    require(safe.get('historicalClassificationProven') is True and safe.get('sourcePhysicallyAbsent') is False
            and safe.get('businessDataModified') is False and safe.get('employeeRowCount') == 1
            and safe.get('employeeStatusCategory') == 'filtered' and safe.get('excludedByStatusFiltered') is True
            and safe.get('baselineWeComUIDAndNoHostUIDAndEmployeeNumberMatch') is True
            and safe.get('employeeNameMatchesBaseline') is True
            and safe.get('retainedProjectionStrictlyBoundDisabledAndNeverApplied') is True
            and safe.get('excludedFromCurrentHRMViewByExactSourceRules') is True
            and safe.get('excludedByExplicitRules') is True
            and safe.get('explicitRuleHits', {}).get('explicitUserName') is True
            and current['status'] == 'filtered'
            and all(current.get(k) == original.get(k) for k in ('id', 'name', 'wecom_userid'))
            and int(current.get('user_id') or 0) == int(original.get('user_id') or 0) == 0
            and str(current.get('employee_no') or '') == str(original.get('employee_no') or '') == '',
            'Historical source is not the proven filtered baseline mapping')
    rows = [row for row in projections if row['id'] == identity]
    require(len(rows) == 1 and identity not in {r['id'] for r in employees},
            'Proven historical identity must be absent only from the current HRM view')
    row, captured = rows[0], evidence['captured']
    p = row['projection']
    require(row == captured and row['status'] == sql['status'] == 'waiting' and not row.get('error')
            and row['desiredVersion'] == sql['desiredVersion'] and row['appliedVersion'] == sql['appliedVersion'] == 0
            and p.get('enabled') is False and sql['enabled'] == sql['provisioned'] == 0
            and p.get('employeeID') == sql['employeeID'] == identity
            and p.get('userID') == sql['nativeUserID'] == base.im_id(identity)
            and int(p.get('hostUID') or 0) == int(sql.get('uid') or 0) == 0 and not p.get('account')
            and hashlib.sha256(p['reason'].encode()).hexdigest() == request['reasonSHA256'],
            'Historical projection changed or is no longer strictly disabled and unapplied')
    return identity


def validate_attachment_scope(attachment_plan, owner):
    spec = importlib.util.spec_from_file_location('counts_attachment_scope_validator', ROOT / 'deploy/production/fixtures_cleanup.py')
    validator = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(validator)
    require(attachment_plan.get('namespace') == 'attachment-20261103', 'Explicit exact attachment fixture plan required')
    return validator.validate_plan(attachment_plan, owner)


def attachment_summary(responses, old_plan, organization_plan, attachment_plan, stage, historical):
    previous, organization = fixture_bindings(old_plan, organization_plan)
    current = validate_attachment_scope(attachment_plan, old_plan['instanceID'])
    require(set(attachment_plan['excludedOriginalUIDs']) == {r['uid'] for r in previous + organization}
            and set(attachment_plan['excludedOriginalEmployeeIDs']) == {r['employeeID'] for r in previous + organization},
            'All four earlier QA identities must be independently bound and excluded')
    for reply in responses.values():
        require(isinstance(reply, dict) and reply.get('code') == 0, 'Normal API response unavailable')
    employees, projections = responses['hrmEmployees']['data'], responses['imProjections']['data']
    require(responses['hrmSummary']['data'].get('employees') == len(employees)
            and len(employees) < 500 and len({r['id'] for r in employees}) == len(employees)
            and len({r['id'] for r in projections}) == len(projections), 'Complete original lists must agree')
    expected = {r['employeeID']: r for r in current}
    uids, im_ids = {r['uid'] for r in current}, {base.im_id(r['employeeID']) for r in current}
    selected_employees, selected_projections = [], []
    for row in employees:
        if row['id'] not in expected:
            require(int(row.get('user_id') or 0) not in uids, 'E/F UID appeared under another employee')
            continue
        fixture = expected[row['id']]
        require(int(row.get('user_id') or 0) == fixture['uid']
                and row.get('employee_no') == row.get('host_username') == fixture['username']
                and row.get('name') == fixture['name'] and not row.get('wecom_userid')
                and str(row.get('mobile') or '') == fixture['mobile'] and int(row.get('dept_id', 0)) == 53,
                'Exact E/F employee binding changed')
        selected_employees.append(row)
    for row in projections:
        p = row['projection']
        if row['id'] not in expected:
            require(int(p.get('hostUID') or 0) not in uids and p.get('userID') not in im_ids,
                    'E/F projection appeared under another identity')
            continue
        fixture = expected[row['id']]
        require(p.get('employeeID') == fixture['employeeID'] and int(p.get('hostUID') or 0) == fixture['uid']
                and p.get('account') == fixture['username'] and p.get('userID') == base.im_id(fixture['employeeID']),
                'Exact retained E/F projection binding changed')
        selected_projections.append(row)
    require(len(selected_projections) == 2, 'Both retained E/F projections must remain identifiable')
    if stage == 'after-cleanup':
        require(not selected_employees and all(r['projection'].get('enabled') is False
                and r['desiredVersion'] == r['appliedVersion'] and not r.get('error') for r in selected_projections),
                'E/F source rows remain or disabled projections have not converged')
    else:
        require(stage == 'before-cleanup' and len(selected_employees) == 2, 'Both exact E/F source rows required before cleanup')
    working = copy.deepcopy(responses)
    working['hrmEmployees']['data'] = [r for r in employees if r['id'] not in expected]
    working['imProjections']['data'] = [r for r in projections if r['id'] not in expected]
    working['hrmSummary']['data']['employees'] = len(working['hrmEmployees']['data'])
    # C/D and A/B have already completed cleanup in either E/F phase. Their
    # existing strict validator and historical-source proof remain unchanged.
    report = summarize(working, old_plan, organization_plan, 'after-cleanup', historical)
    qa_ids = {r['employeeID'] for r in previous + organization + current}
    all_qa = [r for r in projections if r['id'] in qa_ids]
    require(len(all_qa) == 6, 'Exactly six separately bound retained QA projections required')
    report['phase'] = stage
    report['HRMEmployees'].update(currentTotal=len(employees), syntheticCount=len(selected_employees),
        syntheticByCohort={'originalAB': 0, 'organizationCD': 0, 'attachmentEF': len(selected_employees)})
    report['IMProjections'].update(total=len(projections), allStates=dict(collections.Counter(r['status'] for r in projections)),
        syntheticCount=6, syntheticStates=dict(collections.Counter(r['status'] for r in all_qa)),
        syntheticDisabledCount=sum(r['projection'].get('enabled') is False for r in all_qa),
        retainedQAByCohort={'originalAB': 2, 'organizationCD': 2, 'attachmentEF': 2})
    report['fixtureNamespaces'].append('attachment-20261103')
    report['sixQAScopesExactlyBoundAndExcluded'] = True
    report['postCleanupInterpretation'] += ' Exact attachment E/F identities are separately excluded; six retained QA projections are never real employees.'
    return report


def summarize(responses, old_plan, organization_plan, stage, historical=None, attachment_plan=None):
    require(stage in ('before-cleanup', 'after-cleanup'), 'Explicit cleanup stage required')
    if attachment_plan is not None:
        return attachment_summary(responses, old_plan, organization_plan, attachment_plan, stage, historical)
    previous, current = fixture_bindings(old_plan, organization_plan)
    for reply in responses.values():
        require(isinstance(reply, dict) and reply.get('code') == 0, 'Normal API response unavailable')
    employees, projections = responses['hrmEmployees']['data'], responses['imProjections']['data']
    summary = responses['hrmSummary']['data']
    require(isinstance(employees, list) and isinstance(projections, list) and len(employees) < 500
            and summary.get('employees') == len(employees), 'Complete original HRM list and summary must agree')
    require(len({r['id'] for r in employees}) == len(employees)
            and len({r['id'] for r in projections}) == len(projections), 'Duplicate response identities')
    expected = {r['employeeID']: r for r in current}
    current_uids, current_im = {r['uid'] for r in current}, {base.im_id(r['employeeID']) for r in current}
    current_employees, current_projections = [], []
    for row in employees:
        if row['id'] not in expected:
            require(int(row.get('user_id') or 0) not in current_uids, 'C/D UID appeared under another employee')
            continue
        fixture = expected[row['id']]
        require(int(row.get('user_id') or 0) == fixture['uid']
                and row.get('employee_no') == row.get('host_username') == fixture['username']
                and row.get('name') == fixture['name'] and not row.get('wecom_userid')
                and str(row.get('mobile') or '') == fixture['mobile'] and int(row.get('dept_id', 0)) == 53,
                'Exact C/D employee binding changed')
        current_employees.append(row)
    for row in projections:
        p = row['projection']
        if row['id'] not in expected:
            require(int(p.get('hostUID') or 0) not in current_uids and p.get('userID') not in current_im,
                    'C/D projection appeared under another identity')
            continue
        fixture = expected[row['id']]
        require(p.get('employeeID') == fixture['employeeID'] and int(p.get('hostUID') or 0) == fixture['uid']
                and p.get('account') == fixture['username'] and p.get('userID') == base.im_id(fixture['employeeID']),
                'Exact retained C/D projection binding changed')
        current_projections.append(row)
    require(len(current_projections) == 2, 'Both retained C/D projections must remain identifiable')
    if stage == 'after-cleanup':
        require(not current_employees, 'C/D employee rows remain; after-cleanup cannot run yet')
        require(all(r['projection'].get('enabled') is False for r in current_projections),
                'A retained C/D projection remains enabled')
    else:
        require(len(current_employees) == 2, 'Both C/D source rows required before cleanup')
    # Exclude the independently verified C/D pair in a private working view only.
    # Actual captured responses and their original totals are never overwritten.
    all_expected = {r['employeeID'] for r in previous + current}
    history = {historical_identity(historical, employees, projections, all_expected, old_plan['instanceID'])} if historical else set()
    filtered = copy.deepcopy(responses)
    filtered['hrmEmployees']['data'] = [r for r in employees if r['id'] not in expected]
    filtered['imProjections']['data'] = [r for r in projections if r['id'] not in expected and r['id'] not in history]
    filtered['hrmSummary']['data']['employees'] = len(filtered['hrmEmployees']['data'])
    # Original A/B cleanup is already complete in both new phases.
    report = base.summarize(filtered, old_plan, 'after-cleanup')
    all_qa = [r for r in projections if r['id'] in all_expected]
    require(len(all_qa) == 4, 'Exactly four retained QA projections required')
    report['phase'] = stage
    report['HRMEmployees'].update(currentTotal=len(employees), syntheticCount=len(current_employees),
        syntheticByCohort={'originalAB': 0, 'organizationCD': len(current_employees)})
    report['IMProjections'].update(total=len(projections),
        allStates=dict(collections.Counter(r['status'] for r in projections)),
        syntheticCount=4, syntheticStates=dict(collections.Counter(r['status'] for r in all_qa)),
        syntheticDisabledCount=sum(r['projection'].get('enabled') is False for r in all_qa),
        retainedQAByCohort={'originalAB': 2, 'organizationCD': 2})
    report['fixtureNamespaces'] = ['original-AB', 'organization-20261103']
    report['fourQAScopesExactlyBoundAndExcluded'] = True
    report['postCleanupInterpretation'] = 'All four retained QA projections are excluded from real employees. Old A/B rows are absent; C/D rows must also be absent before after-cleanup is accepted.'
    report['expectedProductionCountsMatch'] = (
        report['HRMEmployees']['realCount'] == 302 and report['HRMEmployees']['realStatus'] == {'active': 301, 'resigned': 1}
        and report['IMProjections']['realCount'] == 302 and report['IMProjections']['realStates'] == {'ready': 300, 'waiting': 2}
        and report['IMProjections']['realWaitingCategories'] ==
            {'missing_explicit_wecom_employee_number': 1, 'non_active_employee': 1})
    if historical:
        baseline = historical['baseline']
        expected_status = collections.Counter(r['status'] for r in baseline)
        expected_status['active'] -= 1
        expected_status = {k: v for k, v in expected_status.items() if v}
        report['original302BaselineCountsUnchanged'] = report['expectedProductionCountsMatch']
        report['HRMEmployees'].update(historicalBaselineRealCount=len(baseline),
            currentViewChangeFromBaseline=report['HRMEmployees']['realCount'] - len(baseline),
            filteredHistoricalSourceRowCount=1, physicallyDeletedHistoricalSourceRowCount=0)
        report['IMProjections'].update(realCountScope='current HRM view', historicalFilteredCount=1,
            historicalFilteredStates={'waiting': 1}, historicalFilteredDisabledCount=1,
            totalRealIncludingHistorical=report['IMProjections']['realCount'] + 1,
            allCurrentAndHistoricalRealProjectionIDsAudited=True)
        report['historicalExclusionAudit'] = {'count': 1, 'sourceStatus': 'filtered',
            'explicitUserNameRuleProven': True, 'sourcePhysicallyDeleted': False,
            'baselineMappingUnchanged': True, 'disabledUnprovisionedAndNeverApplied': True,
            'proofSHA256': historical['proofSHA256']}
        report['expectedProductionCountsMatch'] = (len(baseline) == 302
            and report['HRMEmployees']['realCount'] == len(baseline) - 1
            and report['HRMEmployees']['realStatus'] == expected_status
            and report['IMProjections']['totalRealIncludingHistorical'] == len(baseline)
            and report['IMProjections']['realStates'] == {'ready': 300, 'waiting': 1}
            and report['IMProjections']['realWaitingCategories'] == {'non_active_employee': 1})
        report['postCleanupInterpretation'] += ' One exact SQL-proven filtered real employee remains in storage and is excluded by current explicit rules: current real employees 301, historical baseline 302; its disabled waiting projection is audited separately. No real source row was deleted by this counter.'
    return report


def save_private(folder, key, value, protect):
    path = folder / (key + '.private.json')
    with path.open('x', encoding='utf-8') as output:
        output.write(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    protect(path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stage', choices=('before-cleanup', 'after-cleanup'), required=True)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--fetch', action='store_true')
    source.add_argument('--snapshots', type=pathlib.Path)
    parser.add_argument('--historical-proof', type=pathlib.Path)
    parser.add_argument('--historical-proof-sha256')
    parser.add_argument('--fixture-scope', choices=('organization-20261103', 'attachment-20261103'), default='organization-20261103')
    args = parser.parse_args()
    old, organization = read(OLD_PLAN), read(ORG_PLAN)
    fixture_bindings(old, organization)
    owner = read(PRIVATE.parent / 'openim/instance.json')
    require(owner.get('production') is True and owner.get('testOnly') is False
            and owner.get('instanceID') == old['instanceID'] == organization['instanceID'],
            'Private production ownership mismatch')
    require(bool(args.historical_proof) == bool(args.historical_proof_sha256), 'Historical proof path and reviewed digest are both required')
    historical = load_historical_proof(args.historical_proof, args.historical_proof_sha256) if args.historical_proof else None
    attachment = read(ATTACHMENT_PLAN) if args.fixture_scope == 'attachment-20261103' else None
    folder = None
    if args.fetch:
        import production_hrm_acceptance as hrm
        from production_backup import protect
        parent = PRIVATE / 'attachment-20261103' if attachment else PRIVATE
        folder = parent / (('attachment-counts-' if attachment else 'organization-counts-') + args.stage + '-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(3))
        folder.mkdir(mode=0o700)
        protect(folder)
        session = PRIVATE / 'hrm-session.json'
        session_before = hashlib.sha256(session.read_bytes()).hexdigest()
        responses = {}
        for key, path in base.PATHS.items():
            status, reply = hrm.request(path, method='GET')
            save_private(folder, key, reply, protect)
            require(status == 200, 'Normal HRM read-only API unavailable')
            responses[key] = reply
        require(hashlib.sha256(session.read_bytes()).hexdigest() == session_before, 'Shared session file changed')
    else:
        require(args.snapshots.resolve().is_relative_to(PRIVATE.resolve()), 'Private saved responses required')
        responses = {key: read(args.snapshots / (key + '.private.json')) for key in base.PATHS}
    try:
        report = summarize(responses, old, organization, args.stage, historical, attachment)
    except Exception:
        if folder:
            save_private(folder, 'failure', {'countReviewFailed': True, 'rawResponsesPreserved': True}, protect)
            raise RuntimeError('Count validation failed; original responses preserved in ' + str(folder)) from None
        raise
    report.update(capturedAtUTC=datetime.datetime.now(datetime.timezone.utc).isoformat(), freshNetworkRead=args.fetch,
                  originalFixturePlanSHA256=hashlib.sha256(OLD_PLAN.read_bytes()).hexdigest(),
                  organizationFixturePlanSHA256=hashlib.sha256(ORG_PLAN.read_bytes()).hexdigest())
    if attachment:
        report['attachmentFixturePlanSHA256'] = hashlib.sha256(ATTACHMENT_PLAN.read_bytes()).hexdigest()
    if args.fetch:
        save_private(folder, 'report', report, protect)
        print(json.dumps({'report': report, 'privateEvidenceDirectory': str(folder)}, ensure_ascii=True))
    else:
        print(json.dumps(report, ensure_ascii=True))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'countReviewFailed': True, 'category': type(error).__name__,
                          'message': str(error) if type(error) is RuntimeError else 'private_details_suppressed'}))
        sys.exit(1)
