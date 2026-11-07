// Remove only fixtures belonging to a named local browser test run.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { resolve, join } from "node:path";
import { createHash, randomBytes } from "node:crypto";
const root = resolve(import.meta.dirname, "..");
const require = createRequire(join(root, "web/package.json"));
const { parse } = require("yaml");
async function localSQL(sql) {
  const config = parse(
    await readFile(join(root, "manifest/config/config.yaml"), "utf8")
  ).database.default;
  assert.equal(
    config.name,
    "kaoqin_dev",
    "Cleanup is restricted to the new local development database"
  );
  assert.ok(["127.0.0.1", "localhost"].includes(config.host));
  assert.equal(config.prefix, "gf_");
  return execFileSync(
    process.env.MYSQL_CLI ||
      "C:/Program Files/MySQL/MySQL Server 8.0/bin/mysql.exe",
    [
      "--protocol=TCP",
      "--host=" + config.host,
      "--port=" + config.port,
      "--user=" + config.user,
      "--database=" + config.name,
      "--default-character-set=utf8mb4",
      "--batch",
      "--skip-column-names",
    ],
    {
      input: sql,
      encoding: "utf8",
      windowsHide: true,
      env: { ...process.env, MYSQL_PWD: String(config.pass) },
      stdio: ["pipe", "pipe", "pipe"],
    }
  );
}
export async function createTestAdmin(run) {
  assert.match(run, /^QA\d{13}$/);
  const username = run.toLowerCase() + "a",
    password = randomBytes(20).toString("hex"),
    salt = randomBytes(8).toString("hex");
  const md5 = (s) => createHash("md5").update(s).digest("hex");
  const uid = Number(
    (
      await localSQL(`START TRANSACTION;
INSERT INTO gf_admin(username,password,salt,name,nickname,email,mobile,avatar,tel,remark,status,dept_id,createtime,updatetime) VALUES('${username}','${md5(
        md5(password) + salt
      )}','${salt}','${username}','${username}','','','/resource/static/brand/logo.png?v=suxin1','','Local browser QA temporary account',0,1,UTC_TIMESTAMP(),UTC_TIMESTAMP());
SET @qa=LAST_INSERT_ID(); INSERT INTO gf_auth_role_access(uid,role_id) VALUES(@qa,1); COMMIT; SELECT @qa;`)
    ).trim()
  );
  assert.ok(uid > 1);
  return { username, password, uid };
}
// Isolated scope fixtures use only existing seeded permissions. No production
// role, department, user, or organization-sync setting is modified.
export async function createOrganizationScopeFixtures(run) {
  assert.match(run, /^QA\d{13}$/);
  const permissions = JSON.parse((await localSQL(`SELECT JSON_OBJECT(
    'gate',(SELECT id FROM gf_auth_rule WHERE permission='runtime.plugin.biz-suite.admin' AND type=2 AND status=0 LIMIT 1),
    'entry',(SELECT id FROM gf_auth_rule WHERE routename='runtime_plugin_biz-suite' AND type=1 AND status=0 LIMIT 1),
    'self',(SELECT id FROM gf_auth_rule WHERE permission='biz-suite.hrm.self' AND type=2 AND status=0 LIMIT 1),
    'department',(SELECT id FROM gf_auth_rule WHERE permission='biz-suite.hrm.department' AND type=2 AND status=0 LIMIT 1));`)).trim());
  assert.ok(Object.values(permissions).every((id) => Number.isSafeInteger(id) && id > 0), "Deploy HRM scope permissions before running browser QA");
  const fixtures = [
    { key: "self", name: run + "本人", department: "main", role: "self" },
    { key: "manager", name: run + "负责人", department: "main", role: "department" },
    { key: "child", name: run + "下级员工", department: "child" },
    { key: "outside", name: run + "外部门员工", department: "outside" },
    { key: "disabled", name: run + "停用账号员工", department: "main", role: "self", disabled: true },
  ].map((item) => ({ ...item, username: run.toLowerCase() + ({ self: "ss", manager: "sm", child: "sc", outside: "so", disabled: "sd" })[item.key],
    employeeId: run + "-scope-" + item.key, password: randomBytes(24).toString("base64url"), salt: randomBytes(8).toString("hex") }));
  assert.ok(fixtures.every((item) => item.username.length <= 20), "QA usernames must fit the host schema");
  const md5 = (value) => createHash("md5").update(value).digest("hex");
  const marker = run + " isolated scope fixture";
  const sql = [`START TRANSACTION;`,
    `SET @qa_scope_owner=(SELECT id FROM gf_admin WHERE username='${run.toLowerCase()}a');`];
  for (const [key, label, parent] of [["main", "权限主部门", "0"], ["child", "权限下级部门", "@qa_scope_main"], ["outside", "权限外部门", "0"]]) {
    sql.push(`INSERT INTO gf_auth_dept(name,pid,weigh,status,remark,account_id,createtime,updatetime) VALUES('${run + label}',${parent},0,0,'${marker}',@qa_scope_owner,UTC_TIMESTAMP(),UTC_TIMESTAMP()); SET @qa_scope_${key}=LAST_INSERT_ID();`);
  }
  for (const scope of ["self", "department"]) {
    const buttons = JSON.stringify([permissions.gate, permissions[scope]]), menu = JSON.stringify([permissions.entry]), rules = JSON.stringify([permissions.entry]);
    sql.push(`INSERT INTO gf_auth_role(pid,name,rules,menu,btns,status,data_access,remark,weigh,account_id,createtime) VALUES(1,'${run}scope-${scope}','${rules}','${menu}','${buttons}',0,0,'${marker}',0,@qa_scope_owner,UTC_TIMESTAMP()); SET @qa_scope_role_${scope}=LAST_INSERT_ID();`);
  }
  for (const item of fixtures) {
    sql.push(`INSERT INTO gf_admin(username,password,salt,name,nickname,email,mobile,avatar,tel,remark,status,dept_id,createtime,updatetime)
      VALUES('${item.username}','${md5(md5(item.password) + item.salt)}','${item.salt}','${item.name}','${item.name}','','','/resource/static/brand/logo.png?v=suxin1','','${marker}',${item.disabled ? 1 : 0},@qa_scope_${item.department},UTC_TIMESTAMP(),UTC_TIMESTAMP()); SET @qa_scope_user_${item.key}=LAST_INSERT_ID();`);
    if (item.role) sql.push(`INSERT INTO gf_auth_role_access(uid,role_id) VALUES(@qa_scope_user_${item.key},@qa_scope_role_${item.role});`);
    sql.push(`INSERT INTO biz_hrm_employee(id,user_id,employee_no,name,dept_id,status,created_at,updated_at) VALUES('${item.employeeId}',@qa_scope_user_${item.key},'${item.username}','${item.name}',@qa_scope_${item.department},'active',UTC_TIMESTAMP(),UTC_TIMESTAMP());`);
    sql.push(`INSERT INTO biz_hrm_attendance_clock(id,employee_id,clock_time,clock_type,status,source,address,raw_json,created_at,updated_at) VALUES('${item.employeeId}-clock','${item.employeeId}','2026-10-29 09:00:00','in','normal','manual','${run}权限测试地点','{}',UTC_TIMESTAMP(),UTC_TIMESTAMP());`);
    sql.push(`INSERT INTO biz_hrm_attendance_leave(id,employee_id,source,approval_status,leave_type,start_time,end_time,duration,reason,raw_json,created_at,updated_at) VALUES('${item.employeeId}-leave','${item.employeeId}','manual','approved','事假','2026-10-29 10:00:00','2026-10-29 11:00:00',1,'${run}权限测试事由','{}',UTC_TIMESTAMP(),UTC_TIMESTAMP());`);
  }
  sql.push(`COMMIT; SELECT JSON_OBJECT('main',@qa_scope_main,'child',@qa_scope_child,'outside',@qa_scope_outside);`);
  const departments = JSON.parse((await localSQL(sql.join("\n"))).trim());
  // Secrets are returned in memory only to the login helper, never persisted.
  return { departments, employees: fixtures.map(({ password, salt, ...item }) => item),
    self: { username: fixtures[0].username, password: fixtures[0].password },
    manager: { username: fixtures[1].username, password: fixtures[1].password },
    disabled: { username: fixtures[4].username, password: fixtures[4].password } };
}
export async function createEnterpriseIMFixtures(run) {
  assert.match(run, /^QA\d{13}$/);
  const marker = run + " enterprise IM fixture";
  const owner = run.toLowerCase() + "a", peer = run.toLowerCase();
  const fixtureUsers = [
    { key: "third", suffix: "ic", name: run + "同名同事", status: 0, role: true, department: "outside" },
    { key: "disabled", suffix: "id", name: run + "未启用同事", status: 1, role: true, department: "child" },
    { key: "noim", suffix: "in", name: run + "未授权同事", status: 0, role: false, department: "child" },
  ].map(item => ({ ...item, username: run.toLowerCase() + item.suffix, password: randomBytes(20).toString("hex"), salt: randomBytes(8).toString("hex") }));
  const md5 = value => createHash("md5").update(value).digest("hex");
  const sql = [`START TRANSACTION; SET @im_owner=(SELECT id FROM gf_admin WHERE username='${owner}' AND remark='Local browser QA temporary account'); SET @im_peer=(SELECT id FROM gf_admin WHERE username='${peer}' AND remark='Local browser acceptance fixture');`];
  for (const [key, name, parent] of [["main", "企业测试部门", "0"], ["child", "企业下级部门", "@im_main"], ["outside", "企业其他部门", "0"]]) {
    sql.push(`INSERT INTO gf_auth_dept(name,pid,weigh,status,remark,account_id,createtime,updatetime) VALUES('${run + name}',${parent},0,0,'${marker}',@im_owner,UTC_TIMESTAMP(),UTC_TIMESTAMP()); SET @im_${key}=LAST_INSERT_ID();`);
  }
  sql.push(`UPDATE gf_admin SET dept_id=@im_main WHERE id=@im_owner; UPDATE gf_admin SET dept_id=@im_child,name='${run}同名同事',nickname='${run}同名同事' WHERE id=@im_peer;`);
  for (const item of fixtureUsers) {
    sql.push(`INSERT INTO gf_admin(username,password,salt,name,nickname,email,mobile,avatar,tel,remark,status,dept_id,createtime,updatetime) VALUES('${item.username}','${md5(md5(item.password) + item.salt)}','${item.salt}','${item.name}','${item.name}','','','/resource/static/brand/logo.png?v=suxin1','','${marker}',${item.status},@im_${item.department},UTC_TIMESTAMP(),UTC_TIMESTAMP()); SET @im_${item.key}=LAST_INSERT_ID();`);
    if (item.role) sql.push(`INSERT INTO gf_auth_role_access(uid,role_id) VALUES(@im_${item.key},1);`);
  }
  const entries = [
    { key: "owner", username: owner, name: run + "企业管理员", department: "main" },
    // The earlier HRM fixture uses run as its employee number (case-insensitive
    // in MySQL), so give this separate colleague a distinct public number.
    { key: "peer", username: peer + "ip", name: run + "同名同事", department: "child" },
    ...fixtureUsers,
    { key: "unbound", username: run.toLowerCase() + "iu", name: run + "未关联同事", department: "child" },
  ];
  for (const item of entries) {
    sql.push(`INSERT INTO biz_hrm_employee(id,user_id,employee_no,name,dept_id,status,position,created_at,updated_at) VALUES('${run}-im-${item.key}',${item.key === "unbound" ? 0 : "@im_" + item.key},'${item.username}','${item.name}',@im_${item.department},'active','企业IM验收',UTC_TIMESTAMP(),UTC_TIMESTAMP());`);
  }
  sql.push(`COMMIT; SELECT JSON_OBJECT('main',@im_main,'child',@im_child,'outside',@im_outside,'owner',@im_owner,'peer',@im_peer,'third',@im_third,'disabled',@im_disabled,'noim',@im_noim);`);
  const ids = JSON.parse((await localSQL(sql.join("\n"))).trim());
  assert.ok(ids.owner && ids.peer && ids.third);
  return { ids, entries: entries.map(({ password, salt, ...item }) => ({ ...item, user_id: ids[item.key] || 0 })), third: { username: fixtureUsers[0].username, password: fixtureUsers[0].password } };
}

