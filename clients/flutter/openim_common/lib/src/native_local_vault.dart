import 'dart:convert';
import 'dart:io';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/services.dart';
import 'package:flutter/painting.dart';
import 'dart:typed_data';
import 'dart:async';
import 'package:openim_common/openim_common.dart';

/// Native owns the master and verifies the online actor itself. Session proof is
/// used only in RAM and never becomes a local archive key or a URL parameter.
class NativeLocalVault {
  static const channel = MethodChannel('puty/local_store');
  static int _generation = 0;
  static int _authorizationSerial = 0;
  static int? _nativeActorGeneration;
  static String? _actorAccount, _actorOrigin;
  static bool _unlocked = false;
  static bool restoring = false;
  static Future<void> Function()? quiesceSDKForRestore;
  static bool get unlocked => _unlocked;
  static int get generation => _generation;
  static final _invalidated = StreamController<void>.broadcast();
  static Stream<void> get invalidated => _invalidated.stream;
  static final _backgroundLocked = StreamController<void>.broadcast();
  static Stream<void> get backgroundLocked => _backgroundLocked.stream;
  static final _locked = StreamController<void>.broadcast();
  static Stream<void> get locked => _locked.stream;
  static void install() {
    channel.setMethodCallHandler((call) async {
      if (call.method == 'onVaultLocked') {
        _authorizationSerial++;
        _nativeActorGeneration = null;
        _actorAccount = null;
        _actorOrigin = null;
        _generation++;
        _unlocked = false;
        PaintingBinding.instance.imageCache.clear();
        PaintingBinding.instance.imageCache.clearLiveImages();
        if (call.arguments == 'BACKGROUND_LOCK' ||
            call.arguments == 'ONLINE_REAUTHORIZATION_REQUIRED') {
          _backgroundLocked.add(null);
        } else {
          _locked.add(null);
        }
        _invalidated.add(null);
      }
    });
  }

  static Future<void> unlock(LoginCertificate certificate) async {
    final request = ++_authorizationSerial;
    final origin = Config.serverBaseUrl;
    final response = await channel.invokeMapMethod<String, dynamic>('unlock', {
      'origin': origin,
      'userID': certificate.userID,
      'imToken': certificate.imToken,
      'chatToken': certificate.chatToken,
    });
    if (request != _authorizationSerial)
      throw StateError('Local vault authorization superseded');
    final actor = response?['actorGeneration'];
    if (response?['userID'] != certificate.userID ||
        actor is! int ||
        actor < 0) {
      await lock();
      throw StateError('Local vault identity changed');
    }
    if (!_unlocked ||
        _actorAccount != certificate.userID ||
        _actorOrigin != origin ||
        _nativeActorGeneration != actor) {
      _generation++;
      PaintingBinding.instance.imageCache.clear();
      PaintingBinding.instance.imageCache.clearLiveImages();
      _invalidated.add(null);
    }
    _actorAccount = certificate.userID;
    _actorOrigin = origin;
    _nativeActorGeneration = actor;
    _unlocked = true;
  }

  static Future<void> lock({bool reauthorize = false}) async {
    _authorizationSerial++;
    _nativeActorGeneration = null;
    _actorAccount = null;
    _actorOrigin = null;
    _generation++;
    _unlocked = false;
    PaintingBinding.instance.imageCache.clear();
    PaintingBinding.instance.imageCache.clearLiveImages();
    if (reauthorize) {
      _backgroundLocked.add(null);
    } else {
      _locked.add(null);
    }
    _invalidated.add(null);
    await channel.invokeMethod<bool>('lock');
  }

  static Future<void> suspend() => lock(reauthorize: true);

  /// Android may freeze its renewal scheduler while the screen is locked.
  /// Check the native monotonic lease before reusing its current actor on resume.
  static Future<bool> ensureCurrentAuthorization() async {
    if (!_unlocked) return false;
    final generation = _generation;
    final authorization = _authorizationSerial;
    try {
      if (await channel.invokeMethod<bool>('authorization/current') == true &&
          _unlocked && generation == _generation &&
          authorization == _authorizationSerial) return true;
    } catch (_) { /* Keep the archive closed until a fresh online unlock. */ }
    if (generation == _generation && authorization == _authorizationSerial)
      await suspend();
    return false;
  }

