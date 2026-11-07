import 'dart:async';

import 'package:app_im/app_im.dart';
import 'package:openim_common/openim_common.dart';

class ArchiveConversation {
  final String id;
  final String name;
  const ArchiveConversation(this.id, this.name);
}

class ArchiveAttachment {
  final String taskID;
  final String name;
  final String month;
  final int size;
  final String conversationID;
  final String messageID;
  final String? localImportID;
  final int? firstImportedAt;

  const ArchiveAttachment({
    required this.taskID,
    required this.name,
    required this.month,
    required this.size,
    required this.conversationID,
    required this.messageID,
    this.localImportID,
    this.firstImportedAt,
  });

  static ArchiveAttachment? parse(Object? value) {
    if (value is! Map) return null;
    final id = value['taskID'];
    final name = value['name'];
    final size = value['size'];
    final imported = value['localImportID'];
    final importedAt = value['firstImportedAt'];
    if (id is! String ||
        id.isEmpty ||
        name is! String ||
        name.isEmpty ||
        size is! num ||
        !size.isFinite ||
        size < 0 ||
        size != size.roundToDouble() ||
        (imported != null &&
            (imported is! String ||
                imported.isEmpty ||
                importedAt is! int ||
                importedAt <= 0)) ||
        (imported == null && importedAt != null)) {
      return null;
    }
    return ArchiveAttachment(
      taskID: id,
      name: name,
      size: size.toInt(),
      month: value['month'] is String ? value['month'] as String : '',
      conversationID: value['conversationID'] is String
          ? value['conversationID'] as String
          : '',
      messageID:
          value['messageID'] is String ? value['messageID'] as String : '',
      localImportID: imported is String ? imported : null,
      firstImportedAt: importedAt is int ? importedAt : null,
    );
  }
}

class ArchiveCleanupPreview {
  final int messageCount, localImportCount, attachmentReferences, filesToDelete;
  final int estimatedReleaseBytes;
  const ArchiveCleanupPreview(this.messageCount, this.localImportCount,
      this.attachmentReferences, this.filesToDelete,
      [this.estimatedReleaseBytes = 0]);

  factory ArchiveCleanupPreview.parse(Map<String, dynamic> value,
      {bool requireReleaseBytes = false}) {
    int count(String key) {
      final result = value[key];
      if (result is! int || result < 0 || result > 9007199254740991) {
        throw const FormatException('Invalid local cleanup preview');
      }
      return result;
    }

    return ArchiveCleanupPreview(
        count('messageCount'),
        count('localImportCount'),
        count('attachmentReferences'),
        count('filesToDelete'),
        requireReleaseBytes ? count('estimatedReleaseBytes') : 0);
  }
}

class ArchiveAttachmentPage {
  final List<ArchiveAttachment> items;
  final String? nextCursor;
  const ArchiveAttachmentPage(this.items, this.nextCursor);

  factory ArchiveAttachmentPage.parse(Map<String, dynamic> value) {
    final items = <ArchiveAttachment>[];
    for (final item
        in value['items'] is List ? value['items'] as List : const []) {
      final parsed = ArchiveAttachment.parse(item);
      if (parsed == null) {
        throw const FormatException('Invalid local attachment');
      }
      items.add(parsed);
    }
    final next = value['nextCursor'];
    if (next != null && next is! String) {
      throw const FormatException('Invalid local attachment cursor');
    }
    return ArchiveAttachmentPage(
        items, next is String && next.isNotEmpty ? next : null);
  }
}

/// Date picker values are calendar dates in the archive's fixed UTC+08 zone.
/// Device timezone changes must not change the selected cleanup interval.
List<int> archiveDateBounds(DateTime start, DateTime end) {
  final from = DateTime.utc(start.year, start.month, start.day)
      .subtract(const Duration(hours: 8));
  final through = DateTime.utc(end.year, end.month, end.day + 1)
      .subtract(const Duration(hours: 8, milliseconds: 1));
  if (through.isBefore(from)) throw ArgumentError('Invalid date range');
  return [from.millisecondsSinceEpoch, through.millisecondsSinceEpoch];
}

/// Union cleanup uses a half-open interval [from, toExclusive).
List<int> archiveCleanupBounds(DateTime start, DateTime end) {
  final inclusive = archiveDateBounds(start, end);
  return [inclusive[0], inclusive[1] + 1];
}

/// The selected Beijing calendar date itself is excluded entirely.
List<int> archiveBeforeBounds(DateTime date) => [
      0,
      DateTime.utc(date.year, date.month, date.day)
          .subtract(const Duration(hours: 8))
          .millisecondsSinceEpoch
    ];

