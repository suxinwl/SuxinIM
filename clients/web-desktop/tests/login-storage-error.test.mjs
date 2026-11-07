import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'esbuild';
const output=await build({entryPoints:['src/pages/login/local-storage-error.ts'],bundle:true,write:false,format:'esm',platform:'node'});
const {isLocalStorageIPCFailure,localStorageFailureMessage,localStorageIPCMessage}=await import('data:text/javascript;base64,'+Buffer.from(output.outputFiles[0].text).toString('base64'));
const main=await build({entryPoints:['electron/main/localStore/legacyFailure.ts'],bundle:true,write:false,format:'esm',platform:'node'});
const {legacyMigrationFailure}=await import('data:text/javascript;base64,'+Buffer.from(main.outputFiles[0].text).toString('base64'));
const remote=message=>new Error("Error invoking remote method 'puty-store': Error: "+message);
test('only the native exact local-storage remote-method failure becomes a fixed recovery instruction',()=>{
  assert.equal(isLocalStorageIPCFailure(new Error("Error invoking remote method 'puty-store': Error: SYNTHETIC PATH"),true),true);
  assert.match(localStorageFailureMessage,/本地存储修复/);assert.doesNotMatch(localStorageFailureMessage,/puty-store|SYNTHETIC/);
  for(const error of [new Error('验证码不正确'),new Error('密码错误'),new Error('PASSWORD_CHANGE_REQUIRED'),new Error("Error invoking remote method 'other-channel': Error: synthetic"),new Error('server message mentioning puty-store'),"Error invoking remote method 'puty-store': synthetic",null])assert.equal(isLocalStorageIPCFailure(error,true),false);
  assert.equal(isLocalStorageIPCFailure(new Error("Error invoking remote method 'puty-store': synthetic"),false),false);
});

test('native migration stages contain only fixed enums and never serialize underlying paths or exception text',()=>{
  for(const phase of ['inventory','reader','seal','cleanup','import']){
    const failure=legacyMigrationFailure(phase,Object.assign(Error('SYNTHETIC SECRET E:/private/source.sqlite'),{code:'EBUSY'}));
    assert.equal(failure.message,`PUTY_LEGACY_${phase}_EBUSY`);
    const displayed=localStorageIPCMessage(remote(failure.message),true);
    assert.match(displayed,/旧版数据迁移未完成/);assert.match(displayed,/EBUSY/);
    assert.doesNotMatch(displayed,/SYNTHETIC|private|原文件未改/);
  }
  assert.equal(legacyMigrationFailure('cleanup',{code:'SYNTHETIC_SECRET'}).message,'PUTY_LEGACY_cleanup_UNKNOWN');
});

test('only exact approved native recovery messages or exact migration enums are displayed',()=>{
  for(const message of ['请选择一个空目录，避免与其他文件混用','请选择独立于旧版数据和系统密钥目录的存储位置','本机密钥无法解锁，请使用加密恢复包','已有档案缺少密钥，请先恢复密钥'])assert.equal(localStorageIPCMessage(remote(message),true),message);
  for(const message of ['请选择一个空目录，避免与其他文件混用 E:/private','PUTY_LEGACY_cleanup_EBUSY E:/private','PUTY_LEGACY_untrusted_EBUSY','PUTY_LEGACY_cleanup_SECRET','raw error E:/private'])assert.equal(localStorageIPCMessage(remote(message),true),localStorageFailureMessage);
});

test('SSO errors, other IPC channels, web errors and lookalike strings remain outside local storage recovery mapping',()=>{
  for(const error of [new Error('密码错误'),new Error('验证码错误'),new Error('PASSWORD_CHANGE_REQUIRED'),new Error("Error invoking remote method 'other': Error: PUTY_LEGACY_cleanup_EBUSY"),'PUTY_LEGACY_cleanup_EBUSY'])assert.equal(localStorageIPCMessage(error,true),undefined);
  assert.equal(localStorageIPCMessage(remote('PUTY_LEGACY_cleanup_EBUSY'),false),undefined);
});

