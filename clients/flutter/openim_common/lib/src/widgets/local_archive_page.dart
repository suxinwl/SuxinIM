import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';

import 'local_archive_access.dart';
import 'protected_file_page.dart';

/// Local-only archive UI. Clearing never calls a server delete endpoint.
class LocalArchivePage extends StatefulWidget {
  final String? conversationID;
  final LocalArchiveAccess? access;
  final Future<void> Function()? onRestored;

  const LocalArchivePage(
      {super.key, this.conversationID, this.access, this.onRestored});

  @override
  State<LocalArchivePage> createState() => _LocalArchiveState();
}

class _LocalArchiveState extends State<LocalArchivePage> {
  late final LocalArchiveAccess access;
  final conversations = <ArchiveConversation>[];
  final messages = <Map<String, dynamic>>[];
  final attachments = <ArchiveAttachment>[];
  StreamSubscription<void>? lockSubscription;
  StreamSubscription<void>? invalidationSubscription;
  String? selected, error, attachmentCursor;
  bool busy = false, messageEnd = false, attachmentEnd = false;
  int generation = 0, conversationOffset = 0;
  int before = DateTime.now().millisecondsSinceEpoch + 1;
  String beforeID = '';
  final cleanupIDs = <String>{};
  bool beforeDateMode = false;
  late DateTime cutoffDate;
  late DateTimeRange range;

  @override
  void initState() {
    super.initState();
    access = widget.access ?? const NativeArchiveAccess();
    selected = widget.conversationID;
    if (selected != null && selected!.isNotEmpty) cleanupIDs.add(selected!);
    final today = archiveCalendarDate(DateTime.now());
    cutoffDate = today;
    range = DateTimeRange(
        start: today.subtract(const Duration(days: 30)), end: today);
    lockSubscription = access.locked.listen((_) {
      generation++;
      if (!mounted) return;
      setState(() {
        messages.clear();
        attachments.clear();
        conversations.clear();
        cleanupIDs.clear();
        busy = false;
        error = '当前在线授权已失效，本机档案已锁定。请重新登录。';
      });
    });
    invalidationSubscription = access.invalidated.listen((_) {
      generation++;
      if (!mounted) return;
      setState(() {
        messages.clear();
        attachments.clear();
        attachmentCursor = null;
        messageEnd = false;
        attachmentEnd = false;
        before = DateTime.now().millisecondsSinceEpoch + 1;
        beforeID = '';
        busy = false;
      });
    });
    if (access.unlocked) {
      loadConversations();
    } else {
      error = '请联网登录后打开本机档案。';
    }
  }

  @override
  void dispose() {
    generation++;
    lockSubscription?.cancel();
    invalidationSubscription?.cancel();
    super.dispose();
  }

  bool current(int attempt) =>
      mounted && generation == attempt && access.unlocked;

  void fail(int attempt, String message) {
    if (mounted && generation == attempt) setState(() => error = message);
  }

  void finish(int attempt) {
    if (mounted && generation == attempt) setState(() => busy = false);
  }