  static Future<List<Map<String, dynamic>>> page(String conversationID,
      {required int before, String beforeID = '', int limit = 50}) async {
    if (!_unlocked) throw StateError('请联网验证身份后查看本地档案');
    final generation = _generation;
    final value = await channel.invokeMethod<String>('archive/page', {
      'conversationID': conversationID,
      'before': before,
      'beforeID': beforeID,
      'limit': limit,
    });
    if (!_unlocked || generation != _generation)
      throw StateError('Local vault locked');
    final data = jsonDecode(value ?? '{}') as Map<String, dynamic>;
    if (data['error'] != null) throw StateError('本地档案暂不可用，请联网重试');
    return (data['items'] as List? ?? [])
        .map((e) => Map<String, dynamic>.from(e as Map))
        .toList();
  }

  static Future<void> clear(String conversationID, int from, int to) async {
    if (!_unlocked) throw StateError('请联网验证身份后清理本地档案');
    final generation = _generation;
    if (await channel.invokeMethod<bool>('archive/clear', {
          'conversationID': conversationID,
          'from': from,
          'to': to,
        }) !=
        true) throw StateError('本机清理未完成，请重试');
    if (!_unlocked || generation != _generation) throw StateError('本机档案身份已变更');
    _generation++;
    PaintingBinding.instance.imageCache.clear();
    PaintingBinding.instance.imageCache.clearLiveImages();
    _invalidated.add(null);
  }

  static Future<Map<String, dynamic>> save(String url, String sourceName,
      {int? size, String? conversationID, String? messageID}) async {
    if (!_unlocked) throw StateError('请联网验证身份后下载附件');
    final generation = _generation;
    final result =
        await channel.invokeMapMethod<String, dynamic>('attachment/save', {
      'url': url,
      'sourceName': sourceName,
      if (size != null) 'size': size,
      'conversationID': conversationID ?? '',
      'messageID': messageID ?? ''
    });
    if (!_unlocked || generation != _generation || result?['encrypted'] != true)
      throw StateError('加密附件保存未完成');
    return Map<String, dynamic>.from(result!);
  }

  static Future<Uint8List> read(String taskID) async {
    if (!_unlocked) throw StateError('请联网验证身份后预览附件');
    final generation = _generation;
    final nativeBytes = await channel
        .invokeMethod<Uint8List>('attachment/read', {'taskID': taskID});
    // Engine platform-message views may be unmodifiable. Callers own this
    // mutable buffer and can wipe it on lock, invalidation or disposal.
    final result = nativeBytes == null ? null : Uint8List.fromList(nativeBytes);
    if (!_unlocked || generation != _generation || result == null) {
      result?.fillRange(0, result.length, 0);
      throw StateError('加密附件预览不可用');
    }
    return result;
  }

  static Future<String> importSource(String path) async {
    if (!_unlocked) throw StateError('请联网验证身份后导入媒体');
    final generation = _generation;
    final result =
        await channel.invokeMethod<String>('source/import', {'path': path});
    if (!_unlocked ||
        generation != _generation ||
        result == null ||
        result.isEmpty) throw StateError('加密媒体导入失败');
    return result;
  }

  static Future<String> sourceBytes(Uint8List bytes, String name) async {
    if (!_unlocked || bytes.length > 32 << 20) throw StateError('加密媒体大小超出限制');
    final generation = _generation;
    final result = await channel.invokeMethod<String>(
        'source/fromBytes', {'bytes': bytes, 'name': name});
    if (!_unlocked ||
        generation != _generation ||
        result == null ||
        result.isEmpty) throw StateError('加密媒体保存失败');
    return result;
  }