const importSteps={
  OPEN_SNAPSHOT:'打开旧版备份快照',MATCH_ACCOUNT:'核对旧版账号',
  CHECK_DESTINATION:'检查本机档案目标',LOAD_SQL:'加载数据库运行组件',
  READ_DATABASE:'读取旧版数据库',VERIFY_DATABASE:'校验旧版数据库',
  SCAN_MESSAGES:'扫描旧版聊天记录',IMPORT_ATTACHMENT:'迁入旧版附件',
  PREPARE_MESSAGES:'整理待迁入消息',COMMIT_HISTORY:'写入历史记录',
  COMMIT_MARKER:'记录迁移完成标记',
};
const importReasons={
  ENOENT:'所需文件不存在',EACCES:'没有文件访问权限',EPERM:'系统拒绝文件操作',
  EBUSY:'文件正在被占用',ENOTEMPTY:'目标目录已有内容',EIO:'磁盘读写失败',
  ENOSPC:'磁盘空间不足',UNKNOWN:'未知错误',ACCOUNT_QUARANTINED:'旧版账号档案需要核对',
  ACCOUNT_IDENTITY_MISMATCH:'新旧账号身份不一致',BACKUP_INTEGRITY:'备份完整性校验未通过',
  SQLITE_INTEGRITY:'旧版数据库完整性校验未通过',SQLITE_LIMIT:'旧版数据库超过处理限制',
  SCHEMA_UNSUPPORTED:'旧版数据库结构暂不支持',VAULT_NOT_EMPTY:'目标档案已有历史记录',
  SOURCE_MARKER_MISMATCH:'旧版来源标记不一致',FILENAME_TOO_LONG:'附件文件名过长',
  AUTHORIZATION_CHANGED:'账号授权已变化',MEMORY_LIMIT:'可用内存不足',
  SQLITE_RUNTIME:'数据库运行组件不可用',
};

test('approved detailed import stages and reasons show readable diagnostics without unsafe recovery advice',()=>{
  for(const [step,label] of Object.entries(importSteps))for(const [reason,description] of Object.entries(importReasons)){
    const code=`PUTY_LEGACY_import_${step}_${reason}`;
    const displayed=localStorageIPCMessage(remote(code),true);
    assert.ok(displayed.includes(label),`${step}: readable stage missing`);
    assert.ok(displayed.includes(description),`${reason}: readable reason missing`);
    assert.ok(displayed.includes(code),'the original allowlisted diagnostic code must remain available');
    assert.match(displayed,/保留.*备份.*源文件.*密钥/);
    assert.doesNotMatch(displayed,/删除|空目录|跳过|重建|明文/);
  }
});

test('detailed import diagnostics reject unknown enums, secret suffixes and line-boundary lookalikes',()=>{
  const safe='PUTY_LEGACY_import_READ_DATABASE_SQLITE_RUNTIME';
  for(const value of [
    `${safe} E:/SYNTHETIC_PRIVATE/backup.sqlite`,`${safe}\nSYNTHETIC_SECRET=never-display`,
    `${safe}\n`,`${safe}\r\n`,`${safe}\0SYNTHETIC_SECRET`,
    'PUTY_LEGACY_import_READ_DATABASE_SYNTHETIC_SECRET',
    'PUTY_LEGACY_import_SYNTHETIC_SECRET_SQLITE_RUNTIME',
    'PUTY_LEGACY_import_read_database_SQLITE_RUNTIME',
    'PUTY_LEGACY_import_READ_DATABASE_SQLITE_RUNTIME_EACCES',
    'prefix_PUTY_LEGACY_import_READ_DATABASE_SQLITE_RUNTIME',
    'PUTY_LEGACY_import_UNKNOWN_READ_DATABASE_SQLITE_RUNTIME',
    'PUTY_LEGACY_reader_READ_DATABASE_SQLITE_RUNTIME',
    'PUTY_LEGACY_import_UNKNOWN\n',
  ])assert.equal(localStorageIPCMessage(remote(value),true),localStorageFailureMessage,value);
});

test('legacy import reason codes remain readable and detailed diagnostics stay native-channel-only',()=>{
  for(const reason of ['ENOENT','EACCES','EPERM','EBUSY','ENOTEMPTY','EIO','ENOSPC','UNKNOWN']){
    const displayed=localStorageIPCMessage(remote(`PUTY_LEGACY_import_${reason}`),true);
    assert.match(displayed,/迁入历史记录/);assert.ok(displayed.includes(reason));
    assert.match(displayed,/保留备份、剩余源文件及密钥/);
  }
  const code='PUTY_LEGACY_import_OPEN_SNAPSHOT_BACKUP_INTEGRITY';
  assert.equal(localStorageIPCMessage(remote(code),false),undefined);
  assert.equal(localStorageIPCMessage(new Error(code),true),undefined);
  assert.equal(localStorageIPCMessage(new Error(`Error invoking remote method 'other': Error: ${code}`),true),undefined);
});

const quarantineReasons={
  QUARANTINE_NO_CANDIDATE:'缺少可恢复的原始数据库',
  QUARANTINE_SCHEMA:'原始数据库结构不受支持',
  QUARANTINE_PAGES:'原始数据库页校验未通过',
  QUARANTINE_CONFLICT:'同账号旧库内容不一致，需要核对',
  QUARANTINE_IDENTITY:'原始数据库账号身份校验未通过',
  QUARANTINE_SQLITE:'原始数据库完整性校验未通过',
  QUARANTINE_CAPSULE:'恢复校验未通过',
  QUARANTINE_LIMIT:'原始数据库重检超过处理限制',
};