  void notice(String message) {
    if (mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(message)));
    }
  }

  Future<void> loadConversations() async {
    if (busy || !access.unlocked) return;
    final attempt = generation;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final batch = await access.conversations(conversationOffset);
      if (!current(attempt)) return;
      setState(() {
        conversationOffset += batch.length;
        final existing = conversations.map((item) => item.id).toSet();
        conversations.addAll(batch.where((item) => existing.add(item.id)));
      });
    } catch (_) {
      fail(attempt, '联系人 / 群组列表暂不可用，请联网重试。');
    } finally {
      finish(attempt);
    }
  }

  void chooseConversation(String? value) {
    generation++;
    setState(() {
      selected = value;
      messages.clear();
      attachments.clear();
      before = DateTime.now().millisecondsSinceEpoch + 1;
      beforeID = '';
      attachmentCursor = null;
      messageEnd = false;
      attachmentEnd = false;
      error = null;
      busy = false;
    });
  }

  Future<void> loadMessages({bool reset = false}) async {
    final id = selected;
    if (id == null || busy || !access.unlocked) return;
    final attempt = generation;
    if (reset) {
      messages.clear();
      before = DateTime.now().millisecondsSinceEpoch + 1;
      beforeID = '';
      messageEnd = false;
    }
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final page =
          await access.messages(id, before: before, beforeID: beforeID);
      if (!current(attempt) || selected != id) return;
      setState(() {
        final known = messages
            .map((item) => item['client_msg_id'])
            .whereType<String>()
            .toSet();
        messages.addAll(page.where((item) {
          final key = item['client_msg_id'];
          return key is String && key.isNotEmpty && known.add(key);
        }));
        messageEnd = page.isEmpty;
        if (page.isNotEmpty) {
          final time = page.last['send_time'];
          final key = page.last['client_msg_id'];
          if (time is! num || key is! String || key.isEmpty) {
            messageEnd = true;
          } else {
            before = time.toInt();
            beforeID = key;
          }
        }
      });
    } catch (_) {
      fail(attempt, '本地历史暂不可用，请联网验证身份后重试。');
    } finally {
      finish(attempt);
    }
  }

  Future<void> loadAttachments({bool reset = false}) async {
    if (busy || !access.unlocked) return;
    final attempt = generation;
    final id = selected;
    if (reset) {
      attachments.clear();
      attachmentCursor = null;
      attachmentEnd = false;
    }
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final page = await access.attachments(
          conversationID: id, cursor: attachmentCursor);
      if (!current(attempt) || selected != id) return;
      setState(() {
        final known = attachments.map((item) => item.taskID).toSet();
        attachments.addAll(page.items.where((item) => known.add(item.taskID)));
        attachmentEnd =
            page.nextCursor == null || page.nextCursor == attachmentCursor;
        attachmentCursor = page.nextCursor;
      });
    } catch (_) {
      fail(attempt, '本机附件暂不可用，请联网验证身份后重试。');
    } finally {
      finish(attempt);
    }
  }

  Future<bool> confirm(String title, String message, String action) async =>
      await showDialog<bool>(
          context: context,
          builder: (context) => AlertDialog(
                title: Text(title),
                content: SingleChildScrollView(child: Text(message)),
                actions: [
                  TextButton(
                      onPressed: () => Navigator.pop(context, false),
                      child: const Text('取消')),
                  FilledButton(
                      onPressed: () => Navigator.pop(context, true),
                      child: Text(action)),
                ],
              )) ==
      true;

  Future<void> clearRange() async {
    if (cleanupIDs.isEmpty || busy || !access.unlocked) return;
    final ids = cleanupIDs.toList()..sort();
    final attempt = generation;
    final bounds = beforeDateMode
        ? archiveBeforeBounds(cutoffDate)
        : archiveCleanupBounds(range.start, range.end);
    bool unchanged() =>
        current(attempt) &&
        cleanupIDs.length == ids.length &&
        ids.every(cleanupIDs.contains) &&
        (beforeDateMode
                    ? archiveBeforeBounds(cutoffDate)
                    : archiveCleanupBounds(range.start, range.end))
                .join(':') ==
            bounds.join(':');
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final preview =
          await access.previewClearScopes(ids, bounds[0], bounds[1]);
      if (!mounted || !unchanged()) return;
      final accepted = await showDialog<bool>(
              context: context,
              builder: (_) => _CleanupConfirmation(
                  access: access,
                  message:
                      '已选 ${ids.length} 个联系人 / 群组。${beforeDateMode ? '${dateLabel(cutoffDate)} 00:00 之前（不含该时刻）' : '${dateLabel(range.start)} 至 ${dateLabel(range.end)}'}，北京时间 UTC+08。\n\n'
                      '当前范围：消息 ${preview.messageCount} 条，本机导入 ${preview.localImportCount} 项，附件引用 ${preview.attachmentReferences} 项；预计删除 ${preview.filesToDelete} 个不再被引用的本机文件，预计释放 ${sizeLabel(preview.estimatedReleaseBytes)}（${preview.estimatedReleaseBytes} 字节）。共用文件按联合范围去重；不计工作数据库压缩差异。\n\n'
                      '清理所选联系人 / 群组在此日期范围（北京时间 UTC+08）的本机消息及已保存附件，导入项按首次导入时间计算。其它客户端和服务器记录不受影响；仍被其它消息引用的附件会保留。本机已清理范围会保留，同步与恢复旧备份不会重新带回。系统下载目录或其它应用的外部原件不随本次清理。')) ==
          true;
      if (!accepted || !unchanged()) return;
      await access.clearScopes(ids, bounds[0], bounds[1]);
      if (!mounted || !access.unlocked) return;
      setState(() {
        messages.clear();
        attachments.clear();
        attachmentCursor = null;
      });
      notice('本机清理完成');
    } catch (_) {
      fail(attempt, '本机清理未完成，请联网重试；未删除服务器记录。');
    } finally {
      finish(attempt);
    }
  }

  Future<void> importAttachment() async {
    if (busy || !access.unlocked) return;
    final id = selected;
    if (id == null || id.isEmpty) {
      notice('请先选择联系人或群组，再导入已有文件。');
      return;
    }
    final attempt = generation;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final imported = await access.importAttachment(id);
      if (!current(attempt) || selected != id || imported == null) return;
      if (imported.conversationID != id ||
          imported.localImportID == null ||
          imported.messageID.isNotEmpty) {
        throw const FormatException('Invalid import scope');
      }
      setState(() {
        attachments.removeWhere((item) => item.taskID == imported.taskID);
        attachments.insert(0, imported);
      });
      notice('文件已加密导入本机档案；未发送消息，外部原件保留。');
    } catch (_) {
      fail(attempt, '文件导入未完成。请检查在线授权、可用空间和文件访问权限后重试。');
    } finally {
      finish(attempt);
    }
  }

  Future<String?> passphrase({required bool creating}) => showDialog<String>(
        context: context,
        barrierDismissible: false,
        builder: (_) => _PassphraseDialog(creating: creating, access: access),
      );

  Future<void> exportBackup() async {
    if (busy || !access.unlocked) return;
    final attempt = generation;
    String? phrase = await passphrase(creating: true);
    if (phrase == null || !current(attempt)) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final path = await access.exportBackup(phrase);
      phrase = null;
      if (!current(attempt)) return;
      final saved = await access.saveBackup(path);
      if (current(attempt)) notice(saved ? '加密备份已导出。请妥善保管备份口令。' : '已取消导出加密备份');
    } catch (_) {
      fail(attempt, '加密备份未完成。请检查可用空间、在线授权和目标目录后重试。');
    } finally {
      phrase = null;
      finish(attempt);
    }
  }

  Future<void> restoreBackup() async {
    if (busy || !access.unlocked || widget.onRestored == null) return;
    final attempt = generation;
    String? restoreID;
    String? phrase;
    bool committed = false;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final picked = await access.pickRestoreBackup();
      if (picked == null || !current(attempt)) return;
      phrase = await passphrase(creating: false);
      if (phrase == null || !current(attempt)) return;
      restoreID = await access.prepareRestore(picked, phrase);
      phrase = null;
      if (!current(attempt)) return;
      final accepted = await confirm(
          '恢复此账户的本机档案',
          '备份已通过本机校验。恢复将替换此账户的本机档案，并保留本机已清理范围。不会覆盖服务器记录。完成后退出当前登录，请重新联网登录。',
          '恢复并重新登录');
      if (!accepted || !current(attempt)) return;
      await access.commitRestore(restoreID);
      committed = true;
      if (mounted) await widget.onRestored!();
    } catch (_) {
      fail(attempt, '恢复未完成。请检查备份口令、账户、平台、可用空间和在线授权。');
    } finally {
      phrase = null;
      if (restoreID != null && !committed) {
        try {
          await access.cancelRestore(restoreID);
        } catch (_) {
          fail(attempt, '恢复准备未完成，请重新登录后重试。');
        }
      }
      finish(attempt);
    }
  }

  String label(Map<String, dynamic> message) {
    final body = message['content'];
    if (body is String) {
      try {
        final parsed = jsonDecode(body);
        if (parsed is Map && parsed['content'] is String) {
          return parsed['content'] as String;
        }
      } catch (_) {/* Plain message content. */}
      return body;
    }
    return '加密消息';
  }

  String dateLabel(DateTime date) =>
      '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')}';
  String sizeLabel(int size) => size >= 1048576
      ? '${(size / 1048576).toStringAsFixed(1)} MiB'
      : '${(size / 1024).toStringAsFixed(1)} KiB';

  Future<void> chooseCleanupScopes() async {
    if (busy || !access.unlocked) return;
    final attempt = generation;
    final chosen = {...cleanupIDs};
    final accepted = await showDialog<bool>(
        context: context,
        builder: (dialogContext) => _ArchiveAccessDialog(
            access: access,
            child: StatefulBuilder(
                builder: (context, update) => AlertDialog(
                      title: const Text('选择清理联系人 / 群组'),
                      content: SizedBox(
                          width: double.maxFinite,
                          child: ListView(shrinkWrap: true, children: [
                            for (final item in conversations)
                              CheckboxListTile(
                                  key: ValueKey('cleanup-scope-${item.id}'),
                                  title: Text(item.name),
                                  value: chosen.contains(item.id),
                                  onChanged: (value) => update(() {
                                        if (value == true) {
                                          chosen.add(item.id);
                                        } else {
                                          chosen.remove(item.id);
                                        }
                                      })),
                          ])),
                      actions: [
                        TextButton(
                            onPressed: () =>
                                Navigator.pop(dialogContext, false),
                            child: const Text('取消')),
                        FilledButton(
                            onPressed: () => Navigator.pop(dialogContext, true),
                            child: const Text('确定范围'))
                      ],
                    ))));
    if (accepted != true || !current(attempt)) return;
    generation++;
    setState(() {
      cleanupIDs
        ..clear()
        ..addAll(chosen);
      error = null;
    });
  }

  Future<void> chooseCleanupDates() async {
    if (busy || !access.unlocked) return;
    final attempt = generation;
    if (beforeDateMode) {
      final chosen = await showDatePicker(
          context: context,
          firstDate: DateTime(2000),
          lastDate:
              archiveCalendarDate(DateTime.now()).add(const Duration(days: 1)),
          initialDate: cutoffDate);
      if (chosen != null && current(attempt)) {
        generation++;
        setState(() => cutoffDate = chosen);
      }
    } else {
      final chosen = await showDateRangePicker(
          context: context,
          firstDate: DateTime(2000),
          lastDate: archiveCalendarDate(DateTime.now()),
          initialDateRange: range);
      if (chosen != null && current(attempt)) {
        generation++;
        setState(() => range = chosen);
      }
    }
  }

  Widget conversationPicker({bool all = false}) {
    final selectedKnown = conversations.any((item) => item.id == selected);
    return DropdownButtonFormField<String>(
      key: ValueKey('conversation-${all ? 'attachment' : 'history'}-$selected'),
      value: selectedKnown ? selected : null,
      isExpanded: true,
      decoration: InputDecoration(labelText: all ? '联系人 / 群组（可选）' : '联系人 / 群组'),
      items: [
        if (all)
          const DropdownMenuItem<String>(value: null, child: Text('全部已保存附件')),
        for (final item in conversations)
          DropdownMenuItem(
              value: item.id,
              child: Text(item.name, overflow: TextOverflow.ellipsis)),
      ],
      onChanged: busy || !access.unlocked ? null : chooseConversation,
    );
  }

  Widget historyTab() => ListView(padding: const EdgeInsets.all(16), children: [
        const Text('历史按月加密保存。查看和清理均需联网授权；清理只影响本机。'),
        const SizedBox(height: 16),
        conversationPicker(),
        TextButton(
            onPressed: busy || !access.unlocked ? null : loadConversations,
            child: const Text('加载更多联系人 / 群组')),
        Wrap(spacing: 12, children: [
          OutlinedButton(
              onPressed: busy || selected == null || !access.unlocked
                  ? null
                  : () => loadMessages(reset: true),
              child: const Text('查看历史')),
        ]),
        for (final message in messages)
          ListTile(
            title: Text(label(message),
                maxLines: 5, overflow: TextOverflow.ellipsis),
            subtitle: Text(DateTime.fromMillisecondsSinceEpoch(
                    (message['send_time'] as num?)?.toInt() ?? 0,
                    isUtc: true)
                .add(const Duration(hours: 8))
                .toIso8601String()
                .replaceAll('T', ' ')
                .replaceAll('Z', ' UTC+08')),
          ),
        if (messages.isNotEmpty && !messageEnd)
          TextButton(
              onPressed: busy ? null : loadMessages, child: const Text('更早历史')),
        if (messageEnd)
          const Padding(padding: EdgeInsets.all(16), child: Text('已读完本机历史')),
      ]);

  Widget cleanupTab() => ListView(padding: const EdgeInsets.all(16), children: [
        const Text('可同时清理多个联系人和群组。先计算联合范围，再确认删除；共用附件不会重复计算。'),
        const SizedBox(height: 16),
        OutlinedButton.icon(
            onPressed: busy || !access.unlocked ? null : chooseCleanupScopes,
            icon: const Icon(Icons.checklist),
            label: Text('选择清理联系人 / 群组（已选 ${cleanupIDs.length}）')),
        TextButton(
            onPressed: busy || !access.unlocked ? null : loadConversations,
            child: const Text('加载更多联系人 / 群组')),
        DropdownButtonFormField<bool>(
            key: const ValueKey('cleanup-date-mode'),
            value: beforeDateMode,
            decoration: const InputDecoration(labelText: '清理时间模式'),
            items: const [
              DropdownMenuItem(value: false, child: Text('日期范围')),
              DropdownMenuItem(value: true, child: Text('某天之前'))
            ],
            onChanged: busy || !access.unlocked
                ? null
                : (value) {
                    if (value == null) return;
                    generation++;
                    setState(() => beforeDateMode = value);
                  }),
        ListTile(
            key: const ValueKey('cleanup-calendar'),
            contentPadding: EdgeInsets.zero,
            title: const Text('清理日期 · 北京时间 UTC+08'),
            subtitle: Text(beforeDateMode
                ? '${dateLabel(cutoffDate)} 00:00 之前（不含该时刻）'
                : '${dateLabel(range.start)} 至 ${dateLabel(range.end)}'),
            trailing: const Icon(Icons.date_range),
            onTap: busy || !access.unlocked ? null : chooseCleanupDates),
        FilledButton(
            onPressed: busy || cleanupIDs.isEmpty || !access.unlocked
                ? null
                : clearRange,
            child: const Text('仅清理本机')),
        const SizedBox(height: 16),
        const Text('仅删除本机受管档案，不删除服务器或其它设备的记录。外部另存副本保留。'),
      ]);

  Widget attachmentTab() =>
      ListView(padding: const EdgeInsets.all(16), children: [
        const Text('仅显示已主动加密保存的附件。原文件名用于识别；预览在应用内存中完成，不生成明文副本。'),
        const SizedBox(height: 16),
        conversationPicker(all: true),
        OutlinedButton.icon(
            onPressed: busy || !access.unlocked ? null : importAttachment,
            icon: const Icon(Icons.file_open_outlined),
            label: const Text('导入已有文件')),
        TextButton(
            onPressed: busy || !access.unlocked
                ? null
                : () => loadAttachments(reset: true),
            child: const Text('读取本机附件')),
        for (final item in attachments)
          ListTile(
            leading: const Icon(Icons.insert_drive_file_outlined),
            title:
                Text(item.name, maxLines: 2, overflow: TextOverflow.ellipsis),
            subtitle: Text(item.localImportID == null
                ? '${item.month}  ·  ${sizeLabel(item.size)}'
                : '本机导入  ·  ${sizeLabel(item.size)}\n首次导入：${dateLabel(archiveCalendarDate(DateTime.fromMillisecondsSinceEpoch(item.firstImportedAt!, isUtc: true)))}'),
            trailing: const Icon(Icons.chevron_right),
            onTap: busy || !access.unlocked
                ? null
                : () => Navigator.of(context).push(MaterialPageRoute<void>(
                    builder: (_) => ProtectedFilePage(
                        name: item.name, taskID: item.taskID))),
          ),
        if (attachmentEnd && attachments.isEmpty)
          const Padding(padding: EdgeInsets.all(16), child: Text('没有已保存的本机附件')),
        if (!attachmentEnd && attachments.isNotEmpty)
          TextButton(
              onPressed: busy ? null : loadAttachments,
              child: const Text('更多附件')),
        const Padding(
            padding: EdgeInsets.only(top: 16),
            child: Text('图片、文本和平台支持的媒体可在应用内预览。不支持的格式不会交给外部应用明文打开。')),
        const Padding(
            padding: EdgeInsets.only(top: 12),
            child: Text(
                '可选择已有文件加密导入所选联系人 / 群组的本机档案。此操作不会发送消息，也不会删除外部原件。旧版本另存到系统下载目录或其它应用的文件不随本机档案清理。')),
      ]);

  Widget backupTab() => ListView(padding: const EdgeInsets.all(16), children: [
        const Icon(Icons.enhanced_encryption_outlined, size: 48),
        const SizedBox(height: 16),
        const Text('备份包包含本账户的加密档案及已保存附件。导出的是密文 ZIP，需独立备份口令才能恢复；管理员无法找回该口令。'),
        const SizedBox(height: 16),
        FilledButton.icon(
            onPressed: busy || !access.unlocked ? null : exportBackup,
            icon: const Icon(Icons.save_alt),
            label: const Text('导出加密备份')),
        const SizedBox(height: 24),
        const Text('仅接受当前账户、兼容平台的完整备份。恢复前验证内容，保留本机已清理范围；完成后必须重新登录。请先导出现有档案备份。'),
        const SizedBox(height: 12),
        OutlinedButton.icon(
            onPressed: busy || !access.unlocked || widget.onRestored == null
                ? null
                : restoreBackup,
            icon: const Icon(Icons.restore),
            label: const Text('选择加密备份并恢复')),
        if (widget.onRestored == null)
          const Text('请从“我的 → 账户设置 → 本机档案与备份”执行恢复。'),
      ]);

  @override
  Widget build(BuildContext context) => DefaultTabController(
      length: 4,
      child: Scaffold(
        appBar: AppBar(
            title: const Text('本机档案与备份'),
            bottom: const TabBar(tabs: [
              Tab(text: '消息历史'),
              Tab(text: '已存附件'),
              Tab(text: '本机清理'),
              Tab(text: '加密备份')
            ])),
        body: Column(children: [
          if (busy) const LinearProgressIndicator(),
          if (error != null)
            Material(
                color: Theme.of(context).colorScheme.errorContainer,
                child: Padding(
                    padding: const EdgeInsets.all(12),
                    child: Text(error!, key: const ValueKey('archive-error')))),
          Expanded(
              child: TabBarView(children: [
            historyTab(),
            attachmentTab(),
            cleanupTab(),
            backupTab()
          ])),
        ]),
      ));
}

