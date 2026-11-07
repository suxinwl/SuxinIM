package com.example.video_compress

import android.content.Context
import android.net.Uri
import android.util.Log
import com.otaliastudios.transcoder.Transcoder
import com.otaliastudios.transcoder.TranscoderListener
import com.otaliastudios.transcoder.source.TrimDataSource
import com.otaliastudios.transcoder.source.UriDataSource
import com.otaliastudios.transcoder.strategy.DefaultAudioStrategy
import com.otaliastudios.transcoder.strategy.DefaultVideoStrategy
import com.otaliastudios.transcoder.strategy.RemoveTrackStrategy
import com.otaliastudios.transcoder.strategy.TrackStrategy
import io.flutter.embedding.engine.plugins.FlutterPlugin
import io.flutter.plugin.common.BinaryMessenger
import com.otaliastudios.transcoder.internal.utils.Logger
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.MethodChannel.MethodCallHandler
import java.io.File
import java.text.SimpleDateFormat
import java.util.*
import java.util.concurrent.Future

/**
 * VideoCompressPlugin
 */
class VideoCompressPlugin : MethodCallHandler, FlutterPlugin {


    private var _context: Context? = null
    private var _channel: MethodChannel? = null
    private val TAG = "VideoCompressPlugin"
    private val LOG = Logger(TAG)
    private var transcodeFuture:Future<Void>? = null
    var channelName = "video_compress"

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        val context = _context;
        val channel = _channel;

        if (context == null || channel == null) {
            Log.w(TAG, "Calling VideoCompress plugin before initialization")
            return
        }

