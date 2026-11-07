export type FormValue = string | number | boolean | undefined;
export type Row = Record<string, unknown> & { __key: string };
export function object(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : {};
}
const booleans = new Set(['allowAddFriend', 'discoverEnabled', 'force', 'enabled', 'confirm']);
export function formValue(key: string, value: unknown): FormValue {
  if (booleans.has(key)) return value === true || value === 'true' || value === 1 || value === '1';
  if (typeof value === 'number' || typeof value === 'string' || typeof value === 'boolean') return value;
  return undefined;
}
export function extractResponse(value: unknown): { rows: Row[]; total: number | null } {
  const top = object(value);
  const config = object(top.config);
  const source = Object.keys(config).length || 'config' in top ? config : top;
  const keys = ['items', 'groups', 'groupInfos', 'groupList', 'members', 'memberList', 'messages', 'chatLogs', 'users', 'userList', 'friendsInfo', 'friends', 'records', 'versions', 'list', 'data'];
  const found = keys.map(key => source[key]).find(Array.isArray);
  const collection: unknown[] = Array.isArray(value) ? value : found as unknown[] || (Object.keys(source).length ? [source] : []);
  let total: number | null = null;
  for (const key of ['total', 'totalCount', 'totalNum']) if (source[key] !== undefined && Number.isFinite(Number(source[key]))) total = Number(source[key]);
  const rows = collection.map((item, index) => {
    const row: Record<string, unknown> = typeof item === 'object' && item !== null ? { ...object(item) } : { value: item };
    for (const key of ['groupInfo', 'userInfo', 'friendInfo', 'projection', 'message']) {
      const nested = object(row[key]);
      if (Object.keys(nested).length) { delete row[key]; Object.assign(row, nested); }
    }
    for (const key of ['authFingerprint', 'password', 'salt', 'token']) delete row[key];
    for (const [raw, normalized] of Object.entries({ send_id: 'sendID', recv_id: 'recvID', group_id: 'groupID', send_time: 'sendTime', content_type: 'contentType', client_msg_id: 'clientMsgID' })) {
      if (row[raw] !== undefined) { row[normalized] = row[raw]; delete row[raw]; }
    }
    if (row.url !== undefined && row.version !== undefined) row.downloadURL = row.url;
    if (row.text !== undefined && row.version !== undefined) row.description = row.text;
    if (!row.conversationID && typeof row.docID === 'string') row.conversationID = row.docID.replace(/:\d+$/, '');
    if (!row.userID && row.sendID) row.userID = row.sendID;
    return { ...row, __key: String(row.groupID || row.userID || row.id || row.employeeID || index) + ':' + index };
  });
  return { rows, total };
}
export function mergeDiscovery(existing: Row[], item: Record<string, unknown>): Record<string, unknown>[] {
  const rows = existing.map(({ __key, ...entry }) => entry);
  const index = rows.findIndex(entry => item.id && entry.id === item.id);
  if (index >= 0) rows[index] = item; else rows.push(item);
  return rows;
}
export function validateHTTPS(value: unknown, label: string) {
  if (!value) return;
  let url: URL;
  try { url = new URL(String(value)); } catch { throw new Error(`${label}必须是有效的 HTTPS 地址`); }
  if (url.protocol !== 'https:' || url.username || url.password) throw new Error(`${label}必须是有效的 HTTPS 地址`);
}
