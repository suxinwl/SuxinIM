package com.example.video_compress

import android.media.MediaDataSource
import android.media.MediaExtractor
import android.media.MediaMetadataRetriever
import android.os.ParcelFileDescriptor
import com.otaliastudios.transcoder.source.DefaultDataSource
import java.io.IOException
import java.nio.ByteBuffer
import java.util.concurrent.atomic.AtomicBoolean

/** API23+ native readers use authenticated random access. They never reopen an
 * opaque scratch path, and every decoder is canceled on vault lock. */
internal class ProtectedMediaSource(private val path:String):MediaDataSource(){
 private val scratch=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles")
 private val selected=Class.forName("io.openim.flutter_openim_sdk.EncryptedSelectedFiles")
 private val isScratch=scratch.getMethod("owns",String::class.java).invoke(null,path) as Boolean
 private val closed=AtomicBoolean(false)
 private val input=if(isScratch)ParcelFileDescriptor.AutoCloseInputStream(scratch.getMethod("descriptor",String::class.java).invoke(null,path) as ParcelFileDescriptor) else null
 private var registration:String?=null
 init{try{registration=scratch.getMethod("registerConsumer",Runnable::class.java).invoke(null,Runnable{close()}) as String}catch(rejected:Exception){close();throw IOException("Encrypted media reader rejected")}}
 override fun getSize():Long{if(closed.get())throw IOException("Encrypted media lease ended");return ((if(isScratch)scratch else selected).getMethod("size",String::class.java).invoke(null,path) as Long)}
 override fun readAt(position:Long,buffer:ByteArray,offset:Int,size:Int):Int{
  if(closed.get()||position<0||offset<0||size<0||offset>buffer.size-size)throw IOException("Encrypted media range rejected")
  if(size==0)return 0
  val length=getSize();if(position>=length)return -1
  val count=minOf(size.toLong(),length-position,1048576L).toInt()
  if(input!=null){val n=input.channel.read(ByteBuffer.wrap(buffer,offset,count),position);if(closed.get()){buffer.fill(0,offset,offset+count);throw IOException("Encrypted media lease ended")};return n}
  val bytes=selected.getMethod("readRange",String::class.java,java.lang.Long.TYPE,Integer.TYPE).invoke(null,path,position,count) as ByteArray
  try{if(closed.get()||bytes.size!=count)throw IOException("Encrypted media range rejected");bytes.copyInto(buffer,offset);return bytes.size}finally{bytes.fill(0)}
 }
 override fun close(){if(closed.compareAndSet(false,true)){try{input?.close()}finally{registration?.let{scratch.getMethod("unregisterConsumer",String::class.java).invoke(null,it)};registration=null}}}
 companion object{
  fun name(path:String):String{val bridge=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles");val own=bridge.getMethod("owns",String::class.java).invoke(null,path) as Boolean;return (if(own)bridge else Class.forName("io.openim.flutter_openim_sdk.EncryptedSelectedFiles")).getMethod("name",String::class.java).invoke(null,path) as String}
 }
}

internal class ProtectedTranscodeSource(private val path:String):DefaultDataSource(){
 private val readers=mutableListOf<ProtectedMediaSource>()
 override fun initializeExtractor(extractor:MediaExtractor){val source=ProtectedMediaSource(path);readers.add(source);extractor.setDataSource(source)}
 override fun initializeRetriever(retriever:MediaMetadataRetriever){val source=ProtectedMediaSource(path);readers.add(source);retriever.setDataSource(source)}
 override fun deinitialize(){try{super.deinitialize()}finally{readers.forEach{it.close()};readers.clear()}}
}
