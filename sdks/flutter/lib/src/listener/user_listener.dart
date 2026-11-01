import 'package:flutter_openim_sdk/flutter_openim_sdk.dart';

/// Current User Profile Listener
class OnUserListener {
  /// The information of the logged-in user has been updated
  Function(UserInfo info)? onSelfInfoUpdated;
  Function(String command)? onUserCommandAdd;
  Function(String command)? onUserCommandDelete;
  Function(String command)? onUserCommandUpdate;
  Function(UserStatusInfo info)? onUserStatusChanged;

  OnUserListener({
    this.onSelfInfoUpdated,
    this.onUserCommandAdd,
    this.onUserCommandDelete,
    this.onUserCommandUpdate,
    this.onUserStatusChanged,
  });

  /// Callback for changes in user's own information
  void selfInfoUpdated(UserInfo info) {
    onSelfInfoUpdated?.call(info);
  }

  void userCommandAdd(String command) {
    onUserCommandAdd?.call(command);
  }

  void userCommandDelete(String command) {
    onUserCommandDelete?.call(command);
  }

  void userCommandUpdate(String command) {
    onUserCommandUpdate?.call(command);
  }

  /// Callback for changes in user status
  void userStatusChanged(UserStatusInfo info) {
    onUserStatusChanged?.call(info);
  }
}
