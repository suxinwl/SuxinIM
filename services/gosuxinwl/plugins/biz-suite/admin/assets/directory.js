import { el, api, asRows, can, page, surface, button, dialog, errorNode, navigate, state, safeURL } from './core.js';

export const memberName = (row, fallback = '未匹配成员') => {
  const name = String(row?.name || '').trim();
  return name && name !== String(row?.id ?? row?.user_id ?? row?.conversation_id ?? '') ? name : fallback;
};
export const contactName = (row) => row?.display_name || memberName(row);
export const maySend = (row) => Boolean(row?.can_send ?? row?.can_chat) && can('im', 'write');
export const openPrivate = (row) => navigate('im/chat?' + new URLSearchParams({ kind: 'private', id: String(row.id ?? row.user_id) }));

// Reused by the directory and group picker. Only the current server page is
// loaded; selection is keyed by local account, never by name or employee number.
function directoryPicker(host, { selected = new Map(), onChange = () => {}, allowChat = false } = {}) {
  let pageNumber = 1, department = '', sequence = 0, stopped = false, departments = [];
  const epoch = state.epoch, controller = new AbortController();
  const tree = el('nav', { class: 'directory-tree', 'aria-label': '企业部门' });
  const results = el('div', { class: 'directory-results', 'aria-live': 'polite' });
  const query = el('input', { name: 'directory_search', placeholder: '搜索姓名或工号', 'aria-label': '姓名或工号' });
  const children = el('input', { type: 'checkbox', name: 'include_subdepartments', checked: true });
  const search = el('form', { class: 'directory-search' }, query,
    el('label', { class: 'directory-check' }, children, '包含下级部门'),
    el('button', { type: 'submit', class: 'button primary' }, '查询'),
    button('清空', () => { query.value = ''; department = ''; children.checked = true; pageNumber = 1; renderTree(); load(); }));
  const selectedList = el('div', { class: 'directory-selection', 'aria-label': '已选成员' });
  host.append(search, selectedList, el('div', { class: 'directory-layout' }, tree, results));
  const current = (request) => !stopped && epoch === state.epoch && host.isConnected && request === sequence;
  function selectionChanged() {
    selectedList.replaceChildren();
    for (const [id, row] of selected) selectedList.append(el('span', { class: 'selection-chip', title: row.department_path || '' }, memberName(row) + (row.employee_no ? ' · ' + row.employee_no : ''),
      button('×', () => { selected.delete(id); selectionChanged(); for (const input of results.querySelectorAll('[data-member-id]')) if (input.dataset.memberId === String(id)) input.checked = false; }, 'small', { 'aria-label': '移除' + memberName(row) + (row.employee_no ? '（' + row.employee_no + '）' : '') })));
    onChange([...selected.values()]);
  }
  function renderTree() {
    tree.replaceChildren(button('全公司', () => { department = ''; pageNumber = 1; renderTree(); load(); }, department ? '' : 'active', { 'data-department': '', name: 'directory_department', value: '' }));
    const known = new Set(departments.map(row => String(row.id))), visited = new Set();
    function branch(parent, items) {
      const list = el('ul');
      for (const row of items) {
        const id = String(row.id); if (visited.has(id)) continue; visited.add(id);
        const item = el('li', {}, button(row.name || '未命名部门', () => { department = id; pageNumber = 1; renderTree(); load(); }, department === id ? 'active' : '', { 'data-department': id, name: 'directory_department', value: id, title: row.path || row.name }));
        const descendants = departments.filter(child => String(child.parent_id) === id && !visited.has(String(child.id)));
        if (descendants.length) item.append(branch(id, descendants));
        list.append(item);
      }
      return list;
    }
    tree.append(branch('', departments.filter(row => !known.has(String(row.parent_id)))));
    if (visited.size < departments.length) tree.append(branch('', departments.filter(row => !visited.has(String(row.id)))));
    tree.append(button('未分配部门', () => { department = 'unassigned'; pageNumber = 1; renderTree(); load(); }, department === 'unassigned' ? 'active' : '', { name: 'directory_department', value: 'unassigned', 'data-department': 'unassigned' }));
  }
  async function load() {
    const request = ++sequence;
    results.replaceChildren(el('p', { class: 'loading' }, '正在读取通讯录…'));
    const params = new URLSearchParams({ q: query.value.trim(), include_subdepartments: String(children.checked), page: String(pageNumber), page_size: '20' });
    if (department === 'unassigned') params.set('unassigned', 'true');
    else if (department) params.set('department_id', department);
    try {
      const data = await api('/im/directory/users?' + params, undefined, undefined, false, { signal: controller.signal });
      if (!current(request)) return;
      const rows = asRows(data), total = Number(data.total || 0), size = Number(data.page_size || 20);
      results.replaceChildren(el('p', { class: 'hint directory-count' }, `${total} 名员工 · 第 ${pageNumber} 页`));
      const list = el('div', { class: 'directory-people' });
      for (const row of rows) {
        const id = Number(row.id ?? row.user_id), available = maySend(row) && id > 0;
        if (!available && selected.delete(id)) selectionChanged();
        const reason = row.readonly_reason || row.unavailable_reason || (!can('im', 'write') ? '当前账号没有发送消息权限' : '暂不可发起聊天');
        const checkbox = el('input', { type: 'checkbox', 'data-member-id': id || '', disabled: !available, 'aria-label': '选择' + memberName(row), title: available ? '选择成员' : reason });
        checkbox.checked = available && selected.has(id);
        checkbox.addEventListener('change', () => { if (checkbox.checked) selected.set(id, row); else selected.delete(id); selectionChanged(); });
        const avatar = typeof row.avatar === 'string' && row.avatar.trim() ? safeURL(row.avatar) : null;
        const placeholder = () => el('span', { class: 'directory-avatar' }, memberName(row).slice(0, 1));
        const identity = el('div', { class: 'directory-person-info' }, el('strong', {}, memberName(row)),
          el('small', {}, [row.employee_no ? '工号 ' + row.employee_no : '未填写工号', row.position].filter(Boolean).join(' · ')),
          el('small', {}, row.department_path || row.department_name || '未分配部门'),
          el('small', { class: 'directory-account-status' }, ({ active: '账号已启用', disabled: '账号已停用', unlinked: '尚未关联账号', deleted: '账号已删除', no_permission: '未获通讯权限', resigned: '员工已离职' })[row.account_status] || (row.can_chat ? '账号可用' : '账号状态待核对')),
          row.display_name ? el('small', {}, '备注：' + row.display_name) : null,
          !available ? el('span', { class: 'directory-unavailable' }, reason) : null);
        list.append(el('article', { class: 'directory-person', 'data-entry-id': row.entry_id || row.employee_id || '', 'data-employee': row.employee_id || '', 'data-user': id || '', 'data-user-id': id || '' },
          checkbox, avatar ? el('img', { src: avatar, alt: '', class: 'directory-avatar', loading: 'lazy', referrerpolicy: 'no-referrer', onerror: event => event.target.replaceWith(placeholder()) }) : placeholder(), identity,
          allowChat ? button('发起聊天', () => openPrivate(row), 'small', { disabled: !available, title: available ? '与' + memberName(row) + '聊天' : reason }) : null));
      }
      if (!rows.length) list.append(el('p', { class: 'empty' }, '没有符合条件的员工，请调整部门或姓名、工号。'));
      results.append(list, el('div', { class: 'pager' },
        button('上一页', () => { pageNumber--; load(); }, '', { disabled: pageNumber <= 1 }),
        el('span', {}, `共 ${total} 人`),
        button('下一页', () => { pageNumber++; load(); }, '', { disabled: pageNumber * size >= total })));
    } catch (error) {
      if (!current(request) || error.name === 'AbortError') return;
      results.replaceChildren(errorNode(error), button('重试', load));
    }
  }
  search.addEventListener('submit', event => { event.preventDefault(); pageNumber = 1; load(); });
  children.addEventListener('change', () => { pageNumber = 1; load(); });
  selectionChanged();
  (async () => {
    try {
      departments = asRows(await api('/im/directory/departments', undefined, undefined, false, { signal: controller.signal }));
      if (stopped || epoch !== state.epoch || !host.isConnected) return;
      renderTree();
    } catch (error) { if (!stopped && epoch === state.epoch && error.name !== 'AbortError') tree.replaceChildren(errorNode(error)); }
  })();
  load();
  return { destroy() { stopped = true; controller.abort(); }, selected };
}