DateTime archiveCalendarDate(DateTime instant) {
  final date = instant.toUtc().add(const Duration(hours: 8));
  return DateTime(date.year, date.month, date.day);
}

/// No key material or decrypted attachment paths cross this interface.
abstract class LocalArchiveAccess {
  bool get unlocked;
  Stream<void> get locked;
  Stream<void> get invalidated;
  Future<List<ArchiveConversation>> conversations(int offset);
  Future<List<Map<String, dynamic>>> messages(String conversationID,
      {required int before, String beforeID = ''});
  Future<ArchiveAttachmentPage> attachments(
      {String? conversationID, String? cursor});
  Future<void> clear(String conversationID, int from, int to);
  Future<ArchiveCleanupPreview> previewClear(
      String conversationID, int from, int to);
  Future<ArchiveCleanupPreview> previewClearScopes(
      List<String> conversationIDs, int from, int toExclusive);
  Future<void> clearScopes(
      List<String> conversationIDs, int from, int toExclusive);
  Future<ArchiveAttachment?> importAttachment(String conversationID);
  Future<String> exportBackup(String passphrase);
  Future<bool> saveBackup(String encryptedPath);
  Future<String?> pickRestoreBackup();
  Future<String> prepareRestore(String selectedBackup, String passphrase);
  Future<void> cancelRestore(String restoreID);
  Future<void> commitRestore(String restoreID);
}

class NativeArchiveAccess implements LocalArchiveAccess {
  const NativeArchiveAccess();
  @override
  bool get unlocked => NativeLocalVault.unlocked;
  @override
  Stream<void> get locked => NativeLocalVault.locked;
  @override
  Stream<void> get invalidated => NativeLocalVault.invalidated;
  @override
  Future<List<ArchiveConversation>> conversations(int offset) async {
    final values = await AppIm.iMManager.conversationManager
        .getConversationListSplit(offset: offset, count: 50);
    return values
        .where((item) => item.conversationID.isNotEmpty)
        .map((item) => ArchiveConversation(
            item.conversationID, item.showName ?? '联系人 / 群组'))
        .toList();
  }

  @override
  Future<List<Map<String, dynamic>>> messages(String conversationID,
          {required int before, String beforeID = ''}) =>
      NativeLocalVault.page(conversationID, before: before, beforeID: beforeID);
  @override
  Future<ArchiveAttachmentPage> attachments(
          {String? conversationID, String? cursor}) async =>
      ArchiveAttachmentPage.parse(await NativeLocalVault.listAttachments(
          conversationID: conversationID, cursor: cursor, limit: 50));
  @override
  Future<void> clear(String id, int from, int to) =>
      NativeLocalVault.clear(id, from, to);
  @override
  Future<ArchiveCleanupPreview> previewClear(
          String id, int from, int to) async =>
      ArchiveCleanupPreview.parse(
          await NativeLocalVault.previewClear(id, from, to));
  @override
  Future<ArchiveCleanupPreview> previewClearScopes(
          List<String> ids, int from, int toExclusive) async =>
      ArchiveCleanupPreview.parse(
          await NativeLocalVault.previewClearScopes(ids, from, toExclusive),
          requireReleaseBytes: true);
  @override
  Future<void> clearScopes(List<String> ids, int from, int toExclusive) =>
      NativeLocalVault.clearScopes(ids, from, toExclusive);
  @override
  Future<ArchiveAttachment?> importAttachment(String id) async {
    final value = await NativeLocalVault.pickImportAttachment(id);
    if (value == null) return null;
    final item = ArchiveAttachment.parse(value);
    if (item == null ||
        item.conversationID != id ||
        item.localImportID == null ||
        item.messageID.isNotEmpty) {
      throw const FormatException('Invalid imported local attachment');
    }
    return item;
  }

  @override
  Future<String> exportBackup(String passphrase) =>
      NativeLocalVault.exportBackup(passphrase);
  @override
  Future<bool> saveBackup(String path) => NativeLocalVault.saveBackup(path);
  @override
  Future<String?> pickRestoreBackup() => NativeLocalVault.pickRestoreBackup();
  @override
  Future<String> prepareRestore(String path, String passphrase) =>
      NativeLocalVault.prepareRestore(path, passphrase);
  @override
  Future<void> cancelRestore(String id) => NativeLocalVault.cancelRestore(id);
  @override
  Future<void> commitRestore(String id) => NativeLocalVault.commitRestore(id);
}
