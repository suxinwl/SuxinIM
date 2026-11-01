import '../app_im.dart';

extension EnterpriseMessageManager on MessageManager {
  Future<void> sendGroupMessageReadReceipt(
    String conversationID,
    List<String> clientMsgIDs, {
    String? operationID,
  }) async {
    final messages = await findMessageList(
      searchParams: [
        SearchParams(
          conversationID: conversationID,
          clientMsgIDList: clientMsgIDs,
        ),
      ],
    );
    final list =
        messages.findResultItems
            ?.expand((e) => e.messageList ?? <Message>[])
            .toList() ??
        <Message>[];
    if (list.isEmpty)
      throw StateError('Read receipt requires a real local message');
    await AppIm.enterprise.request('/msg/mark_group_message_read', {
      'conversationID': conversationID,
      'groupID': list.first.groupID,
      'clientMsgs': {for (final msg in list) msg.clientMsgID!: msg.sendID},
      'seqs': {for (final msg in list) msg.clientMsgID!: msg.seq},
    });
  }

  Future<List<GroupMembersInfo>> getGroupMessageReaderList(
    String conversationID,
    String clientMsgID, {
    int filter = 0,
    int offset = 0,
    int count = 20,
    String? operationID,
  }) async {
    final messages = await findMessageList(
      searchParams: [
        SearchParams(
          conversationID: conversationID,
          clientMsgIDList: [clientMsgID],
        ),
      ],
    );
    final message =
        messages.findResultItems?.firstOrNull?.messageList?.firstOrNull;
    final groupID = message?.groupID;
    if (groupID == null)
      throw StateError('Group message is not available locally');
    final ids = <String>[];
    var next = offset;
    while (ids.length < count) {
      final limit = (count - ids.length).clamp(1, 200);
      final data = await AppIm.enterprise
          .request('/msg/get_group_message_has_read', {
            'conversationID': conversationID,
            'groupID': groupID,
            'clientMsgID': clientMsgID,
            'seq': message!.seq,
            'sendID': message.sendID,
            'filter': filter,
            'offset': next,
            'count': limit,
          });
      final reads = data['reads'] as List? ?? [];
      ids.addAll(reads.map((e) => e['userID'] as String));
      next += reads.length;
      if (reads.isEmpty || next >= ((data['count'] as num?)?.toInt() ?? next))
        break;
    }
    if (ids.isEmpty) return [];
    final current = <GroupMembersInfo>[];
    final profiles = <PublicUserInfo>[];
    for (var i = 0; i < ids.length; i += 100) {
      final chunk = ids.sublist(i, (i + 100).clamp(0, ids.length));
      current.addAll(
        await AppIm.iMManager.groupManager.getGroupMembersInfo(
          groupID: groupID,
          userIDList: chunk,
        ),
      );
      profiles.addAll(
        await AppIm.iMManager.userManager.getUsersInfo(userIDList: chunk),
      );
    }
    return ids.map((id) {
      final member = current.where((m) => m.userID == id).firstOrNull;
      if (member != null) return member;
      final profile = profiles.where((u) => u.userID == id).firstOrNull;
      return GroupMembersInfo.fromJson({
        ...?profile?.toJson(),
        'groupID': groupID,
        'userID': id,
        'nickname': profile?.nickname ?? id,
      });
    }).toList();
  }
}

extension EnterpriseConversationAdapter on ConversationManager {
  Future<dynamic> setConversationIsMsgDestruct({
    required String conversationID,
    required bool isMsgDestruct,
    String? operationID,
  }) => setConversation(
    conversationID,
    ConversationReq(isMsgDestruct: isMsgDestruct),
    operationID: operationID,
  );
  Future<dynamic> setConversationMsgDestructTime({
    required String conversationID,
    required int duration,
    String? operationID,
  }) => setConversation(
    conversationID,
    ConversationReq(msgDestructTime: duration),
    operationID: operationID,
  );
}
