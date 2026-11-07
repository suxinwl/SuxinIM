import 'dart:io';
import 'dart:typed_data';
import 'package:photo_manager/photo_manager.dart';
import 'native_local_vault.dart';

/// A picker/camera result already imported by the native vault. Asset widgets
/// can reuse their existing interface without asking PhotoManager to export a
/// second, ordinary temporary file or save a captured image into the gallery.
class NativeMediaAsset extends AssetEntity {
 final String protectedPath;
 final String? snapshotPath;
 NativeMediaAsset(Map<String,dynamic> item):protectedPath=item['path'] as String,snapshotPath=item['snapshotPath'] as String?,super(id:item['path'] as String,typeInt:item['kind']=='video'?2:1,width:(item['width'] as num?)?.toInt()??0,height:(item['height'] as num?)?.toInt()??0,duration:(item['duration'] as num?)?.toInt()??0,title:item['name'] as String?,mimeType:item['mimeType'] as String?);
 @override Future<File?> get file async=>File(protectedPath);
 @override Future<File?> get originFile async=>File(protectedPath);
 @override Future<File?> get fileWithSubtype async=>File(protectedPath);
 @override Future<File?> get originFileWithSubtype async=>File(protectedPath);
 @override Future<String> get titleAsync async=>title??(await NativeLocalVault.sourceInfo(protectedPath))['name'] as String;
 @override Future<String?> get mimeTypeAsync async=>mimeType;
 @override Future<Uint8List?> get originBytes=>NativeLocalVault.readSource(protectedPath);
 @override Future<Uint8List?> thumbnailDataWithSize(ThumbnailSize size,{ThumbnailFormat format=ThumbnailFormat.jpeg,int quality=100,PMProgressHandler? progressHandler,int frame=0})async{
   if(type==AssetType.video){final path=snapshotPath??await NativeLocalVault.nativeThumbnail(protectedPath,quality:quality);return NativeLocalVault.readSource(path);}
   return NativeLocalVault.readSource(protectedPath);
 }
}
