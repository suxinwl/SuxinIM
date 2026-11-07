import 'dart:async';
import 'dart:typed_data';
import 'dart:ui';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/src/native_local_vault.dart';
import 'package:openim_common/src/models/login_certificate.dart';
import 'package:openim_common/src/utils/voice_record.dart';
import 'package:openim_common/src/utils/protected_crop_geometry.dart';

void main(){
 TestWidgetsFlutterBinding.ensureInitialized();
 final messenger=TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
 LoginCertificate certificate()=>LoginCertificate.fromJson({'userID':'synthetic-vault','imToken':'synthetic-im','chatToken':'synthetic-chat'});
 setUp((){messenger.setMockMethodCallHandler(NativeLocalVault.channel,(call)async=>call.method=='unlock'?{'userID':'synthetic-vault','actorGeneration':1}:true);});
 tearDown(()async{await NativeLocalVault.lock();messenger.setMockMethodCallHandler(NativeLocalVault.channel,null);});
 test('same native actor refresh retains active decoder generation',()async{
  await NativeLocalVault.unlock(certificate());final generation=NativeLocalVault.generation;
  await NativeLocalVault.unlock(certificate());expect(NativeLocalVault.generation,generation);expect(NativeLocalVault.unlocked,isTrue);
 });
 test('native SID epoch or entry change invalidates actor views',()async{
  await NativeLocalVault.unlock(certificate());final generation=NativeLocalVault.generation;
  messenger.setMockMethodCallHandler(NativeLocalVault.channel,(call)async=>call.method=='unlock'?{'userID':'synthetic-vault','actorGeneration':2}:true);
  await NativeLocalVault.unlock(certificate());expect(NativeLocalVault.generation,generation+1);
 });
 test('late rejected authorization cannot lock a newer successful actor',()async{
  final pending=Completer<Map<String,dynamic>>();var calls=0;
  messenger.setMockMethodCallHandler(NativeLocalVault.channel,(call)async{if(call.method!='unlock')return true;if(++calls==1)return pending.future;return {'userID':'synthetic-vault','actorGeneration':2};});
  final late=NativeLocalVault.unlock(certificate());final rejected=expectLater(late,throwsStateError);
  await Future<void>.delayed(Duration.zero);await NativeLocalVault.lock();await NativeLocalVault.unlock(certificate());final generation=NativeLocalVault.generation;
  pending.complete({'userID':'synthetic-vault','actorGeneration':1});await rejected;
  expect(NativeLocalVault.unlocked,isTrue);expect(NativeLocalVault.generation,generation);
 });
 test('malformed native generation fails closed',()async{
  messenger.setMockMethodCallHandler(NativeLocalVault.channel,(call)async=>call.method=='unlock'?{'userID':'synthetic-vault','actorGeneration':1.0}:true);
  await expectLater(NativeLocalVault.unlock(certificate()),throwsStateError);expect(NativeLocalVault.unlocked,isFalse);
 });
 test('background native lock clears views and permits only fresh online unlock',()async{
  NativeLocalVault.install();await NativeLocalVault.unlock(certificate());final before=NativeLocalVault.generation;
  final soft=NativeLocalVault.backgroundLocked.first,invalidated=NativeLocalVault.invalidated.first;var hardLocks=0;final hard=NativeLocalVault.locked.listen((_){hardLocks++;});
  final delivered=Completer<void>();ServicesBinding.instance.channelBuffers.push(NativeLocalVault.channel.name,const StandardMethodCodec().encodeMethodCall(const MethodCall('onVaultLocked','BACKGROUND_LOCK')),(_){delivered.complete();});
  await delivered.future;await soft;await invalidated;
  expect(NativeLocalVault.unlocked,isFalse);expect(NativeLocalVault.generation,greaterThan(before));expect(hardLocks,0);
  await NativeLocalVault.unlock(certificate());expect(NativeLocalVault.unlocked,isTrue);await hard.cancel();
 });
 test('RAM voice WAV carries correct PCM format and independent payload',(){
  final pcm=Uint8List.fromList([1,2,3,4]);final wav=encodeProtectedWav(pcm);final view=ByteData.sublistView(wav);
  expect(String.fromCharCodes(wav.sublist(0,4)),'RIFF');expect(view.getUint32(4,Endian.little),40);expect(view.getUint32(24,Endian.little),16000);expect(view.getUint16(34,Endian.little),16);expect(view.getUint32(40,Endian.little),4);
  pcm.fillRange(0,pcm.length,0);expect(wav.sublist(44),[1,2,3,4]);
 });
 test('crop square on landscape centers without stretching',(){expect(protectedCropRect(const Size(400,200),1,1,const Offset(.5,.5)),const Rect.fromLTWH(100,0,200,200));});
 test('crop original aspect and zoom preserve dimensions',(){expect(protectedCropRect(const Size(400,200),0,2,const Offset(.5,.5)),const Rect.fromLTWH(100,50,200,100));});
 test('crop portrait aspect remains within source',(){expect(protectedCropRect(const Size(200,400),.5,1,const Offset(.5,.5)),const Rect.fromLTWH(0,0,200,400));});
 test('crop pan clamps at image edge',(){expect(protectedCropRect(const Size(400,200),1,1,const Offset(-2,8)),const Rect.fromLTWH(0,0,200,200));});
 test('crop rejects invalid coordinates and excessive zoom',(){expect(()=>protectedCropRect(const Size(400,200),1,5,const Offset(.5,.5)),throwsArgumentError);expect(()=>protectedCropRect(const Size(400,200),1,1,const Offset(double.nan,.5)),throwsArgumentError);});
}
