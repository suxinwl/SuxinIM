import 'dart:io';
import 'package:package_info_plus/package_info_plus.dart';
import 'dart:convert';

import 'package:collection/collection.dart';
import 'package:dio/dio.dart';
import 'package:app_im/app_im.dart';
import 'package:openim_common/openim_common.dart';

class Apis {
  static Options get imTokenOptions => Options(headers: {'token': DataSp.imToken});

  static Options get chatTokenOptions => Options(headers: {'token': DataSp.chatToken});

  /// login
  static Future<LoginCertificate> login({
    String? areaCode,
    String? phoneNumber,
    String? email,
    String? password,
    String? verificationCode,
  }) async {
    return HrmSSO.login(email ?? phoneNumber ?? '',password ?? '');
  }

  /// register
  static Future<LoginCertificate> register({
    required String nickname,
    required String password,
    String? faceURL,
    String? areaCode,
    String? phoneNumber,
    String? email,
    int birth = 0,
    int gender = 1,
    required String verificationCode,
    String? invitationCode,
  }) async {
    throw StateError('账号及密码由人事系统统一管理');
  }

  /// reset password
  static Future<dynamic> resetPassword({
    String? areaCode,
    String? phoneNumber,
    String? email,
    required String password,
    required String verificationCode,
  }) async {
    throw StateError('账号及密码由人事系统统一管理');
  }

  /// change password
  static Future<bool> changePassword({
    required String userID,
    required String currentPassword,
    required String newPassword,
  }) async {
    throw StateError('账号及密码由人事系统统一管理');
  }

  /// update user info
  static Future<dynamic> updateUserInfo({
    required String userID,
    String? account,
    String? phoneNumber,
    String? areaCode,
    String? email,
    String? nickname,
    String? faceURL,
    int? gender,
    int? birth,
    int? level,
    int? allowAddFriend,
    int? allowBeep,
    int? allowVibration,
  }) async {
    if ([account,phoneNumber,areaCode,email,nickname,faceURL,gender,birth,level].any((x)=>x!=null)) {
      throw StateError('个人资料由人事系统统一管理');
    }
    Map<String, dynamic> param = {'userID': userID};
    void put(String key, dynamic value) {
      if (null != value) {
        param[key] = value;
      }
    }

    put('account', account);
    put('phoneNumber', phoneNumber);
    put('areaCode', areaCode);
    put('email', email);
    put('nickname', nickname);
    put('faceURL', faceURL);
    put('gender', gender);
    put('gender', gender);
    put('level', level);
    put('birth', birth);
    put('allowAddFriend', allowAddFriend);
    put('allowBeep', allowBeep);
    put('allowVibration', allowVibration);

    return HttpUtil.post(
      Urls.updateUserInfo,
      data: {
        ...param,
        'platform': IMUtils.getPlatform(),
        // 'operationID': operationID,
      },
      options: chatTokenOptions,
    );
  }

  static Future<List<FriendInfo>> searchFriendInfo(
    String keyword, {
    int pageNumber = 1,
    int showNumber = 10,
  }) async {
    final data = await HttpUtil.post(
      Urls.searchFriendInfo,
      data: {
        'pagination': {'pageNumber': pageNumber, 'showNumber': showNumber},
        'keyword': keyword,
      },
      options: chatTokenOptions,
    );
    if (data['users'] is List) {
      return (data['users'] as List).map((e) => FriendInfo.fromJson(e)).toList();
    }
    return [];
  }

  static Future<List<UserFullInfo>?> getUserFullInfo({
    int pageNumber = 0,
    int showNumber = 10,
    required List<String> userIDList,
  }) async {
    final data = await HttpUtil.post(
      Urls.getUsersFullInfo,
      data: {
        'pagination': {'pageNumber': pageNumber, 'showNumber': showNumber},
        'userIDs': userIDList,
        'platform': IMUtils.getPlatform(),
        // 'operationID': operationID,
      },
      options: chatTokenOptions,
    );
    if (data['users'] is List) {
      return (data['users'] as List).map((e) => UserFullInfo.fromJson(e)).toList();
    }
    return null;
  }

  static Future<List<UserFullInfo>?> searchUserFullInfo({
    required String content,
    int pageNumber = 1,
    int showNumber = 10,
  }) async {
    final data = await HttpUtil.post(
      Urls.searchUserFullInfo,
      data: {
        'pagination': {'pageNumber': pageNumber, 'showNumber': showNumber},
        'keyword': content,
        // 'operationID': operationID,
      },
      options: chatTokenOptions,
    );
    if (data['users'] is List) {
      return (data['users'] as List).map((e) => UserFullInfo.fromJson(e)).toList();
    }
    return null;
  }

  static Future<UserFullInfo?> queryMyFullInfo() async {
    final list = await Apis.getUserFullInfo(
      userIDList: [AppIm.iMManager.userID],
    );
    return list?.firstOrNull;
  }

