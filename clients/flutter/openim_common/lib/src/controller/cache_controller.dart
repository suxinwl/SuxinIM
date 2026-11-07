import 'dart:convert';
import 'dart:async';

import 'package:app_im/app_im.dart';
import 'package:get/get.dart';
import 'package:hive/hive.dart';
import 'package:openim_common/openim_common.dart';
import 'package:uuid/uuid.dart';
import 'encrypted_cache_box.dart';

/// flutter packages pub run build_runner build
/// flutter packages pub run build_runner build --delete-conflicting-outputs
///
class CacheController extends GetxController {
  StreamSubscription<void>? _vaultInvalidated;String? _cacheAccount;
  final favoriteList = <EmojiInfo>[].obs;
  final callRecordList = <CallRecords>[].obs;
  EncryptedCacheBox<EmojiInfo>? favoriteBox;
  EncryptedCacheBox<CallRecords>? callRecordBox;
  bool _isInitFavoriteList = false;
  bool _isInitCallRecords = false;

  String get userID => DataSp.getLoginCertificate()!.userID;

  void addFavoriteFromUrl(String? url, int? width, int? height) {
    var emoji = EmojiInfo(url: url, width: width, height: height);
    favoriteList.insert(0, emoji);
    favoriteBox?.put(userID, favoriteList);
  }

  void addFavoriteFromPath(String path, int width, int height) async {
    var result = await LoadingView.singleton.wrap(
      asyncFunction: () => AppIm.iMManager.uploadFile(
        id: const Uuid().v4(),
        filePath: path,
        fileName: path,
      ),
      // asyncFunction: () => HttpUtil.uploadImageForMinio(path: path),
    );
    if (result is String) {
      final url = jsonDecode(result)['url'];
      Logger.print('url:$url');
      var emoji = EmojiInfo(url: url, width: width, height: height);
      Logger.print('addFavoriteFromPath :$url');
      favoriteList.insert(0, emoji);
      favoriteBox?.put(userID, favoriteList);
    }
  }

  void delFavorite(String url) {
    favoriteList.removeWhere((element) => element.url == url);
    favoriteBox?.put(userID, favoriteList);
  }

  void delFavoriteList(List<String> urlList) {
    for (final url in urlList) {
      favoriteList.removeWhere((element) => element.url == url);
    }
    favoriteBox?.put(userID, favoriteList);
  }

  initFavoriteEmoji() {
    if (!_isInitFavoriteList) {
      _isInitFavoriteList = true;
      var list = favoriteBox?.get(userID, defaultValue: <EmojiInfo>[]);
      if (list != null) {
        favoriteList.assignAll((list as List).cast());
      }
    }
  }

  List<String> get urlList => favoriteList.map((e) => e.url!).toList();

  initCallRecords() {
    if (!_isInitCallRecords) {
      _isInitCallRecords = true;
      var list = callRecordBox?.get(userID, defaultValue: <CallRecords>[]);
      if (list != null) {
        callRecordList.assignAll((list as List).cast());
      }
    }
  }

  void resetCache() {
    if (_isInitCallRecords) {
      final list = callRecordBox?.get(userID, defaultValue: <CallRecords>[]);
      if (list != null) {
        callRecordList.assignAll((list as List).cast());
      }
    }

    if (_isInitFavoriteList) {
      final list = favoriteBox?.get(userID, defaultValue: <EmojiInfo>[]);
      if (list != null) {
        favoriteList.assignAll((list as List).cast());
      }
    }
  }

  addCallRecords(CallRecords records) {
    callRecordList.insert(0, records);
    callRecordBox?.put(userID, callRecordList);
  }

  deleteCallRecords(CallRecords records) async {
    callRecordList.removeWhere((element) => element.userID == records.userID && element.date == records.date);
    await callRecordBox?.put(userID, callRecordList);
  }

  @override
  void onClose() {
    _vaultInvalidated?.cancel();
    _isInitFavoriteList = false;
    _isInitCallRecords = false;
    Hive.close();
    super.onClose();
  }

  static Future<void>? _legacyMigration;
  static Future<void> prepareLegacyCaches()=>_legacyMigration??=_migrateLegacyCaches();
  static Future<void> _migrateLegacyCaches()async{
    final emojiAdapter=EmojiInfoAdapter(),callAdapter=CallRecordsAdapter();
    if(!Hive.isAdapterRegistered(emojiAdapter.typeId))Hive.registerAdapter(emojiAdapter);
    if(!Hive.isAdapterRegistered(callAdapter.typeId))Hive.registerAdapter(callAdapter);
    final favorite=EncryptedCacheBox<EmojiInfo>('favoriteEmoji',encode:(e)=>{'url':e.url,'width':e.width,'height':e.height},decode:(e)=>EmojiInfo(url:e['url'],width:e['width'],height:e['height']));
    final calls=EncryptedCacheBox<CallRecords>('callRecords',encode:(e)=>e.toJson(),decode:CallRecords.fromJson);
    if(await Hive.boxExists('favoriteEmoji')){
      final legacy=await Hive.openBox<List>('favoriteEmoji');
      try{for(final key in legacy.keys){await favorite.put(key.toString(),(legacy.get(key)??[]).cast<EmojiInfo>());}}
      finally{await legacy.close();}
      await Hive.deleteBoxFromDisk('favoriteEmoji');
    }
    if(await Hive.boxExists('callRecords')){
      final legacy=await Hive.openBox<List>('callRecords');
      try{for(final key in legacy.keys){await calls.put(key.toString(),(legacy.get(key)??[]).cast<CallRecords>());}}
      finally{await legacy.close();}
      await Hive.deleteBoxFromDisk('callRecords');
    }
  }
  @override
  void onInit(){
    _cacheAccount=DataSp.userID;
    _vaultInvalidated=NativeLocalVault.invalidated.listen((_){final account=DataSp.userID;if(!NativeLocalVault.unlocked||account!=_cacheAccount){favoriteList.clear();callRecordList.clear();_isInitFavoriteList=false;_isInitCallRecords=false;}_cacheAccount=account;});
    favoriteBox=EncryptedCacheBox<EmojiInfo>('favoriteEmoji',encode:(e)=>{'url':e.url,'width':e.width,'height':e.height},decode:(e)=>EmojiInfo(url:e['url'],width:e['width'],height:e['height']));
    callRecordBox=EncryptedCacheBox<CallRecords>('callRecords',encode:(e)=>e.toJson(),decode:CallRecords.fromJson);
    super.onInit();
  }
}
