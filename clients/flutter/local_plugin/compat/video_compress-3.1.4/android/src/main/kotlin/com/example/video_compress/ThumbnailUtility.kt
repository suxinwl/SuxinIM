package com.example.video_compress

import android.content.Context
import android.graphics.Bitmap
import io.flutter.plugin.common.MethodChannel
import java.io.ByteArrayOutputStream

private class ZeroingBytes:ByteArrayOutputStream(){override fun close(){buf.fill(0);reset();super.close()}}
class ThumbnailUtility(channelName:String){
 private val utility=Utility(channelName)
 fun getByteThumbnail(path:String,quality:Int,position:Long,result:MethodChannel.Result){
  val bmp=utility.getBitmap(path,position,result)
  try{ZeroingBytes().use{stream->
   if(!bmp.compress(Bitmap.CompressFormat.JPEG,quality,stream))throw IllegalStateException("Encrypted thumbnail unavailable")
   val bytes=stream.toByteArray();try{result.success(bytes)}finally{bytes.fill(0)}
  }}finally{bmp.recycle()}
 }
 fun getFileThumbnail(context:Context,path:String,quality:Int,position:Long,result:MethodChannel.Result){
  val bmp=utility.getBitmap(path,position,result)
  try{ZeroingBytes().use{stream->
   if(!bmp.compress(Bitmap.CompressFormat.JPEG,quality,stream))throw IllegalStateException("Encrypted thumbnail unavailable")
   val bytes=stream.toByteArray()
   try{val output=Class.forName("io.openim.flutter_openim_sdk.EncryptedSelectedFiles")
     .getMethod("importStream",java.io.InputStream::class.java,String::class.java)
     .invoke(null,java.io.ByteArrayInputStream(bytes),"video-thumbnail.jpg") as String
    result.success(output)
   }finally{bytes.fill(0)}
  }}finally{bmp.recycle()}
 }
}
