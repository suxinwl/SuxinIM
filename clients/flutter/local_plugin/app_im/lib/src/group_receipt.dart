import '../app_im.dart';

class GroupMessageReceipt {
  late String conversationID;

  List<GroupMessageReadInfo> groupMessageReadInfo = [];

  GroupMessageReceipt.fromJson(Map<String, dynamic> json) {
    if (json['groupMessageReadInfo'] == null) {
      groupMessageReadInfo = [];
    } else {
      groupMessageReadInfo = (json['groupMessageReadInfo'] as List)
          .map((e) => GroupMessageReadInfo.fromJson(e))
          .toList();
    }
    conversationID = json['conversationID'];
  }

  Map<String, dynamic> toJson() {
    final data = Map<String, dynamic>();
    data['groupMessageReadInfo'] = this.groupMessageReadInfo;
    data['conversationID'] = this.conversationID;
    return data;
  }
}

/// 群消息已读信息
class GroupMessageReadInfo {
  /// 已读的用户列表
  List<GroupMembersInfo>? readMembers;

  /// 已读总数
  int hasReadCount = 0;

  /// 发送此条消息时的群人数
  int unreadCount = 0;

  late String clientMsgID;

  GroupMessageReadInfo.fromJson(Map<String, dynamic> json) {
    if (json['readMembers'] == null) {
      readMembers = [];
    } else {
      readMembers = (json['readMembers'] as List)
          .map((e) => GroupMembersInfo.fromJson(e))
          .toList();
    }
    hasReadCount = json['hasReadCount'];
    unreadCount = json['unreadCount'];
    clientMsgID = json['clientMsgID'];
  }

  Map<String, dynamic> toJson() {
    final data = Map<String, dynamic>();
    data['readMembers'] = this.readMembers;
    data['hasReadCount'] = this.hasReadCount;
    data['unreadCount'] = this.unreadCount;
    data['clientMsgID'] = this.clientMsgID;
    return data;
  }
}
