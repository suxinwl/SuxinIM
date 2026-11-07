import 'package:flutter/material.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim/routes/app_navigator.dart';

class OrganizationPage extends StatefulWidget {
  final bool active;
  final HrmDirectory? directory;
  const OrganizationPage({super.key, this.active = true, this.directory});
  @override
  State<OrganizationPage> createState() => _OrganizationPageState();
}

class _OrganizationPageState extends State<OrganizationPage> {
  final _search = TextEditingController();
  List<DirectoryDepartment> _departments = [];
  List<DirectoryPerson> _people = [];
  String _departmentID = '', _keyword = '', _error = '';
  bool _loading = false, _loaded = false;
  int _page = 1, _total = 0, _generation = 0;
  HrmDirectory get _directory => widget.directory ?? HrmDirectory.instance;
  @override
  void initState() { super.initState(); if (widget.active) _load(); }
  @override
  void didUpdateWidget(covariant OrganizationPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.active && !_loaded && !_loading) _load();
  }
  @override
  void dispose() { _generation++; _search.dispose(); super.dispose(); }

  Future<void> _load({bool append = false}) async {
    final attempt = ++_generation;
    final page = append ? _page + 1 : 1;
    setState(() { _loading = true; _error = ''; if (!append) _people = []; });
    try {
      final result = await _directory.list(keyword: _keyword, departmentID: _departmentID, page: page);
      if (!mounted || attempt != _generation) return;
      setState(() {
        _departments = result.departments;
        _people = append ? [..._people, ...result.items] : result.items;
        _total = result.total; _page = page; _loaded = true;
      });
    } catch (_) {
      if (mounted && attempt == _generation) setState(() => _error = '组织通讯录暂不可用，请重试');
    } finally {
      if (mounted && attempt == _generation) setState(() => _loading = false);
    }
  }

  void _select(String id) {
    _departmentID = id; _keyword = ''; _search.clear(); _load();
  }
  String get _departmentName {
    for (final department in _departments) { if (department.id == _departmentID) return department.name; }
    return '全部同事';
  }
  List<DirectoryDepartment> get _children {
    if (_departmentID.isNotEmpty) return _departments.where((d) => d.parentID == _departmentID && d.id != _departmentID).toList();
    final ids = _departments.map((d) => d.id).toSet();
    final roots = _departments.where((d) => !ids.contains(d.parentID) || d.id == d.parentID).toList();
    return roots.isEmpty ? _departments : roots;
  }

  Widget _departmentsView() => ListView(
    padding: const EdgeInsets.symmetric(vertical: 8),
    children: [
      if (_departmentID.isNotEmpty) ListTile(
        dense: true, leading: const Icon(Icons.arrow_back), title: const Text('返回上级部门'),
        onTap: () { final parent = _departments.where((d) => d.id == _departmentID); _select(parent.isEmpty ? '' : parent.first.parentID); },
      ),
      ListTile(dense: true, leading: const Icon(Icons.account_tree_outlined), title: const Text('全部同事'), selected: _departmentID.isEmpty, onTap: () => _select('')),
      for (final department in _children) ListTile(
        dense: true, leading: const Icon(Icons.folder_outlined), title: Text(department.name),
        subtitle: department.leaderName.isEmpty ? null : Text('负责人：${department.leaderName}'),
        trailing: const Icon(Icons.chevron_right), onTap: () => _select(department.id),
      ),
    ],
  );

  void _open(DirectoryPerson person) => AppNavigator.startUserProfilePane(
    userID: person.userID, nickname: person.nickname, faceURL: person.faceURL,
  );

  Widget _peopleView() => ListView(
    padding: const EdgeInsets.only(bottom: 24),
    children: [
      Padding(padding: const EdgeInsets.fromLTRB(16, 16, 16, 4), child: Text(_keyword.isEmpty ? _departmentName : '搜索“$_keyword”', style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600))),
      const Padding(padding: EdgeInsets.symmetric(horizontal: 16, vertical: 8), child: Text('点击同事查看资料，可直接发消息或添加好友。', style: TextStyle(color: Colors.grey, fontSize: 12))),
      if (_error.isNotEmpty) Padding(padding: const EdgeInsets.all(20), child: Column(children: [Text(_error), TextButton(onPressed: () => _load(), child: const Text('重试'))])),
      if (_loading && _people.isEmpty) const Padding(padding: EdgeInsets.all(32), child: Center(child: CircularProgressIndicator())),
      if (!_loading && _error.isEmpty && _people.isEmpty) Padding(padding: const EdgeInsets.all(28), child: Center(child: Text(_keyword.isEmpty ? '该部门暂无可显示的同事' : '没有找到匹配手机号、姓名或工号的同事'))),
      for (final person in _people) ListTile(
        leading: AvatarView(url: person.faceURL, text: person.nickname, width: 40, height: 40),
        title: Text(person.nickname, maxLines: 1, overflow: TextOverflow.ellipsis),
        subtitle: Text('工号：${person.employeeNo.isEmpty ? '—' : person.employeeNo} · ${person.department}${person.position.isEmpty ? '' : ' · ${person.position}'}', maxLines: 2, overflow: TextOverflow.ellipsis),
        trailing: const Icon(Icons.chevron_right), onTap: () => _open(person),
      ),
      if (_people.isNotEmpty) Padding(padding: const EdgeInsets.all(12), child: Center(child: Text('已显示 ${_people.length} / $_total 位同事', style: const TextStyle(color: Colors.grey)))),
      if (_people.length < _total) Center(child: TextButton(onPressed: _loading ? null : () => _load(append: true), child: Text(_loading ? '加载中…' : '加载更多'))),
    ],
  );

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('组织架构'), actions: [IconButton(tooltip: '刷新组织架构', onPressed: () => _load(), icon: const Icon(Icons.refresh))]),
    body: Column(children: [
      Padding(padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8), child: TextField(
        controller: _search, textInputAction: TextInputAction.search,
        decoration: InputDecoration(hintText: '搜索手机号、姓名或工号', prefixIcon: const Icon(Icons.search), suffixIcon: IconButton(tooltip: '搜索同事', icon: const Icon(Icons.arrow_forward), onPressed: () { _keyword = _search.text.trim(); _load(); })),
        onSubmitted: (value) { _keyword = value.trim(); _load(); },
      )),
      Expanded(child: LayoutBuilder(builder: (context, constraints) {
        if (constraints.maxWidth >= 700) return Row(children: [SizedBox(width: 240, child: _departmentsView()), const VerticalDivider(width: 1), Expanded(child: _peopleView())]);
        return Column(children: [SizedBox(height: _children.isEmpty && _departmentID.isEmpty ? 60 : 160, child: _departmentsView()), const Divider(height: 1), Expanded(child: _peopleView())]);
      })),
    ]),
  );
}
