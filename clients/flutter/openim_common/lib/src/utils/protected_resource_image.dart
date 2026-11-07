import 'dart:ui' as ui;
import 'package:app_im/app_im.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart';
import '../native_local_vault.dart';

/// Memory-only image decoding. A viewer generation is part of the cache key;
/// protected bytes are never delegated to ExtendedImage's disk cache.
class ProtectedResourceImage extends ImageProvider<ProtectedResourceImage> {
  final String source;
  final int generation;
  final int vaultGeneration;
  ProtectedResourceImage(this.source):generation=AppIm.enterprise.resourceGeneration,vaultGeneration=NativeLocalVault.generation;
  @override Future<ProtectedResourceImage> obtainKey(ImageConfiguration configuration)=>SynchronousFuture(this);
  @override ImageStreamCompleter loadImage(ProtectedResourceImage key,ImageDecoderCallback decode)=>MultiFrameImageStreamCompleter(codec:_load(decode),scale:1);
  Future<ui.Codec> _load(ImageDecoderCallback decode) async {
    if(generation!=AppIm.enterprise.resourceGeneration||vaultGeneration!=NativeLocalVault.generation)throw StateError('Image viewer changed');
    final bytes=source.startsWith('/')?await NativeLocalVault.readSource(source):await AppIm.enterprise.readResourceBytes(source);
    try{
      void current(){if(generation!=AppIm.enterprise.resourceGeneration||vaultGeneration!=NativeLocalVault.generation)throw StateError('Image viewer changed');}
      current();final buffer=await ui.ImmutableBuffer.fromUint8List(bytes);
      try{current();}catch(_){buffer.dispose();rethrow;}
      final codec=await decode(buffer);
      try{current();}catch(_){codec.dispose();rethrow;}
      return codec;
    }finally{bytes.fillRange(0,bytes.length,0);}
  }
  @override bool operator==(Object other)=>other is ProtectedResourceImage&&source==other.source&&generation==other.generation&&vaultGeneration==other.vaultGeneration;
  @override int get hashCode=>Object.hash(source,generation,vaultGeneration);
  @override String toString()=>'PUTY protected memory image';
}