// Return booleans only: do not expose encrypted values or real configuration.
export async function inspectCredentialFixtureStorage(run) {
  assert.match(run, /^QA\d{13}$/);
  const contact = run + "-contact-secret", app = run + "-app-secret";
  const values = (await localSQL(`SELECT
  (SELECT COUNT(*)=2 AND SUM(scope IN ('contact','app') AND encrypted_secret LIKE 'v1:%' AND INSTR(encrypted_secret,'${contact}')=0 AND INSTR(encrypted_secret,'${app}')=0)=2 FROM wecom_credentials),
  NOT EXISTS(SELECT 1 FROM wecom_settings WHERE INSTR(settings_json,'${contact}')>0 OR INSTR(settings_json,'${app}')>0),
  NOT EXISTS(SELECT 1 FROM gf_operation_log WHERE INSTR(COALESCE(req_body,''),'${contact}')>0 OR INSTR(COALESCE(req_body,''),'${app}')>0 OR INSTR(COALESCE(resp_body,''),'${contact}')>0 OR INSTR(COALESCE(resp_body,''),'${app}')>0);`)).trim().split("\t");
  return { encryptedOnly: values[0] === "1", settingsNoPlaintext: values[1] === "1", auditNoPlaintext: values[2] === "1" };
}
export async function cleanupRun(run) {
  assert.match(run, /^QA\d{13}$/, "Expected exact browser test run ID");
  const sql = `SET NAMES utf8mb4 COLLATE utf8mb4_general_ci;
START TRANSACTION;
SET @e = (SELECT id FROM biz_hrm_employee WHERE employee_no='${run}');
SET @u = (SELECT id FROM gf_admin WHERE username='${run.toLowerCase()}');
SET @a = (SELECT id FROM gf_admin WHERE username='${run.toLowerCase()}a');
SET @g = (SELECT id FROM biz_im_group WHERE name='${run}测试群');
SET @c = (SELECT id FROM biz_im_channel WHERE name='${run}频道');
SET @ig = (SELECT id FROM biz_im_group WHERE name='${run}企业群');
SET @igbad = (SELECT id FROM biz_im_group WHERE name='${run}无效群');
SET @ic = (SELECT id FROM gf_admin WHERE username='${run.toLowerCase()}ic' AND remark='${run} enterprise IM fixture');
SET @id = (SELECT id FROM gf_admin WHERE username='${run.toLowerCase()}id' AND remark='${run} enterprise IM fixture');
SET @in = (SELECT id FROM gf_admin WHERE username='${run.toLowerCase()}in' AND remark='${run} enterprise IM fixture');
DELETE FROM biz_im_message WHERE sender_id IN (@ic,@id,@in) OR (conversation_type='private' AND target_id IN (CAST(@ic AS CHAR),CAST(@id AS CHAR),CAST(@in AS CHAR))) OR stream_id IN (CONCAT('group:',@ig),CONCAT('group:',@igbad));
DELETE FROM biz_im_conversation WHERE user_id IN (@ic,@id,@in) OR stream_id IN (CONCAT('group:',@ig),CONCAT('group:',@igbad));
DELETE FROM biz_im_group_member WHERE group_id IN (@ig,@igbad) OR user_id IN (@ic,@id,@in);
DELETE FROM biz_im_group_request WHERE group_id IN (@ig,@igbad) OR user_id IN (@ic,@id,@in);
DELETE FROM biz_im_group WHERE id IN (@ig,@igbad);
DELETE FROM biz_im_friend WHERE user_id IN (@ic,@id,@in) OR friend_user_id IN (@ic,@id,@in);
DELETE FROM biz_im_friend_request WHERE sender_id IN (@ic,@id,@in) OR receiver_id IN (@ic,@id,@in);
DELETE FROM gf_login_log WHERE uid IN (@ic,@id,@in);
DELETE FROM gf_operation_log WHERE uid IN (@ic,@id,@in);
DELETE FROM gf_auth_role_access WHERE uid IN (@ic,@id,@in);
DELETE FROM gf_admin WHERE id IN (@ic,@id,@in) AND remark='${run} enterprise IM fixture' AND id>1;
DELETE FROM biz_hrm_employee WHERE id IN (${["owner", "peer", "third", "disabled", "noim", "unbound"].map(key => `'${run}-im-${key}'`).join(",")});
DELETE FROM gf_auth_dept WHERE remark='${run} enterprise IM fixture' AND account_id=@a;
DELETE c FROM biz_hrm_attendance_clock c JOIN biz_hrm_employee e ON e.id=c.employee_id WHERE e.id IN (${["self", "manager", "child", "outside", "disabled"].map((kind) => `'${run}-scope-${kind}'`).join(",")});
DELETE l FROM biz_hrm_attendance_leave l JOIN biz_hrm_employee e ON e.id=l.employee_id WHERE e.id IN (${["self", "manager", "child", "outside", "disabled"].map((kind) => `'${run}-scope-${kind}'`).join(",")});
DELETE FROM biz_hrm_employee WHERE id IN (${["self", "manager", "child", "outside", "disabled"].map((kind) => `'${run}-scope-${kind}'`).join(",")});
DELETE l FROM gf_login_log l JOIN gf_admin u ON u.id=l.uid WHERE u.remark='${run} isolated scope fixture' OR u.id IN (@u,@a);
DELETE l FROM gf_operation_log l JOIN gf_admin u ON u.id=l.uid WHERE u.remark='${run} isolated scope fixture' OR u.id IN (@u,@a);
DELETE r FROM gf_auth_role_access r JOIN gf_admin u ON u.id=r.uid WHERE u.remark='${run} isolated scope fixture';
DELETE FROM gf_admin WHERE remark='${run} isolated scope fixture' AND username IN (${["ss", "sm", "sc", "so", "sd"].map((suffix) => `'${run.toLowerCase()}${suffix}'`).join(",")}) AND id>1;
DELETE FROM gf_auth_role WHERE account_id=@a AND (remark='${run} isolated scope fixture' OR name IN ('${run}网页角色','${run}网页角色已编辑')) AND id>1;
DELETE FROM gf_auth_dept WHERE remark='${run} isolated scope fixture' AND name IN ('${run}权限下级部门','${run}权限主部门','${run}权限外部门');
DELETE FROM biz_hrm_attendance_clock WHERE employee_id=@e;
DELETE FROM biz_hrm_attendance_leave WHERE employee_id=@e;
DELETE FROM biz_hrm_attendance_out WHERE employee_id=@e;
DELETE FROM biz_hrm_attendance_trip WHERE employee_id=@e;
DELETE FROM biz_hrm_attendance_group WHERE name='${run}班组';
DELETE FROM biz_hrm_attendance_holiday WHERE name='${run}调休日';
DELETE FROM biz_hrm_employee WHERE id=@e;
DELETE FROM wecom_approval_template WHERE template_id='${run}';
DELETE FROM biz_im_conversation WHERE user_id IN (@u,@a) OR stream_id=CONCAT('private:1:',@u) OR stream_id=CONCAT('private:',LEAST(@u,@a),':',GREATEST(@u,@a)) OR stream_id=CONCAT('group:',@g) OR stream_id=CONCAT('channel:',@c);
DELETE FROM biz_im_message WHERE sender_id IN (@u,@a) OR (conversation_type='private' AND target_id=CAST(@u AS CHAR)) OR (conversation_type='group' AND target_id=@g) OR (conversation_type='channel' AND target_id=@c);
DELETE FROM biz_im_group_member WHERE group_id=@g OR user_id=@u;
DELETE FROM biz_im_group_request WHERE group_id=@g OR user_id=@u;
DELETE FROM biz_im_channel_member WHERE channel_id=@c OR user_id=@u;
DELETE FROM biz_im_group WHERE id=@g;
DELETE FROM biz_im_channel WHERE id=@c;
DELETE FROM biz_im_friend WHERE user_id=@u OR friend_user_id=@u;
DELETE FROM biz_im_friend_request WHERE sender_id=@u OR receiver_id=@u;
DELETE FROM biz_im_face_pack WHERE name='${run}表情';
DELETE FROM biz_im_sensitive_word WHERE word='${run}敏感';
DELETE FROM gf_auth_role_access WHERE uid=@u;
DELETE FROM gf_admin WHERE id=@u AND id>1;
DELETE FROM gf_auth_role_access WHERE uid=@a;
DELETE FROM gf_admin WHERE id=@a AND id>1;
COMMIT;
SELECT
  (SELECT COUNT(*) FROM gf_admin WHERE remark='${run} isolated scope fixture' OR username IN ('${run.toLowerCase()}','${run.toLowerCase()}a')) +
  (SELECT COUNT(*) FROM gf_admin WHERE remark='${run} enterprise IM fixture') +
  (SELECT COUNT(*) FROM gf_auth_dept WHERE remark='${run} enterprise IM fixture') +
  (SELECT COUNT(*) FROM biz_hrm_employee WHERE id IN (${["owner", "peer", "third", "disabled", "noim", "unbound"].map(key => `'${run}-im-${key}'`).join(",")})) +
  (SELECT COUNT(*) FROM biz_im_group WHERE name IN ('${run}企业群','${run}无效群')) +
  (SELECT COUNT(*) FROM gf_auth_role WHERE account_id=@a AND (remark='${run} isolated scope fixture' OR name IN ('${run}网页角色','${run}网页角色已编辑'))) +
  (SELECT COUNT(*) FROM gf_auth_dept WHERE remark='${run} isolated scope fixture') +
  (SELECT COUNT(*) FROM biz_hrm_employee WHERE id IN (${["self", "manager", "child", "outside", "disabled"].map((kind) => `'${run}-scope-${kind}'`).join(",")}) OR employee_no='${run}') +
  (SELECT COUNT(*) FROM biz_hrm_attendance_clock WHERE id IN (${["self", "manager", "child", "outside", "disabled"].map((kind) => `'${run}-scope-${kind}-clock'`).join(",")})) +
  (SELECT COUNT(*) FROM biz_hrm_attendance_leave WHERE id IN (${["self", "manager", "child", "outside", "disabled"].map((kind) => `'${run}-scope-${kind}-leave'`).join(",")}));`;
  assert.equal(Number((await localSQL(sql)).trim()), 0, "QA organization, role and employee fixtures must be removed completely");
}
if (process.argv[1] && resolve(process.argv[1]) === import.meta.filename) {
  await cleanupRun(process.argv[2]);
  console.log("Cleaned " + process.argv[2] + " from kaoqin_dev");
}
