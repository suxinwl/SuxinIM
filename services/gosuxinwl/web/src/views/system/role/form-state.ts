export function roleSelection(value: unknown): number[] {
  if (value == null || value === '' || value === '*') return [];
  let parsed = value;
  if (typeof value === 'string') {
    const text = value.trim();
    if (text === '' || text === '*') return [];
    parsed = text.startsWith('[') ? JSON.parse(text) : text.split(',').map((item) => item.trim());
  }
  if (!Array.isArray(parsed)) throw new Error('角色权限数据格式无效');
  if (parsed.some((item) => typeof item !== 'number' && typeof item !== 'string')) {
    throw new Error('角色权限编号无效');
  }
  const ids = parsed.map((item) => Number(item));
  if (ids.some((id) => !Number.isSafeInteger(id) || id <= 0)) {
    throw new Error('角色权限编号无效');
  }
  return [...new Set(ids)];
}

export function createRoleForm(record?: Record<string, any> | null) {
  return {
    id: Number(record?.id || 0),
    pid: Number(record?.pid || 0),
    name: String(record?.name || ''),
    remark: String(record?.remark || ''),
    menu: roleSelection(record?.menu),
    btns: roleSelection(record?.btns),
    weigh: Number(record?.weigh || 1),
    data_access: Number(record?.data_access || 0),
  };
}

// Menu visibility must never grant API permissions implicitly. Removing a
// subtree removes its explicit grants; selecting it again leaves them unset.
export function roleButtonsAfterMenuCheck(buttons: unknown, checked: boolean, subtreeButtons: unknown): number[] {
  const current = roleSelection(buttons);
  if (checked) return current;
  const removed = new Set(roleSelection(subtreeButtons));
  return current.filter((id) => !removed.has(id));
}
