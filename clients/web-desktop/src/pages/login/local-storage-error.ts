export const localStorageFailureMessage='本地加密档案未能打开，请使用本地存储修复检查位置；现有密文已保留。';
export function isLocalStorageIPCFailure(error:unknown,native:boolean):boolean{
  return native && error instanceof Error && /^(?:Error: )?Error invoking remote method ['"]puty-store['"]:/.test(error.message);
}
const actionable=new Set([
  '请选择本地存储目录后继续','请选择一个空目录，避免与其他文件混用',
  '请选择独立于旧版数据和系统密钥目录的存储位置','新旧存储目录不能相互包含，原目录仍保留',
  '存储目录不能是链接','存储目录可用空间不足 128 MiB',
  '本机密钥无法解锁，请使用加密恢复包','已有档案缺少密钥，请先恢复密钥',
  '系统密钥保护不可用，未使用明文密钥降级','本地存储正在迁移或恢复，请稍后重试',
]);
const importSteps:Record<string,string>={
  OPEN_SNAPSHOT:'打开旧版备份快照',MATCH_ACCOUNT:'核对旧版账号',
  CHECK_DESTINATION:'检查本机档案目标',LOAD_SQL:'加载数据库运行组件',
  READ_DATABASE:'读取旧版数据库',VERIFY_DATABASE:'校验旧版数据库',
  SCAN_MESSAGES:'扫描旧版聊天记录',IMPORT_ATTACHMENT:'迁入旧版附件',
  PREPARE_MESSAGES:'整理待迁入消息',COMMIT_HISTORY:'写入历史记录',
  COMMIT_MARKER:'记录迁移完成标记',RESOLVE_QUARANTINE:'重检旧版隔离档案',
};
const importReasons:Record<string,string>={
  ENOENT:'所需文件不存在',EACCES:'没有文件访问权限',EPERM:'系统拒绝文件操作',
  EBUSY:'文件正在被占用',ENOTEMPTY:'目标目录已有内容',EIO:'磁盘读写失败',
  ENOSPC:'磁盘空间不足',UNKNOWN:'未知错误',ACCOUNT_QUARANTINED:'旧版账号档案需要核对',
  ACCOUNT_IDENTITY_MISMATCH:'新旧账号身份不一致',BACKUP_INTEGRITY:'备份完整性校验未通过',
  SQLITE_INTEGRITY:'旧版数据库完整性校验未通过',SQLITE_LIMIT:'旧版数据库超过处理限制',
  SCHEMA_UNSUPPORTED:'旧版数据库结构暂不支持',VAULT_NOT_EMPTY:'目标档案已有历史记录',
  SOURCE_MARKER_MISMATCH:'旧版来源标记不一致',FILENAME_TOO_LONG:'附件文件名过长',
  AUTHORIZATION_CHANGED:'账号授权已变化',MEMORY_LIMIT:'可用内存不足',
  SQLITE_RUNTIME:'数据库运行组件不可用',
  QUARANTINE_NO_CANDIDATE:'缺少可恢复的原始数据库',
  QUARANTINE_SCHEMA:'原始数据库结构不受支持',
  QUARANTINE_PAGES:'原始数据库页校验未通过',
  QUARANTINE_CONFLICT:'同账号旧库内容不一致，需要核对',
  QUARANTINE_IDENTITY:'原始数据库账号身份校验未通过',
  QUARANTINE_SQLITE:'原始数据库完整性校验未通过',
  QUARANTINE_CAPSULE:'恢复校验未通过',
  QUARANTINE_LIMIT:'原始数据库重检超过处理限制',
  QUARANTINE_NAME:'旧数据库名称格式不兼容',
  QUARANTINE_KEYS:'原始数据库页索引无效',
  QUARANTINE_SIZE:'原始数据库大小无效或超限',
  QUARANTINE_BINARY:'原始数据库页格式不兼容',
  QUARANTINE_HEADER:'原始数据库头校验未通过',
  QUARANTINE_GEOMETRY:'原始数据库页大小不一致',
  QUARANTINE_MISSING_PAGE:'原始数据库缺少必要页',
  QUARANTINE_LENGTH:'原始数据库页长度不一致',
};
const importDiagnostic=new RegExp(`^PUTY_LEGACY_import_(${Object.keys(importSteps).join('|')})_(${Object.keys(importReasons).join('|')})$`);
export function localStorageIPCMessage(error:unknown,native:boolean):string|undefined{
  if(!isLocalStorageIPCFailure(error,native))return;
  const detail=(error as Error).message.replace(/^(?:Error: )?Error invoking remote method ['"]puty-store['"]:\s*(?:Error: )?/, '');
  if(actionable.has(detail))return detail;
  const diagnostic=importDiagnostic.exec(detail);
  if(diagnostic && diagnostic[0]===detail){
    const advice=diagnostic[1]==='RESOLVE_QUARANTINE'
      ? '请保留备份、剩余源文件及密钥，并将诊断码提供给管理员核对。'
      : '请保留备份、剩余源文件及密钥，关闭旧版客户端后重试；仍失败时请将诊断码提供给管理员。';
    return `旧版数据迁移未完成（${importSteps[diagnostic[1]]}：${importReasons[diagnostic[2]]}；诊断码：${diagnostic[0]}）。${advice}`;
  }
  const match=/^PUTY_LEGACY_(inventory|reader|seal|cleanup|import)_(ENOENT|EACCES|EPERM|EBUSY|ENOTEMPTY|EIO|ENOSPC|UNKNOWN)$/.exec(detail);
  if(!match || match[0]!==detail)return localStorageFailureMessage;
  const phase=({inventory:'检查旧版备份',reader:'读取旧版记录',seal:'加密备份校验',cleanup:'清理旧版缓存',import:'迁入历史记录'} as Record<string,string>)[match[1]];
  return `旧版数据迁移未完成（${phase}，${match[2]}）。请保留备份、剩余源文件及密钥，关闭旧版客户端后重试；仍失败时请联系管理员。`;
}