        try { when (call.method) {
            "getByteThumbnail" -> {
                val path = call.argument<String>("path")
                val quality = call.argument<Int>("quality")!!
                val position = call.argument<Int>("position")!! // to long
                ThumbnailUtility(channelName).getByteThumbnail(path!!, quality, position.toLong(), result)
            }
            "getFileThumbnail" -> {
                val path = call.argument<String>("path")
                val quality = call.argument<Int>("quality")!!
                val position = call.argument<Int>("position")!! // to long
                ThumbnailUtility("video_compress").getFileThumbnail(context, path!!, quality,
                        position.toLong(), result)
            }
            "getMediaInfo" -> {
                val path = call.argument<String>("path")
                result.success(Utility(channelName).getMediaInfoJson(context, path!!).toString())
            }
            "deleteAllCache" -> {
                result.success(Utility(channelName).deleteAllCache(context, result));
            }
            "setLogLevel" -> {
                val logLevel = call.argument<Int>("logLevel")!!
                Logger.setLogLevel(logLevel)
                result.success(true);
            }
            "cancelCompression" -> {
                transcodeFuture?.cancel(true)
                result.success(false);
            }
            "compressVideo" -> {
                if(transcodeFuture?.isDone==false)throw IllegalStateException("Encrypted conversion already active")
                val path = call.argument<String>("path")!!
                val quality = call.argument<Int>("quality")!!
                val deleteOrigin = call.argument<Boolean>("deleteOrigin")!!
                val startTime = call.argument<Int>("startTime")
                val duration = call.argument<Int>("duration")
                val includeAudio = call.argument<Boolean>("includeAudio") ?: true
                val frameRate = if (call.argument<Int>("frameRate")==null) 30 else call.argument<Int>("frameRate")

                if(android.os.Build.VERSION.SDK_INT<26){
                    val json=Utility(channelName).getMediaInfoJson(context,path);json.put("isCancel",false);result.success(json.toString());return
                }
                val out = ProtectedMediaSource.name(path).substringBeforeLast('.')
                val encryptedFile=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles")
                    .getMethod("createTempFile",String::class.java,String::class.java)
                    .invoke(null,out,".mp4") as File
                val destPath:String=encryptedFile.absolutePath

                var videoTrackStrategy: TrackStrategy = DefaultVideoStrategy.atMost(340).build();
                val audioTrackStrategy: TrackStrategy

                when (quality) {

                    0 -> {
                      videoTrackStrategy = DefaultVideoStrategy.atMost(720).build()
                    }

                    1 -> {
                        videoTrackStrategy = DefaultVideoStrategy.atMost(360).build()
                    }
                    2 -> {
                        videoTrackStrategy = DefaultVideoStrategy.atMost(640).build()
                    }
                    3 -> {

                        assert(value = frameRate != null)
                        videoTrackStrategy = DefaultVideoStrategy.Builder()
                                .keyFrameInterval(3f)
                                .bitRate(1280 * 720 * 4.toLong())
                                .frameRate(frameRate!!) // will be capped to the input frameRate
                                .build()
                    }
                    4 -> {
                        videoTrackStrategy = DefaultVideoStrategy.atMost(480, 640).build()
                    }
                    5 -> {
                        videoTrackStrategy = DefaultVideoStrategy.atMost(540, 960).build()
                    }
                    6 -> {
                        videoTrackStrategy = DefaultVideoStrategy.atMost(720, 1280).build()
                    }
                    7 -> {
                        videoTrackStrategy = DefaultVideoStrategy.atMost(1080, 1920).build()
                    }                    
                }

                audioTrackStrategy = if (includeAudio) {
                    val sampleRate = DefaultAudioStrategy.SAMPLE_RATE_AS_INPUT
                    val channels = DefaultAudioStrategy.CHANNELS_AS_INPUT

                    DefaultAudioStrategy.builder()
                        .channels(channels)
                        .sampleRate(sampleRate)
                        .build()
                } else {
                    RemoveTrackStrategy()
                }

                val source=ProtectedTranscodeSource(path)
                val dataSource = if(startTime!=null||duration!=null)TrimDataSource(source,1000000L*(startTime?:0),1000000L*(duration?:0)) else source
                val bridge=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles")
                val descriptor=bridge.getMethod("descriptor",String::class.java).invoke(null,destPath) as android.os.ParcelFileDescriptor
                val finished=java.util.concurrent.atomic.AtomicBoolean(false)
                var registration:String?=null
                val requestFuture=java.util.concurrent.atomic.AtomicReference<Future<Void>?>()
                val canceled=java.util.concurrent.atomic.AtomicBoolean(false)
                fun finish(){try{descriptor.close()}finally{registration?.let{bridge.getMethod("unregisterConsumer",String::class.java).invoke(null,it)};registration=null}}
                fun discard(){try{bridge.getMethod("discard",String::class.java).invoke(null,destPath)}catch(ignored:Exception){}}
                try{
                registration=bridge.getMethod("registerConsumer",Runnable::class.java).invoke(null,Runnable{canceled.set(true);requestFuture.get()?.cancel(true);try{descriptor.close()}catch(ignored:Exception){}}) as String
                if(canceled.get())throw IllegalStateException("Encrypted conversion canceled")
                val future = Transcoder.into(descriptor.fileDescriptor)
                        .addDataSource(dataSource)
                        .setAudioTrackStrategy(audioTrackStrategy)
                        .setVideoTrackStrategy(videoTrackStrategy)
                        .setListener(object : TranscoderListener {
                            override fun onTranscodeProgress(progress: Double) {
                                channel.invokeMethod("updateProgress", progress * 100.00)
                            }
                            override fun onTranscodeCompleted(successCode: Int) {
                                if(!finished.compareAndSet(false,true))return
                                try{val json = Utility(channelName).getMediaInfoJson(context, destPath);json.put("isCancel", false);result.success(json.toString())}catch(rejected:Exception){result.error("ENCRYPTED_MEDIA_UNAVAILABLE","Encrypted video unavailable",null)}finally{finish()}
                                // Originals remain encrypted until the SDK binds/ACKs or explicitly discards them.
                            }

                            override fun onTranscodeCanceled() {
                                if(finished.compareAndSet(false,true)){try{finish()}finally{discard()};result.success(null)}
                            }

                            override fun onTranscodeFailed(exception: Throwable) {
                                if(finished.compareAndSet(false,true)){try{finish()}finally{discard()};result.error("ENCRYPTED_MEDIA_UNAVAILABLE","Encrypted video conversion failed",null)}
                            }
                        }).transcode()
                requestFuture.set(future);transcodeFuture=future
                if(canceled.get())future.cancel(true)
                }catch(rejected:Exception){if(finished.compareAndSet(false,true)){try{finish()}finally{discard()};throw rejected}}
            }
            else -> {
                result.notImplemented()
            }
        }}catch(rejected:Exception){result.error("ENCRYPTED_MEDIA_UNAVAILABLE","Encrypted media operation unavailable",null)}
    }

    override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        init(binding.applicationContext, binding.binaryMessenger)
    }

    override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        transcodeFuture?.cancel(true)
        _channel?.setMethodCallHandler(null)
        _context = null
        _channel = null
    }

    private fun init(context: Context, messenger: BinaryMessenger) {
        val channel = MethodChannel(messenger, channelName)
        channel.setMethodCallHandler(this)
        _context = context
        _channel = channel
    }

    companion object {
        private const val TAG = "video_compress"
    }

}