class _ArchiveAccessDialog extends StatefulWidget {
  final LocalArchiveAccess access;
  final Widget child;
  const _ArchiveAccessDialog({required this.access, required this.child});
  @override
  State<_ArchiveAccessDialog> createState() => _ArchiveAccessDialogState();
}

class _ArchiveAccessDialogState extends State<_ArchiveAccessDialog> {
  StreamSubscription<void>? locked, invalidated;
  bool closing = false;
  void cancel() {
    if (!mounted || closing) return;
    closing = true;
    Navigator.of(context).pop(false);
  }

  @override
  void initState() {
    super.initState();
    locked = widget.access.locked.listen((_) => cancel());
    invalidated = widget.access.invalidated.listen((_) => cancel());
    if (!widget.access.unlocked) {
      WidgetsBinding.instance.addPostFrameCallback((_) => cancel());
    }
  }

  @override
  void dispose() {
    locked?.cancel();
    invalidated?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}

class _CleanupConfirmation extends StatefulWidget {
  final LocalArchiveAccess access;
  final String message;
  const _CleanupConfirmation({required this.access, required this.message});
  @override
  State<_CleanupConfirmation> createState() => _CleanupConfirmationState();
}

class _CleanupConfirmationState extends State<_CleanupConfirmation> {
  StreamSubscription<void>? locked, invalidated;
  bool closing = false;
  void close(bool accepted) {
    if (!mounted || closing) return;
    closing = true;
    Navigator.of(context).pop(accepted && widget.access.unlocked);
  }