export function createGroup(initial = [], onCreated) {
  const epoch = state.epoch;
  let submitting = false;
  const selected = new Map(initial.filter(row => maySend(row) && Number(row.id ?? row.user_id) > 0).map(row => [Number(row.id ?? row.user_id), row]));
  const content = el('div'), errors = el('div'), picker = el('div');
  const name = el('input', { name: 'group_name', required: true, maxlength: 100, placeholder: '请输入群名称' });
  const notice = el('textarea', { name: 'group_notice', rows: 2, placeholder: '群公告（选填）' });
  const submit = el('button', { type: 'submit', class: 'button primary' }, '创建群聊');
  const editor = el('form', {}, errors, el('div', { class: 'form-grid' },
    el('label', { class: 'field' }, '群名称', name), el('label', { class: 'field' }, '群公告', notice)),
    el('p', { class: 'notice' }, '选择同事后创建群聊。后续新成员申请加入需要审批。'), el('footer', { class: 'form-actions' }, submit));
  content.append(editor, picker);
  const view = dialog('选择同事创建群聊', content); view.modal.classList.add('directory-dialog');
  const control = directoryPicker(picker, { selected, onChange(rows) { submit.disabled = !rows.length; submit.textContent = `创建群聊（${rows.length} 人）`; } });
  closePickerOnNavigation(view, control);
  editor.addEventListener('submit', async event => {
    event.preventDefault(); errors.replaceChildren();
    if (submitting || !name.value.trim() || !selected.size || epoch !== state.epoch || !can('im', 'write')) return;
    submitting = true;
    submit.disabled = true;
    try {
      const group = await api('/im/groups', { name: name.value.trim(), notice: notice.value.trim(), join_approval: 1, member_ids: [...selected.keys()] });
      if (!view.modal.isConnected || epoch !== state.epoch) return;
      control.destroy(); view.close();
      if (onCreated) await onCreated(group);
      else navigate('im/chat?' + new URLSearchParams({ kind: 'group', id: String(group.id), name: name.value.trim() }));
    } catch (error) {
      if (view.modal.isConnected) errors.append(errorNode({ message: '创建失败，请刷新通讯录并重新选择成员后重试。' + (error.message ? '\n服务端：' + error.message : '') }));
    }
    finally { submitting = false; submit.disabled = !selected.size; }
  });
}

