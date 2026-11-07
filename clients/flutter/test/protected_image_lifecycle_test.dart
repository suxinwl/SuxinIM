import 'dart:async';
import 'dart:typed_data';
import 'dart:ui' as ui;
import 'package:flutter/painting.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/src/native_local_vault.dart';
import 'package:openim_common/src/models/login_certificate.dart';
import 'package:openim_common/src/utils/protected_resource_image.dart';

class _LateCodec implements ui.Codec {
  bool disposed = false;
  @override int get frameCount => 1;
  @override int get repetitionCount => 0;
  @override Future<ui.FrameInfo> getNextFrame() => throw StateError('Stale codec published');
  @override void dispose() { disposed = true; }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  setUp(() {
    messenger.setMockMethodCallHandler(NativeLocalVault.channel, (call) async {
      if (call.method == 'unlock') return {'userID':'synthetic-image','actorGeneration':1};
      if (call.method == 'source/read') return Uint8List.fromList([1,2,3,4]);
      return true;
    });
  });
  tearDown(() async { await NativeLocalVault.lock(); messenger.setMockMethodCallHandler(NativeLocalVault.channel, null); });
  test('cleanup during asynchronous decode disposes late codec before image publication', () async {
    await NativeLocalVault.unlock(LoginCertificate.fromJson({'userID':'synthetic-image','imToken':'synthetic','chatToken':'synthetic'}));
    final decoded = Completer<ui.Codec>(), started = Completer<void>(), error = Completer<Object>();
    final provider = ProtectedResourceImage('/synthetic/encrypted-source');
    final stream = provider.loadImage(provider, (buffer,{ui.TargetImageSize Function(int,int)? getTargetSize}) {
      buffer.dispose(); started.complete(); return decoded.future;
    });
    var published=0;
    final listener=ImageStreamListener((image,synchronous){published++;},onError:(Object failure,StackTrace? stack){if(!error.isCompleted)error.complete(failure);});
    stream.addListener(listener);
    await started.future;
    await NativeLocalVault.clearScopes(['synthetic-conversation'],0,100);
    final codec=_LateCodec();decoded.complete(codec);
    expect(await error.future,isA<StateError>());
    expect(codec.disposed,isTrue);expect(published,0);
    stream.removeListener(listener);
  });
}