  @override
  void initState() {
    super.initState();
    locked = widget.access.locked.listen((_) => close(false));
    invalidated = widget.access.invalidated.listen((_) => close(false));
    if (!widget.access.unlocked) {
      WidgetsBinding.instance.addPostFrameCallback((_) => close(false));
    }
  }

  @override
  void dispose() {
    locked?.cancel();
    invalidated?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: const Text('仅清理本机'),
        content: SingleChildScrollView(child: Text(widget.message)),
        actions: [
          TextButton(onPressed: () => close(false), child: const Text('取消')),
          FilledButton(onPressed: () => close(true), child: const Text('确认清理'))
        ],
      );
}

class _PassphraseDialog extends StatefulWidget {
  final bool creating;
  final LocalArchiveAccess access;
  const _PassphraseDialog({required this.creating, required this.access});
  @override
  State<_PassphraseDialog> createState() => _PassphraseState();
}

class _PassphraseState extends State<_PassphraseDialog> {
  final phrase = TextEditingController(), repeated = TextEditingController();
  StreamSubscription<void>? locked;
  String? error;
  bool closing = false;
  void close([String? result]) {
    if (!mounted || closing) return;
    closing = true;
    phrase.clear();
    repeated.clear();
    Navigator.of(context).pop(result);
  }