test('quarantine reinspection diagnostics distinguish missing source, conflicting bytes and failed validation',()=>{
  for(const [reason,description] of Object.entries(quarantineReasons)){
    const code=`PUTY_LEGACY_import_RESOLVE_QUARANTINE_${reason}`;
    const displayed=localStorageIPCMessage(remote(code),true);
    assert.match(displayed,/重检旧版隔离档案/);
    assert.ok(displayed.includes(description),reason);
    assert.ok(displayed.includes(code),'safe complete diagnostic code must be copyable');
    assert.match(displayed,/保留备份、剩余源文件及密钥/);
    assert.match(displayed,/管理员核对/);
    assert.doesNotMatch(displayed,/删除|删库|空目录|跳过|重建|重命名|改文件名|关闭旧版客户端后重试/);
  }
});

test('quarantine diagnostics reject appended secrets, malformed enums and untrusted channels',()=>{
  const code='PUTY_LEGACY_import_RESOLVE_QUARANTINE_QUARANTINE_CONFLICT';
  for(const value of [
    `${code} E:/SYNTHETIC_PRIVATE/1599.db`,`${code}\nTOKEN=SYNTHETIC_SECRET`,
    `${code}\r\n`,`${code}\0SYNTHETIC_SECRET`,
    `${code}_SYNTHETIC_SECRET`,
    'PUTY_LEGACY_import_RESOLVE_QUARANTINE_QUARANTINE_UNKNOWN',
    'PUTY_LEGACY_import_RESOLVE_QUARANTINE_SYNTHETIC_SECRET',
    'PUTY_LEGACY_import_RESOLVE_QUARANTINE_EXTRA_QUARANTINE_CONFLICT',
    'PUTY_LEGACY_import_resolve_quarantine_QUARANTINE_CONFLICT',
    'PUTY_LEGACY_import_RESOLVE_QUARANTINE_QUARANTINE_CONFLICT ',
  ])assert.equal(localStorageIPCMessage(remote(value),true),localStorageFailureMessage,value);
  assert.equal(localStorageIPCMessage(remote(code),false),undefined);
  assert.equal(localStorageIPCMessage(code,true),undefined);
  assert.equal(localStorageIPCMessage(new Error(code),true),undefined);
  assert.equal(localStorageIPCMessage(new Error(`Error invoking remote method 'other': Error: ${code}`),true),undefined);
});

const specificPageReasons={
  QUARANTINE_NAME:'旧数据库名称格式不兼容',
  QUARANTINE_KEYS:'原始数据库页索引无效',
  QUARANTINE_SIZE:'原始数据库大小无效或超限',
  QUARANTINE_BINARY:'原始数据库页格式不兼容',
  QUARANTINE_HEADER:'原始数据库头校验未通过',
  QUARANTINE_GEOMETRY:'原始数据库页大小不一致',
  QUARANTINE_MISSING_PAGE:'原始数据库缺少必要页',
  QUARANTINE_LENGTH:'原始数据库页长度不一致',
};
for(const [reason,description] of Object.entries(specificPageReasons))test(`specific quarantine diagnostic ${reason} is readable, exact and private`,()=>{
  const code=`PUTY_LEGACY_import_RESOLVE_QUARANTINE_${reason}`;
  const displayed=localStorageIPCMessage(remote(code),true);
  assert.match(displayed,/重检旧版隔离档案/);
  assert.ok(displayed.includes(description));
  assert.ok(displayed.includes(code));
  assert.match(displayed,/保留备份、剩余源文件及密钥/);
  assert.match(displayed,/管理员核对/);
  assert.doesNotMatch(displayed,/删除|删库|空目录|跳过|重命名|改文件名|明文/);
  for(const untrusted of [
    `${code} D:/SYNTHETIC_PRIVATE/key-file`,`${code}\nTOKEN_SYNTHETIC_PRIVATE`,
    `${code}\r\n`,`${code}\0TOKEN_SYNTHETIC_PRIVATE`,`${code}_SECRET`,
    `prefix_${code}`,code.toLowerCase(),
  ])assert.equal(localStorageIPCMessage(remote(untrusted),true),localStorageFailureMessage);
  assert.equal(localStorageIPCMessage(remote(code),false),undefined);
  assert.equal(localStorageIPCMessage(new Error(code),true),undefined);
  assert.equal(localStorageIPCMessage(new Error(`Error invoking remote method 'other': Error: ${code}`),true),undefined);
});
