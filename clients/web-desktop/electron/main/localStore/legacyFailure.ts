export type LegacyMigrationPhase = 'inventory' | 'reader' | 'seal' | 'cleanup' | 'import';
const codes = new Set(['ENOENT', 'EACCES', 'EPERM', 'EBUSY', 'ENOTEMPTY', 'EIO', 'ENOSPC']);
export const legacyImportSteps = ['OPEN_SNAPSHOT','MATCH_ACCOUNT','RESOLVE_QUARANTINE','CHECK_DESTINATION','LOAD_SQL','READ_DATABASE','VERIFY_DATABASE','SCAN_MESSAGES','IMPORT_ATTACHMENT','PREPARE_MESSAGES','COMMIT_HISTORY','COMMIT_MARKER'] as const;
export type LegacyImportStep = typeof legacyImportSteps[number];
export const legacyQuarantineReasons = ['QUARANTINE_NO_CANDIDATE','QUARANTINE_SCHEMA','QUARANTINE_PAGES','QUARANTINE_CONFLICT','QUARANTINE_IDENTITY','QUARANTINE_SQLITE','QUARANTINE_CAPSULE','QUARANTINE_LIMIT','QUARANTINE_NAME','QUARANTINE_KEYS','QUARANTINE_SIZE','QUARANTINE_BINARY','QUARANTINE_HEADER','QUARANTINE_GEOMETRY','QUARANTINE_MISSING_PAGE','QUARANTINE_LENGTH'] as const;
export const legacyImportReasons = [...codes,'UNKNOWN','ACCOUNT_QUARANTINED','ACCOUNT_IDENTITY_MISMATCH','BACKUP_INTEGRITY','SQLITE_INTEGRITY','SQLITE_LIMIT','SCHEMA_UNSUPPORTED','VAULT_NOT_EMPTY','SOURCE_MARKER_MISMATCH','FILENAME_TOO_LONG','AUTHORIZATION_CHANGED','MEMORY_LIMIT','SQLITE_RUNTIME',...legacyQuarantineReasons] as const;
const steps = new Set<string>(legacyImportSteps), reasons = new Set<string>(legacyImportReasons);
const knownReasons = new Map<string,string>([
  ['This legacy account is quarantined; encrypted source retained for recovery','ACCOUNT_QUARANTINED'],
  ['Legacy account is not the active vault account','ACCOUNT_IDENTITY_MISMATCH'],
  ['Legacy SQLite login identity differs from its declared account','ACCOUNT_IDENTITY_MISMATCH'],
  ['Legacy history has no verifiable login identity; encrypted quarantine retained','ACCOUNT_IDENTITY_MISMATCH'],
  ['Legacy SQLite integrity check failed; encrypted evidence retained','SQLITE_INTEGRITY'],
  ['Legacy SQLite exceeds RAM import bound','SQLITE_LIMIT'],
  ['Legacy in-memory object exceeds bound','SQLITE_LIMIT'],
  ['Legacy message schema cannot be safely assigned','SCHEMA_UNSUPPORTED'],
  ['Invalid legacy conversation identity','SCHEMA_UNSUPPORTED'],
  ['Legacy message has no stable identity','SCHEMA_UNSUPPORTED'],
  ['Existing vault cannot be overwritten by a legacy import','VAULT_NOT_EMPTY'],
  ['Legacy import found an existing SDK journal','VAULT_NOT_EMPTY'],
  ['Legacy import marker does not match the sealed source','SOURCE_MARKER_MISMATCH'],
  ['Legacy import differs from its durable source marker','SOURCE_MARKER_MISMATCH'],
  ['Legacy preparation marker is invalid','SOURCE_MARKER_MISMATCH'],
  ['文件名超出当前文件系统长度限制，请缩短后保存','FILENAME_TOO_LONG'],
  ['重名编号后文件名超出系统长度限制，请缩短源文件名后保存','FILENAME_TOO_LONG'],
  ['账号会话已改变','AUTHORIZATION_CHANGED'],
  ['账号安全版本已改变，请重新登录','AUTHORIZATION_CHANGED'],
  ['账号、网络或终端权限已失效','AUTHORIZATION_CHANGED'],
  ['本地档案已锁定，请在线重新登录','AUTHORIZATION_CHANGED'],
  ['本地档案已锁定','AUTHORIZATION_CHANGED'],
  ['Legacy importer requires fixed SQL.js 1.6.7','SQLITE_RUNTIME'],
  ['Legacy SQL.js resource path is unavailable','SQLITE_RUNTIME'],
]);
const backupReasons = new Set(['Legacy encrypted backup is truncated','Invalid legacy encrypted header','Legacy encrypted object context mismatch','Legacy encrypted frame exceeds bound','Legacy backup readback hash mismatch','Legacy backup size mismatch','Unsupported state or unable to authenticate data','Sealed legacy SQLite is absent']);

/** Keep raw messages/paths in memory only; IPC gets a closed diagnostic vocabulary. */
export function legacyImportFailure(step:LegacyImportStep,error:unknown):Error {
  const fields=error && typeof error==='object' ? error as {code?:unknown;legacyImportStep?:unknown;legacyImportReason?:unknown;legacyQuarantineReason?:unknown} : {};
  if(error instanceof Error && typeof fields.legacyImportStep==='string' && steps.has(fields.legacyImportStep) && typeof fields.legacyImportReason==='string' && reasons.has(fields.legacyImportReason))return error;
  const message=error instanceof Error ? error.message : '';
  let reason=typeof fields.code==='string' && codes.has(fields.code) ? fields.code : knownReasons.get(message) || 'UNKNOWN';
  if(typeof fields.legacyQuarantineReason==='string' && (legacyQuarantineReasons as readonly string[]).includes(fields.legacyQuarantineReason))reason=fields.legacyQuarantineReason;
  if(reason==='UNKNOWN'){
    if(backupReasons.has(message))reason='BACKUP_INTEGRITY';
    else if(/^(?:no such (?:table|column):|table .* has no column named |NOT NULL constraint failed:)/.test(message))reason='SCHEMA_UNSUPPORTED';
    else if(/^(?:out of memory|Cannot enlarge memory arrays|Array buffer allocation failed|Invalid typed array length)/i.test(message))reason='MEMORY_LIMIT';
  }
  return Object.assign(new Error(message || 'Legacy import failed',{cause:error}),{legacyImportStep:step,legacyImportReason:reason});
}

// Electron serializes Error.message across IPC. Never include an old path,
// database identity, raw exception, or payload in that message.
export function legacyMigrationFailure(phase: LegacyMigrationPhase, error: unknown): Error {
  if(phase==='import' && error && typeof error==='object'){
    const tagged=error as {legacyImportStep?:unknown;legacyImportReason?:unknown};
    if(typeof tagged.legacyImportStep==='string' && steps.has(tagged.legacyImportStep) && typeof tagged.legacyImportReason==='string' && reasons.has(tagged.legacyImportReason))return new Error(`PUTY_LEGACY_import_${tagged.legacyImportStep}_${tagged.legacyImportReason}`);
  }
  const value = error && typeof error === 'object' ? (error as {code?: unknown}).code : undefined;
  return new Error(`PUTY_LEGACY_${phase}_${typeof value === 'string' && codes.has(value) ? value : 'UNKNOWN'}`);
}
