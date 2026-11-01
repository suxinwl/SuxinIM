import 'dart:convert';
import 'package:flutter/services.dart';
import '../app_im.dart';

/// Uses the pinned SDK channel and its original listener registration. No second engine.
class PublicMessageAdapter extends MessageManager {
  PublicMessageAdapter() : super(const MethodChannel('flutter_openim_sdk'));
  @override
  Future<void> typingStatusUpdate({
    required String userID,
    String? msgTip,
    String? operationID,
  }) async {
    final manager = AppIm.iMManager;
    final actor = manager.userID;
    final token = manager.token;
    if (!manager.isLogined || token == null) return;
    final conversation = await manager.conversationManager.getOneConversation(
      sourceID: userID,
      sessionType: ConversationType.single,
      operationID: operationID,
    );
    if (!manager.isLogined || manager.token != token || manager.userID != actor)
      return;
    await manager.conversationManager.changeInputStates(
      conversationID: conversation.conversationID,
      focus: msgTip == 'yes',
      operationID: operationID,
    );
  }

  // The SDK's cleaner mutates nested maps; constructor defaults can contain const exMap.
  static Message _mutableMessage(Message source) =>
      Message.fromJson(jsonDecode(jsonEncode(source.toJson())));
  @override
  Future<Message> createQuoteMessage({
    required String text,
    required Message quoteMsg,
    String? operationID,
  }) {
    if (quoteMsg.sendID?.isNotEmpty != true ||
        quoteMsg.clientMsgID?.isNotEmpty != true ||
        quoteMsg.status != 2) {
      return Future.error(
        ArgumentError('Only a successfully sent message can be quoted'),
      );
    }
    return super.createQuoteMessage(
      text: text,
      quoteMsg: _mutableMessage(quoteMsg),
      operationID: operationID,
    );
  }

  @override
  Future<Message> sendMessage({
    required Message message,
    required OfflinePushInfo offlinePushInfo,
    String? userID,
    String? groupID,
    bool isOnlineOnly = false,
    String? operationID,
  }) async {
    await registerMeetingInvitation(message, userID: userID, groupID: groupID);
    // Quote carries its own referenced-message snapshot. Patch15's normal
    // send path dereferences a missing destination-DB lookup; the official
    // NotOss path serializes that snapshot while retaining SDK send/persistence.
    final send = message.contentType == MessageType.quote
        ? super.sendMessageNotOss
        : super.sendMessage;
    final result = await send(
      message: _mutableMessage(message),
      offlinePushInfo: offlinePushInfo,
      userID: userID,
      groupID: groupID,
      isOnlineOnly: isOnlineOnly,
      operationID: operationID,
    );
    await hydrate([result]);
    return result;
  }

  @override
  Future<AdvancedMessage> getAdvancedHistoryMessageList({
    String? conversationID,
    Message? startMsg,
    GetHistoryViewType viewType = GetHistoryViewType.history,
    int? count,
    String? operationID,
  }) async {
    final result = await super.getAdvancedHistoryMessageList(
      conversationID: conversationID,
      startMsg: startMsg,
      viewType: viewType,
      count: count,
      operationID: operationID,
    );
    await hydrate(result.messageList ?? [], conversationID: conversationID);
    return result;
  }

  @override
  Future<AdvancedMessage> getAdvancedHistoryMessageListReverse({
    String? conversationID,
    Message? startMsg,
    GetHistoryViewType viewType = GetHistoryViewType.history,
    int? count,
    String? operationID,
  }) async {
    final result = await super.getAdvancedHistoryMessageListReverse(
      conversationID: conversationID,
      startMsg: startMsg,
      viewType: viewType,
      count: count,
      operationID: operationID,
    );
    await hydrate(result.messageList ?? [], conversationID: conversationID);
    return result;
  }

  static Future<void> registerMeetingInvitation(
    Message message, {
    String? userID,
    String? groupID,
  }) async {
    if (message.contentType == MessageType.custom &&
        message.customElem?.data != null) {
      final body = jsonDecode(message.customElem!.data!);
      if (body is Map && body['customType'] == 905) {
        final data = body['data'] is String
            ? jsonDecode(body['data'])
            : body['data'];
        if (data is! Map || data['id'] is! String)
          throw StateError('Invalid meeting card');
        await AppIm.enterprise.request('/signaling/signal_invite_meeting', {
          'roomID': data['id'],
          'userIDs': userID?.isNotEmpty == true ? [userID] : <String>[],
          'groupIDs': groupID?.isNotEmpty == true ? [groupID] : <String>[],
        });
      }
    }
    for (final child in message.mergeElem?.multiMessage ?? <Message>[]) {
      await registerMeetingInvitation(child, userID: userID, groupID: groupID);
    }
  }

  static Future<void> hydrate(
    List<Message> messages, {
    String? conversationID,
  }) async {
    final groups = <String, List<Message>>{};
    for (final m in messages) {
      if (m.groupID?.isNotEmpty == true &&
          (m.seq ?? 0) > 0 &&
          m.clientMsgID != null) {
        groups.putIfAbsent(m.groupID!, () => []).add(m);
      }
    }
    for (final entry in groups.entries) {
      try {
        final cid =
            conversationID ??
            (await AppIm.iMManager.conversationManager.getOneConversation(
              sourceID: entry.key,
              sessionType:
                  entry.value.first.sessionType ?? ConversationType.group,
            )).conversationID;
        final data = await AppIm.enterprise.request(
          '/msg/get_group_message_read_num',
          {
            'conversationID': cid,
            'groupID': entry.key,
            'clientMsgIDs': entry.value.map((m) => m.clientMsgID).toList(),
            'seqs': {for (final m in entry.value) m.clientMsgID!: m.seq},
          },
        );
        final counts = data['num'] as Map? ?? {};
        for (final m in entry.value) {
          final value = counts[m.clientMsgID];
          if (value is! Map) continue;
          m.attachedInfoElem ??= AttachedInfoElem.fromJson({});
          m.attachedInfoElem!.groupHasReadInfo = GroupHasReadInfo.fromJson({
            'hasReadCount': value['readNum'],
            'unreadCount': value['unreadNum'],
          });
          AppIm.receiptKnown.add(m.clientMsgID!);
        }
      } catch (_) {
        // IM sending already succeeded. Receipt service errors must not turn it into a failed send.
        // The UI renders unknown receipt counts as -- and retries on the next history pull/event.
      }
    }
  }
}