function closePickerOnNavigation(view, control) {
  const close = () => view.close();
  window.addEventListener('popstate', close, { once: true });
  view.modal.addEventListener('close', () => { control.destroy(); window.removeEventListener('popstate', close); }, { once: true });
}

export function chooseMembers(title, submitMembers) {
  const epoch = state.epoch, content = el('div'), picker = el('div'), errors = el('div'), selected = new Map();
  const submit = button('邀请选中成员', async () => {
    if (!selected.size || epoch !== state.epoch || !can('im', 'write')) return;
    submit.disabled = true; errors.replaceChildren();
    try {
      await submitMembers([...selected.keys()]);
      view.close();
    } catch (error) { if (view.modal.isConnected) errors.append(errorNode(error)); }
    finally { submit.disabled = !selected.size; }
  }, 'primary', { disabled: true });
  content.append(errors, picker, el('footer', { class: 'form-actions' }, submit));
  const view = dialog(title, content); view.modal.classList.add('directory-dialog');
  const control = directoryPicker(picker, { selected, onChange(rows) { submit.disabled = !rows.length; } });
  closePickerOnNavigation(view, control);
}

export function directory() {
  const selected = new Map();
  const create = button('发起群聊（0 人）', () => createGroup([...selected.values()]), 'primary', { disabled: true });
  const body = page('企业通讯录', '按部门、姓名或工号查找同事，无需添加好友。未启用或未关联账号的员工可查看资料。', can('im', 'write') ? create : null);
  const content = surface(); body.append(content);
  const picker = directoryPicker(content, { selected, allowChat: true, onChange(rows) { create.textContent = `发起群聊（${rows.length} 人）`; create.disabled = !rows.length; } });
  state.cleanup = () => picker.destroy();
}