  /// 获取验证码
  /// [usedFor] 1：注册，2：重置密码 3：登录
  static Future<bool> requestVerificationCode({
    String? areaCode,
    String? phoneNumber,
    String? email,
    required int usedFor,
    String? invitationCode,
  }) async {
    return HttpUtil.post(
      Urls.getVerificationCode,
      data: {"areaCode": areaCode, "phoneNumber": phoneNumber, "email": email, 'usedFor': usedFor, 'invitationCode': invitationCode},
    ).then((value) {
      IMViews.showToast(StrRes.sentSuccessfully);
      return true;
    }).catchError((e, s) {
      Logger.print('e:$e s:$s');
      return false;
    });
  }

  /// 校验验证码
  static Future<dynamic> checkVerificationCode({
    String? areaCode,
    String? phoneNumber,
    String? email,
    required String verificationCode,
    required int usedFor,
    String? invitationCode,
  }) {
    return HttpUtil.post(
      Urls.checkVerificationCode,
      data: {
        "phoneNumber": phoneNumber,
        "areaCode": areaCode,
        "email": email,
        "verifyCode": verificationCode,
        "usedFor": usedFor,
        // 'operationID': operationID,
        'invitationCode': invitationCode
      },
    );
  }

  /// Chat is the version authority; enterprise metadata supplies the full-package hash.
  static Future<UpgradeInfoV2> checkUpgradeV2() async {
    final info = await PackageInfo.fromPlatform();
    final data = await AppIm.enterprise.request('/application/latest', {
      'platform': Platform.isAndroid ? 'android' : 'ios',
      'version': info.version, 'build': int.tryParse(info.buildNumber) ?? 0,
    });
    if(data['available'] != true) return UpgradeInfoV2(buildHaveNewVersion:false,needForceUpdate:false);
    return UpgradeInfoV2.fromJson({
      'buildHaveNewVersion':true,'buildVersion':data['version'],
      'buildVersionNo':'${data['build']}','needForceUpdate':data['force'] == true,
      'downloadURL':data['url'],'buildUpdateDescription':data['text'],
      'sha256':data['sha256'],'size':data['size'],
    });
  }

  static void queryUserOnlineStatus({
    required List<String> uidList,
    Function(Map<String, String>)? onlineStatusDescCallback,
    Function(Map<String, bool>)? onlineStatusCallback,
  }) async {
    var resp = await HttpUtil.post(
      Urls.userOnlineStatus,
      data: <String, dynamic>{"userIDList": uidList},
      options: imTokenOptions,
    );
    Map<String, dynamic> map = resp.data!;
    if (map['errCode'] == 0 && map['data'] is List) {
      _handleStatus(
        (map['data'] as List).map((e) => OnlineStatus.fromJson(e)).toList(),
        onlineStatusCallback: onlineStatusCallback,
        onlineStatusDescCallback: onlineStatusDescCallback,
      );
    }
  }

  /// discoverPageURL
  /// ordinaryUserAddFriend,
  /// bossUserID,
  /// adminURL ,
  /// allowSendMsgNotFriend
  /// needInvitationCodeRegister
  /// robots
  static Future<Map<String, dynamic>> getClientConfig() async {
    var result = await HttpUtil.post(
      Urls.getClientConfig,
      data: {
        // 'operationID': operationID,
      },
      options: chatTokenOptions,
      showErrorToast: false,
    );
    return result['config'] ?? {};
  }

  /// Existing public route, intentionally without an account credential. Only
  /// display fields are consumed before login; no management route is opened.
  static Future<Map<String, dynamic>> getPublicBrand() async {
    final origin = Config.serverBaseUrl;
    final response = await dio.post<Map<String, dynamic>>('$origin/chat/client_config/get', data: {}, options: Options(
      headers: {'operationID': HttpUtil.operationID, 'Content-Type': 'application/json'}, followRedirects: false,
    ));
    final value = response.data;
    if (value == null || value['errCode'] != 0 || origin != Config.serverBaseUrl) throw StateError('Public brand is unavailable');
    final config = value['data']?['config'];
    final result = <String, dynamic>{};
    for (final key in ['appName', 'webTitle']) {
      final text = config is Map ? config[key] : null;
      if (text is String && text.trim().isNotEmpty && text.length <= 128 && !RegExp(r'[\x00-\x1f\x7f]').hasMatch(text)) result[key] = text.trim();
    }
    return result;
  }

