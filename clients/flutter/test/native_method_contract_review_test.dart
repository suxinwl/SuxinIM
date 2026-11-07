import 'dart:async';
import 'dart:convert';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/src/models/login_certificate.dart';
import 'package:openim_common/src/native_local_vault.dart';

class ContractPicker extends FilePicker {
  @override
  Future<FilePickerResult?> pickFiles({String? dialogTitle,String? initialDirectory,
    FileType type=FileType.any,List<String>? allowedExtensions,
    Function(FilePickerStatus)? onFileLoading,bool allowCompression=false,
    int compressionQuality=0,bool allowMultiple=false,bool withData=false,
    bool withReadStream=false,bool lockParentWindow=false,bool readSequential=false}) async =>
      FilePickerResult([PlatformFile(path:'puty-media:synthetic-contract-source',name:'synthetic.xlsx',size:7)]);
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger=TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  final certificate=LoginCertificate.fromJson({'userID':'synthetic-contract','imToken':'synthetic-im','chatToken':'synthetic-chat'});
  tearDown(() async {
    messenger.setMockMethodCallHandler(NativeLocalVault.channel,(_)async=>true);
    await NativeLocalVault.lock();
    messenger.setMockMethodCallHandler(NativeLocalVault.channel,null);
  });

  test('actual MethodChannel invokeMethod<void> accepts both native Boolean envelopes',()async{
    const channel=MethodChannel('puty/review-void-contract');
    try {
      for(final nativeValue in [true,false]) {
        messenger.setMockMethodCallHandler(channel,(_)async=>nativeValue);
        await expectLater(channel.invokeMethod<void>('synthetic'),completes);
      }
    }finally{messenger.setMockMethodCallHandler(channel,null);}
  });

  test('production discard<void> does not throw on native true or false',()async{
    for(final nativeValue in [true,false]) {
      messenger.setMockMethodCallHandler(NativeLocalVault.channel,(call)async{
        expect(call.method,'source/discard');
        expect(call.arguments,{'path':'puty-media:synthetic-contract-source'});
        return nativeValue;
      });
      await expectLater(NativeLocalVault.discard('puty-media:synthetic-contract-source'),completes);
    }
  });

  for(final changeActor in [false,true]) {
    test('pending import ${changeActor?'rejects changed':'survives same'} actor unlock',()async{
      final importReply=Completer<String>(),started=Completer<void>();
      var actor=1,discards=0;
      messenger.setMockMethodCallHandler(NativeLocalVault.channel,(call)async{
        if(call.method=='unlock')return {'userID':'synthetic-contract','actorGeneration':actor};
        if(call.method=='attachment/import'){started.complete();return importReply.future;}
        if(call.method=='source/discard'){discards++;return true;}
        return true;
      });
      FilePicker.platform=ContractPicker();
      await NativeLocalVault.unlock(certificate);
      final generation=NativeLocalVault.generation;
      final pending=NativeLocalVault.pickImportAttachment('synthetic-conversation');
      final rejection=changeActor?expectLater(pending,throwsStateError):null;
      await started.future;
      if(changeActor)actor=2;
      await NativeLocalVault.unlock(certificate);
      expect(NativeLocalVault.generation,generation+(changeActor?1:0));
      importReply.complete(jsonEncode({'conversationID':'synthetic-conversation','messageID':'','localImportID':'synthetic-import','firstImportedAt':1}));
      if(changeActor){await rejection;expect(discards,0);}
      else {expect((await pending)!['localImportID'],'synthetic-import');expect(discards,1);}
    });
  }
}