  static Future<Map<String, dynamic>> sourceInfo(String path) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final result = await channel
        .invokeMapMethod<String, dynamic>('source/info', {'path': path});
    if (!_unlocked || generation != _generation || result == null)
      throw StateError('本机媒体不可用');
    return Map<String, dynamic>.from(result);
  }

  static Future<Uint8List> readSource(String path) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final nativeBytes =
        await channel.invokeMethod<Uint8List>('source/read', {'path': path});
    final result = nativeBytes == null ? null : Uint8List.fromList(nativeBytes);
    if (!_unlocked || generation != _generation || result == null) {
      result?.fillRange(0, result.length, 0);
      throw StateError('本机媒体已锁定');
    }
    return result;
  }

  static Future<String> scratch(String name) async {
    if (!_unlocked) throw StateError('请联网验证身份后创建媒体');
    final dot = name.lastIndexOf('.');
    final result = await channel.invokeMethod<String>('source/scratch', {
      'prefix': dot > 0 ? name.substring(0, dot) : name,
      'suffix': dot > 0 ? name.substring(dot) : '.bin'
    });
    if (result == null || result.isEmpty) throw StateError('加密媒体暂存不可用');
    return result;
  }

  static Future<Map<String, dynamic>> previewClear(
      String conversationID, int from, int to) async {
    if (!_unlocked || conversationID.isEmpty) throw StateError('请选择本机档案会话');
    final generation = _generation;
    final value = await channel.invokeMethod<String>('archive/previewClear',
        {'conversationID': conversationID, 'from': from, 'to': to});
    if (!_unlocked || generation != _generation || value == null)
      throw StateError('本机清理范围已变更');
    final result = Map<String, dynamic>.from(jsonDecode(value) as Map);
    if (result['conversationID'] != conversationID ||
        result['from'] != from ||
        result['to'] != to) throw StateError('本机清理范围不一致');
    return result;
  }

  static List<String> _clearScopeIDs(
      List<String> values, int from, int toExclusive) {
    final ids = values.toSet().toList()..sort();
    if (ids.isEmpty ||
        ids.length > 100 ||
        ids.any((id) => id.isEmpty) ||
        from < 0 ||
        toExclusive <= from ||
        toExclusive > 9007199254740991) throw StateError('请选择有效的本机清理范围');
    return ids;
  }

  static Future<Map<String, dynamic>> previewClearScopes(
      List<String> conversationIDs, int from, int toExclusive) async {
    if (!_unlocked) throw StateError('请联网验证身份后查看清理预估');
    final ids = _clearScopeIDs(conversationIDs, from, toExclusive),
        generation = _generation;
    final encoded = await channel.invokeMethod<String>(
        'archive/previewClearScopes',
        {'conversationIDs': ids, 'from': from, 'toExclusive': toExclusive});
    if (!_unlocked || generation != _generation || encoded == null)
      throw StateError('本机清理范围已变更');
    final result = Map<String, dynamic>.from(jsonDecode(encoded) as Map);
    final echoed = result['conversationIDs'];
    if (echoed is! List ||
        echoed.length != ids.length ||
        List.generate(ids.length, (i) => echoed[i] == ids[i])
            .any((equal) => !equal) ||
        result['from'] != from ||
        result['toExclusive'] != toExclusive) throw StateError('本机清理范围不一致');
    for (final key in [
      'messageCount',
      'localImportCount',
      'attachmentReferences',
      'filesToDelete',
      'estimatedReleaseBytes'
    ]) {
      final value = result[key];
      if (value is! int || value < 0 || value > 9007199254740991)
        throw StateError('本机清理预估无效');
    }
    return result;
  }

  static Future<void> clearScopes(
      List<String> conversationIDs, int from, int toExclusive) async {
    if (!_unlocked) throw StateError('请联网验证身份后清理本机档案');
    final ids = _clearScopeIDs(conversationIDs, from, toExclusive),
        generation = _generation;
    if (await channel.invokeMethod<bool>('archive/clearScopes', {
          'conversationIDs': ids,
          'from': from,
          'toExclusive': toExclusive
        }) !=
        true) throw StateError('本机清理未完成，请重试');
    if (!_unlocked || generation != _generation) throw StateError('本机档案身份已变更');
    _generation++;
    PaintingBinding.instance.imageCache.clear();
    PaintingBinding.instance.imageCache.clearLiveImages();
    _invalidated.add(null);
  }

  static Future<Map<String, dynamic>?> pickImportAttachment(
      String conversationID) async {
    if (!_unlocked || conversationID.isEmpty) throw StateError('请先选择联系人或群组');
    final generation = _generation;
    String? source;
    if (Platform.isIOS) {
      final items = await nativePickFiles(multiple: false);
      if (items.isNotEmpty) source = items.first['path'] as String?;
    } else {
      final result = await FilePicker.platform
          .pickFiles(type: FileType.any, allowMultiple: false, withData: false);
      source = result?.files.single.path;
    }
    if (source == null) return null;
    try {
      if (!_unlocked || generation != _generation)
        throw StateError('本机导入身份已变更');
      final encoded = await channel.invokeMethod<String>('attachment/import',
          {'path': source, 'conversationID': conversationID});
      if (!_unlocked || generation != _generation || encoded == null)
        throw StateError('本机加密导入未完成');
      final result = Map<String, dynamic>.from(jsonDecode(encoded) as Map);
      if (result['conversationID'] != conversationID ||
          result['messageID'] != '' ||
          result['localImportID'] is! String ||
          result['firstImportedAt'] is! int) throw StateError('本机导入归属不一致');
      return result;
    } finally {
      if (_unlocked && generation == _generation) await discard(source);
    }
  }

  static Future<void> finalizeLegacy() async {
    if (!_unlocked) throw StateError('旧数据加密迁移需要在线验证');
    final generation = _generation;
    if (await channel.invokeMethod<bool>('legacy/finalize') != true ||
        !_unlocked ||
        generation != _generation) throw StateError('旧数据加密迁移未完成，请重试');
  }

  static Future<void> discard(String path) =>
      channel.invokeMethod<void>('source/discard', {'path': path});
  static Future<String> exportBackup(String passphrase) async {
    if (!_unlocked) throw StateError('请联网验证身份后备份');
    final result = await channel
        .invokeMethod<String>('backup/export', {'passphrase': passphrase});
    if (result == null || result.isEmpty) throw StateError('加密备份失败');
    return result;
  }

  static Future<String> prepareRestore(String path, String passphrase) async {
    if (!_unlocked) throw StateError('请联网验证身份后恢复');
    final result = await channel.invokeMethod<String>(
        'backup/prepare', {'path': path, 'passphrase': passphrase});
    if (result == null || result.isEmpty) throw StateError('加密恢复校验失败');
    return result;
  }

  static Future<void> cancelRestore(String id) =>
      channel.invokeMethod<void>('backup/cancel', {'restoreID': id});
  static Future<void> commitRestore(String id) async {
    if (!_unlocked || restoring || quiesceSDKForRestore == null)
      throw StateError('恢复需要先停止SDK读写');
    final generation = _generation;
    restoring = true;
    try {
      await quiesceSDKForRestore!();
      if (!_unlocked || generation != _generation) throw StateError('恢复身份已变更');
      if (await channel
              .invokeMethod<bool>('backup/commit', {'restoreID': id}) !=
          true) throw StateError('加密恢复未完成');
    } finally {
      restoring = false;
      await lock();
    }
  }

  static Future<Map<String, dynamic>> listAttachments(
      {String? conversationID, String? cursor, int limit = 50}) async {
    if (!_unlocked) throw StateError('请联网验证身份后查看附件');
    final generation = _generation;
    final value = await channel.invokeMethod<String>('attachment/list', {
      'conversationID': conversationID ?? '',
      'cursor': cursor ?? '',
      'limit': limit
    });
    if (!_unlocked || generation != _generation || value == null)
      throw StateError('本机附件暂不可用');
    return Map<String, dynamic>.from(jsonDecode(value) as Map);
  }

  static Future<String?> pickRestoreBackup() async {
    if (!_unlocked) throw StateError('请联网验证身份后恢复');
    final generation = _generation;
    final value = await channel.invokeMethod<String>('backup/pick');
    if (!_unlocked || generation != _generation) throw StateError('本机身份已变更');
    return value;
  }

  static Future<bool> saveBackup(String encryptedPath) async {
    if (!_unlocked) throw StateError('请联网验证身份后导出');
    final generation = _generation;
    final value = await channel
        .invokeMethod<bool>('backup/save', {'path': encryptedPath});
    if (!_unlocked || generation != _generation) throw StateError('本机身份已变更');
    return value == true;
  }

  static Future<bool> captureSupported() async =>
      await channel.invokeMethod<bool>('source/captureSupported') == true;
  static Future<Uri> sourcePreviewUri(String path) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value =
        await channel.invokeMethod<String>('source/previewUri', {'path': path});
    if (!_unlocked || generation != _generation || value == null)
      throw StateError('本机媒体不可用');
    return Uri.parse(value);
  }

  /// A native range reader, never a plaintext file or a public download URL.
  static Future<Uri> attachmentPreviewUri(String taskID) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value = await channel.invokeMethod<String>(
        'attachment/previewUri', {'taskID': taskID});
    if (!_unlocked || generation != _generation || value == null) {
      throw StateError('本机媒体不可用');
    }
    final uri = Uri.parse(value);
    if (uri.scheme != 'puty-local' ||
        uri.host != 'attachment' ||
        uri.userInfo.isNotEmpty ||
        uri.hasPort ||
        uri.hasQuery ||
        uri.hasFragment ||
        uri.pathSegments.length != 1 ||
        uri.pathSegments.single != taskID) {
      throw StateError('本机媒体标识无效');
    }
    return uri;
  }

  static Future<List<Map<String, dynamic>>> nativePickFiles(
          {bool multiple = true}) async =>
      _nativeList('picker/files', {'multiple': multiple});
  static Future<List<Map<String, dynamic>>> nativePickAssets(
          {bool multiple = true,
          bool images = true,
          bool videos = true}) async =>
      _nativeList('picker/assets',
          {'multiple': multiple, 'images': images, 'videos': videos});
  static Future<List<Map<String, dynamic>>> _nativeList(
      String method, Map<String, dynamic> arguments) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value = await channel.invokeListMethod<dynamic>(method, arguments);
    if (!_unlocked || generation != _generation) throw StateError('本机媒体身份已变更');
    return (value ?? [])
        .map((e) => Map<String, dynamic>.from(e as Map))
        .toList();
  }

  static Future<Map<String, dynamic>?> nativeCapture(
      {String kind = 'both', int maxDuration = 60}) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value = await channel.invokeMapMethod<String, dynamic>(
        'media/capture', {'kind': kind, 'maxDuration': maxDuration});
    if (!_unlocked || generation != _generation) throw StateError('本机媒体身份已变更');
    return value == null ? null : Map<String, dynamic>.from(value);
  }

  static Future<Map<String, dynamic>> nativeMediaInfo(String path) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value = await channel
        .invokeMapMethod<String, dynamic>('media/info', {'path': path});
    if (!_unlocked || generation != _generation || value == null)
      throw StateError('本机媒体不可用');
    return Map<String, dynamic>.from(value);
  }

  static Future<String> nativeThumbnail(String path, {int quality = 80}) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value = await channel.invokeMethod<String>(
        'media/thumbnail', {'path': path, 'positionMS': 0, 'quality': quality});
    if (!_unlocked || generation != _generation || value == null)
      throw StateError('本机缩略图不可用');
    return value;
  }

  static Future<Map<String, dynamic>?> nativeCompress(
      String path, String operationID) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    final value =
        await channel.invokeMapMethod<String, dynamic>('media/compress', {
      'path': path,
      'operationID': operationID,
      'quality': 'medium',
      'includeAudio': true
    });
    if (!_unlocked || generation != _generation) throw StateError('本机媒体身份已变更');
    return value == null ? null : Map<String, dynamic>.from(value);
  }

  static Future<void> nativeCancelMedia(String operationID) =>
      channel.invokeMethod<void>('media/cancel', {'operationID': operationID});
  static Future<void> nativePreview(
      {String? path, String? taskID, String? url, String? name}) async {
    if (!_unlocked) throw StateError('本机媒体已锁定');
    final generation = _generation;
    await channel.invokeMethod<bool>('media/preview', {
      if (path != null) 'path': path,
      if (taskID != null) 'taskID': taskID,
      if (url != null) 'url': url,
      if (name != null) 'name': name
    });
    if (!_unlocked || generation != _generation) {
      throw StateError('本机媒体身份已变更');
    }
  }
}