  static _handleStatus(
    List<OnlineStatus> list, {
    Function(Map<String, String>)? onlineStatusDescCallback,
    Function(Map<String, bool>)? onlineStatusCallback,
  }) {
    final statusDesc = <String, String>{};
    final status = <String, bool>{};
    for (var e in list) {
      if (e.status == 'online') {
        // IOSPlatformStr     = "IOS"
        // AndroidPlatformStr = "Android"
        // WindowsPlatformStr = "Windows"
        // OSXPlatformStr     = "OSX"
        // WebPlatformStr     = "Web"
        // MiniWebPlatformStr = "MiniWeb"
        // LinuxPlatformStr   = "Linux"
        final pList = <String>[];
        for (var platform in e.detailPlatformStatus!) {
          if (platform.platform == "Android" || platform.platform == "IOS") {
            pList.add(StrRes.phoneOnline);
          } else if (platform.platform == "Windows") {
            pList.add(StrRes.pcOnline);
          } else if (platform.platform == "Web") {
            pList.add(StrRes.webOnline);
          } else if (platform.platform == "MiniWeb") {
            pList.add(StrRes.webMiniOnline);
          } else {
            statusDesc[e.userID!] = StrRes.online;
          }
        }
        statusDesc[e.userID!] = '${pList.join('/')}${StrRes.online}';
        status[e.userID!] = true;
      } else {
        statusDesc[e.userID!] = StrRes.offline;
        status[e.userID!] = false;
      }
    }
    onlineStatusDescCallback?.call(statusDesc);
    onlineStatusCallback?.call(status);
  }

  static Future<List<UniMPInfo>> queryUniMPList() async {
    final result=await AppIm.enterprise.request('/discovery/list',{});
    final items=result['items'] as List? ?? [];
    return items.whereType<Map>().where((item)=>item['enabled']!=false).map((item)=>UniMPInfo.fromJson({
      'id':item['id'],'name':item['title'],'url':item['url'],'icon':item['icon'],
    })).toList();
  }

  /// 查询tag组
  static Future<TagGroup> getUserTags({String? userID}) => HttpUtil.post(
        Urls.getUserTags,
        data: {'userID': userID},
        options: chatTokenOptions,
      ).then((value) => TagGroup.fromJson(value));

  /// 创建tag
  static createTag({
    required String tagName,
    required List<String> userIDList,
  }) =>
      HttpUtil.post(
        Urls.createTag,
        data: {'tagName': tagName, 'userIDs': userIDList},
        options: chatTokenOptions,
      );

  /// 创建tag
  static deleteTag({required String tagID}) => HttpUtil.post(
        Urls.deleteTag,
        data: {'tagID': tagID},
        options: chatTokenOptions,
      );

  /// 创建tag
  static updateTag({
    required String tagID,
    required String name,
    required List<String> increaseUserIDList,
    required List<String> reduceUserIDList,
  }) =>
      HttpUtil.post(
        Urls.updateTag,
        data: {
          'tagID': tagID,
          'name': name,
          'addUserIDs': increaseUserIDList,
          'delUserIDs': reduceUserIDList,
        },
        options: chatTokenOptions,
      );

  /// 下发tag通知
  static sendTagNotification({
    // required int contentType,
    TextElem? textElem,
    SoundElem? soundElem,
    PictureElem? pictureElem,
    VideoElem? videoElem,
    FileElem? fileElem,
    CardElem? cardElem,
    LocationElem? locationElem,
    List<String> tagIDList = const [],
    List<String> userIDList = const [],
    List<String> groupIDList = const [],
  }) async {
    return HttpUtil.post(
      Urls.sendTagNotification,
      data: {
        'tagIDs': tagIDList,
        'userIDs': userIDList,
        'groupIDs': groupIDList,
        'senderPlatformID': IMUtils.getPlatform(),
        'content': json.encode({
          'data': json.encode({
            "customType": CustomMessageType.tag,
            "data": {
              // 'contentType': contentType,
              'pictureElem': pictureElem?.toJson(),
              'videoElem': videoElem?.toJson(),
              'fileElem': fileElem?.toJson(),
              'cardElem': cardElem?.toJson(),
              'locationElem': locationElem?.toJson(),
              'soundElem': soundElem?.toJson(),
              'textElem': textElem?.toJson(),
            },
          }),
          'extension': '',
          'description': '',
        }),
      },
      options: chatTokenOptions,
    );
  }

  /// 获取tag通知列表
  static Future<List<TagNotification>> getTagNotificationLog({
    String? userID,
    required int pageNumber,
    required int showNumber,
  }) async {
    final result = await HttpUtil.post(
      Urls.getTagNotificationLog,
      data: {
        'userID': userID,
        'pagination': {'pageNumber': pageNumber, 'showNumber': showNumber},
      },
      options: chatTokenOptions,
    );
    final list = result['tagSendLogs'];
    if (list is List) {
      return list.map((e) => TagNotification.fromJson(e)).toList();
    }
    return [];
  }

  static delTagNotificationLog({
    required List<String> ids,
  }) =>
      HttpUtil.post(
        Urls.delTagNotificationLog,
        data: <String, dynamic>{'ids': ids},
        options: chatTokenOptions,
      );
}
