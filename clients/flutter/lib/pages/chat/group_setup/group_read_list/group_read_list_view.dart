import 'package:flutter/material.dart';
import 'package:app_im/app_im.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:get/get.dart';
import 'package:openim_common/openim_common.dart';
import 'package:pull_to_refresh/pull_to_refresh.dart';
import 'package:sprintf/sprintf.dart';

import 'group_read_list_logic.dart';

class GroupReadListPage extends StatelessWidget {
  final logic = Get.find<GroupReadListLogic>();

  GroupReadListPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: TitleBar.back(title: StrRes.messageRecipientList),
      backgroundColor: Styles.c_F8F9FA,
      body: Obx(() => Column(
            children: [
              CustomTabBar(
                labels: [
                  sprintf(StrRes.unreadCount, [logic.unreadCount ?? '--']),
                  sprintf(StrRes.hasReadCount, [logic.hasReadCount ?? '--']),
                ],
                index: logic.index.value,
                onTabChanged: (i) => logic.switchTab(i),
                showUnderline: true,
              ),
              Expanded(
                child: IndexedStack(
                  index: logic.index.value,
                  children: [
                    _buildMembers(hasRead: false),
                    _buildMembers(hasRead: true),
                  ],
                ),
              ),
            ],
          )),
    );
  }

  Widget _buildMembers({required bool hasRead}) {
    final state = hasRead ? logic.hasReadState.value : logic.unreadState.value;
    final tab = hasRead ? 'read' : 'unread';
    if (state == GroupReadLoadState.initial ||
        state == GroupReadLoadState.loading) {
      return Center(
        key: ValueKey('group-read-$tab-loading'),
        child: const CircularProgressIndicator(),
      );
    }
    if (state == GroupReadLoadState.failed) {
      return Center(
        key: ValueKey('group-read-$tab-error'),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(StrRes.groupReadLoadFailed),
            TextButton(
              key: ValueKey('group-read-$tab-retry'),
              onPressed: () => hasRead
                  ? logic.queryHasReadMembersList()
                  : logic.queryUnreadMemberList(),
              child: Text(StrRes.groupReadRetry),
            ),
          ],
        ),
      );
    }
    final members = hasRead ? logic.hasReadMemberList : logic.unreadMemberList;
    if (members.isEmpty) return Center(child: Text(StrRes.groupReadEmpty));
    return SmartRefresher(
      controller: hasRead
          ? logic.hasReadRefreshController
          : logic.unreadRefreshController,
      enablePullDown: false,
      enablePullUp: true,
      footer: IMViews.buildFooter(),
      child: ListView.builder(
        itemCount: members.length,
        itemBuilder: (_, index) => _buildItemView(members[index]),
      ),
    );
  }

  Widget _buildItemView(GroupMembersInfo info, {String? status}) => Container(
        height: 64.h,
        padding: EdgeInsets.symmetric(horizontal: 16.w),
        color: Styles.c_FFFFFF,
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            AvatarView(
              width: 44.w,
              height: 44.h,
              url: info.faceURL,
              text: info.nickname,
            ),
            10.horizontalSpace,
            Expanded(
              child: (info.nickname ?? '').toText
                ..style = Styles.ts_0C1C33_17sp
                ..maxLines = 1
                ..overflow = TextOverflow.ellipsis,
            ),
          ],
        ),
      );
}
