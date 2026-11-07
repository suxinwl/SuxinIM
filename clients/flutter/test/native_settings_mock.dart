import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

/// Test transport only. Production never falls back to plaintext settings when
/// its native keystore/Keychain channel is unavailable.
void installNativeSettingsMock() {
 var sealedState='{}';
 TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
   .setMockMethodCallHandler(const MethodChannel('puty/local_store'),(call)async{
     if(call.method=='preferences/read')return sealedState;
     if(call.method=='preferences/write'){sealedState=call.arguments as String;return true;}
     if(call.method=='lock')return true;
     throw MissingPluginException('Only synthetic native preferences are mocked');
   });
}
