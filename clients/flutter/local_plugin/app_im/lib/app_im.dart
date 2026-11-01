library app_im;

export 'package:flutter_openim_sdk/flutter_openim_sdk.dart';
export 'src/meeting_info.dart';
export 'src/signaling_info.dart';
export 'src/signaling_listener.dart';
export 'src/enterprise_manager.dart';
export 'src/group_receipt.dart';
export 'src/message_adapter.dart';
export 'src/public_message_adapter.dart';
export 'src/call_termination.dart';
import 'package:flutter_openim_sdk/flutter_openim_sdk.dart';
import 'src/enterprise_manager.dart';
import 'src/public_message_adapter.dart';

/// One application boundary around the pinned public SDK and enterprise APIs.
class AppIm {
  static const sdkVersion = '3.8.3+hotfix.15';
  static bool _adapted = false;
  static final receiptKnown = <String>{};
  static IMManager get iMManager {
    final native = OpenIM.iMManager;
    if (!_adapted) {
      native.messageManager = PublicMessageAdapter();
      _adapted = true;
    }
    return native;
  }

  static final enterprise = EnterpriseManager();
}

extension EnterpriseIMManager on IMManager {
  EnterpriseManager get signalingManager => AppIm.enterprise;
}
