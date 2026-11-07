import 'dart:async';
import 'dart:typed_data';
import 'package:record/record.dart';
import '../native_local_vault.dart';
typedef RecordFc=Function(int sec,String path);

Uint8List encodeProtectedWav(Uint8List pcm){
 final wav=Uint8List(44+pcm.length);final header=ByteData.sublistView(wav);
 wav.setRange(0,4,'RIFF'.codeUnits);header.setUint32(4,36+pcm.length,Endian.little);wav.setRange(8,16,'WAVEfmt '.codeUnits);
 header.setUint32(16,16,Endian.little);header.setUint16(20,1,Endian.little);header.setUint16(22,1,Endian.little);header.setUint32(24,16000,Endian.little);header.setUint32(28,32000,Endian.little);header.setUint16(32,2,Endian.little);header.setUint16(34,16,Endian.little);wav.setRange(36,40,'data'.codeUnits);header.setUint32(40,pcm.length,Endian.little);wav.setRange(44,wav.length,pcm);return wav;
}
/// Production recording uses bounded PCM RAM, never AudioRecorder.start(path).
class VoiceRecord {
 final RecordFc onFinished,onInterrupt;final int maxRecordSec;final Function(int)? onDuration;
 final AudioRecorder _audioRecorder;final _parts=<Uint8List>[];
 StreamSubscription<Uint8List>? _capture;StreamSubscription<void>? _locked;Timer? _timer;
 Future<void>? _starting,_finishing;int _operation=0,_bytes=0,_started=0;bool _cancelled=false,_disposed=false;
 VoiceRecord({required this.maxRecordSec,required this.onInterrupt,required this.onFinished,this.onDuration,AudioRecorder? recorder}):_audioRecorder=recorder??AudioRecorder();
 Future<void> start(){final operation=++_operation;_cancelled=false;return _starting=_start(operation);}
 Future<void> _start(int operation)async{
  if(maxRecordSec<1||maxRecordSec>300)throw StateError('Invalid voice limit');
  _locked=NativeLocalVault.invalidated.listen((_)=>unawaited(cancel()));
  if(!NativeLocalVault.unlocked||!await _audioRecorder.hasPermission()||_cancelled||operation!=_operation)return;
  final stream=await _audioRecorder.startStream(const RecordConfig(encoder:AudioEncoder.pcm16bits,sampleRate:16000,numChannels:1));
  if(_cancelled||operation!=_operation||!NativeLocalVault.unlocked){await _audioRecorder.stop();return;}
  _started=DateTime.now().millisecondsSinceEpoch;
  _capture=stream.listen((data){if(_cancelled||operation!=_operation)return;if(_bytes+data.length>32000*(maxRecordSec+1)){unawaited(cancel());return;}_parts.add(Uint8List.fromList(data));_bytes+=data.length;},onError:(_)=>unawaited(cancel()));
  _timer=Timer.periodic(const Duration(seconds:1),(_){final sec=(DateTime.now().millisecondsSinceEpoch-_started)~/1000;onDuration?.call(sec);if(sec>=maxRecordSec)unawaited(stop(isInterrupt:true));});
 }
 Future<void> stop({bool isInterrupt=false})=>_finishing??=_finish(isInterrupt);
 Future<void> _finish(bool interrupted)async{
  final operation=_operation,vault=NativeLocalVault.generation;String? path;Uint8List? pcm,wav;
  _timer?.cancel();_timer=null;
  try{await _starting;await _capture?.cancel();_capture=null;await _audioRecorder.stop();
   if(_cancelled||operation!=_operation||!NativeLocalVault.unlocked||vault!=NativeLocalVault.generation||_bytes==0)return;
   pcm=Uint8List(_bytes);var offset=0;for(final part in _parts){pcm.setRange(offset,offset+part.length,part);offset+=part.length;part.fillRange(0,part.length,0);}_parts.clear();_bytes=0;
   wav=encodeProtectedWav(pcm);pcm.fillRange(0,pcm.length,0);path=await NativeLocalVault.sourceBytes(wav,'语音$_started.wav');
   if(_cancelled||operation!=_operation||vault!=NativeLocalVault.generation){await NativeLocalVault.discard(path);return;}
   final seconds=((wav.length-44)/32000).ceil();if(interrupted){onInterrupt(seconds,path);}else{onFinished(seconds,path);}
  }finally{pcm?.fillRange(0,pcm.length,0);wav?.fillRange(0,wav.length,0);_clear();await _locked?.cancel();_locked=null;}
 }
 void _clear(){for(final part in _parts){part.fillRange(0,part.length,0);}_parts.clear();_bytes=0;}
 Future<void> cancel()async{_cancelled=true;_operation++;_timer?.cancel();_timer=null;_clear();await _capture?.cancel();_capture=null;await _locked?.cancel();_locked=null;await _audioRecorder.stop();}
 Future<void> dispose()async{if(_disposed)return;_disposed=true;await cancel();await _audioRecorder.dispose();}
}