  @override
  void initState() {
    super.initState();
    locked = widget.access.locked.listen((_) {
      phrase.clear();
      repeated.clear();
      close();
    });
  }

  @override
  void dispose() {
    locked?.cancel();
    phrase.clear();
    repeated.clear();
    phrase.dispose();
    repeated.dispose();
    super.dispose();
  }

  void submit() {
    if (!widget.access.unlocked) {
      close();
      return;
    }
    if (phrase.text.isEmpty || (widget.creating && phrase.text.length < 12)) {
      setState(() => error = widget.creating ? '备份口令至少 12 个字符' : '请输入备份口令');
      return;
    }
    if (widget.creating && phrase.text != repeated.text) {
      setState(() => error = '两次口令不一致');
      return;
    }
    final value = phrase.text;
    phrase.clear();
    repeated.clear();
    close(value);
  }

  @override
  Widget build(BuildContext context) => PopScope<String>(
      onPopInvokedWithResult: (didPop, result) {
        if (didPop) closing = true;
      },
      child: AlertDialog(
        title: Text(widget.creating ? '设置独立备份口令' : '输入备份口令'),
        content: SingleChildScrollView(
            child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Text('口令仅在本次操作内存中使用。不要使用或输入账户登录密码。'),
          TextField(
              key: const ValueKey('backup-passphrase'),
              controller: phrase,
              obscureText: true,
              autocorrect: false,
              enableSuggestions: false,
              enableIMEPersonalizedLearning: false,
              decoration: const InputDecoration(labelText: '备份口令')),
          if (widget.creating)
            TextField(
                key: const ValueKey('backup-confirm'),
                controller: repeated,
                obscureText: true,
                autocorrect: false,
                enableSuggestions: false,
                enableIMEPersonalizedLearning: false,
                decoration: const InputDecoration(labelText: '再次输入备份口令')),
          if (error != null)
            Text(error!, style: const TextStyle(color: Colors.red)),
        ])),
        actions: [
          TextButton(onPressed: close, child: const Text('取消')),
          FilledButton(
              onPressed: submit,
              child: Text(widget.creating ? '生成加密备份' : '校验备份'))
        ],
      ));
}
