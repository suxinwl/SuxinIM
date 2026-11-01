import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:wakelock_plus/wakelock_plus.dart';
import 'package:wakelock_plus_platform_interface/messages.g.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('wakelock Pigeon10 channels and message tags match native1.1.4',
      () async {
    final codec = WakelockPlusApi.codec;
    final toggles = <bool>[];
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockDecodedMessageHandler<Object?>(
      BasicMessageChannel<Object?>(
          'dev.flutter.pigeon.WakelockPlusApi.toggle', codec),
      (message) async {
        toggles.add(((message as List).single as ToggleMessage).enable!);
        return <Object?>[null];
      },
    );
    messenger.setMockDecodedMessageHandler<Object?>(
      BasicMessageChannel<Object?>(
          'dev.flutter.pigeon.WakelockPlusApi.isEnabled', codec),
      (_) async => <Object?>[IsEnabledMessage(enabled: toggles.last)],
    );
    addTearDown(() {
      messenger.setMockDecodedMessageHandler<Object?>(
          BasicMessageChannel<Object?>(
              'dev.flutter.pigeon.WakelockPlusApi.toggle', codec),
          null);
      messenger.setMockDecodedMessageHandler<Object?>(
          BasicMessageChannel<Object?>(
              'dev.flutter.pigeon.WakelockPlusApi.isEnabled', codec),
          null);
    });
    // Native Messages.g.kt and iOS messages.g.m use128/129 in this version.
    expect(
        codec.encodeMessage(IsEnabledMessage(enabled: true))!.getUint8(0), 128);
    expect(codec.encodeMessage(ToggleMessage(enable: true))!.getUint8(0), 129);
    await WakelockPlus.enable();
    expect(await WakelockPlus.enabled, isTrue);
    await WakelockPlus.disable();
    expect(await WakelockPlus.enabled, isFalse);
    expect(toggles, <bool>[true, false]);
  });
}
