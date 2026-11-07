import android.media.MediaCodec;
import android.media.MediaCodecInfo;
import android.media.MediaFormat;
import android.util.Base64;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import org.json.JSONArray;
import org.json.JSONObject;

/** Synthetic 440Hz PCM only. Never opens a microphone, account or app directory. */
public final class PutyAACFixture {
  public static void main(String[] args) throws Exception {
    final int rate=44100,total=18*1024;
    MediaFormat format=MediaFormat.createAudioFormat("audio/mp4a-latm",rate,1);
    format.setInteger(MediaFormat.KEY_AAC_PROFILE,MediaCodecInfo.CodecProfileLevel.AACObjectLC);
    format.setInteger(MediaFormat.KEY_BIT_RATE,64000);
    format.setInteger(MediaFormat.KEY_MAX_INPUT_SIZE,4096);
    MediaCodec codec=MediaCodec.createEncoderByType("audio/mp4a-latm");
    JSONArray packets=new JSONArray();String config=null;int consumed=0;boolean sentEnd=false,done=false;
    long deadline=System.nanoTime()+15_000_000_000L;
    try {
      codec.configure(format,null,null,MediaCodec.CONFIGURE_FLAG_ENCODE);codec.start();
      MediaCodec.BufferInfo info=new MediaCodec.BufferInfo();
      while(!done&&System.nanoTime()<deadline){
        if(!sentEnd){int index=codec.dequeueInputBuffer(10000);if(index>=0){ByteBuffer input=codec.getInputBuffer(index);input.clear();input.order(ByteOrder.LITTLE_ENDIAN);int count=Math.min(Math.min(1024,input.remaining()/2),total-consumed);
          for(int n=0;n<count;n++)input.putShort((short)(Math.sin(2*Math.PI*440*(consumed+n)/rate)*6000));
          codec.queueInputBuffer(index,0,count*2,consumed*1_000_000L/rate,count==0?MediaCodec.BUFFER_FLAG_END_OF_STREAM:0);consumed+=count;if(count==0)sentEnd=true;}}
        int index=codec.dequeueOutputBuffer(info,10000);
        if(index==MediaCodec.INFO_OUTPUT_FORMAT_CHANGED){ByteBuffer bytes=codec.getOutputFormat().getByteBuffer("csd-0");byte[] array=new byte[bytes.remaining()];bytes.get(array);config=Base64.encodeToString(array,Base64.NO_WRAP);}
        if(index>=0){ByteBuffer output=codec.getOutputBuffer(index);output.position(info.offset);output.limit(info.offset+info.size);byte[] array=new byte[info.size];output.get(array);
          if((info.flags&MediaCodec.BUFFER_FLAG_CODEC_CONFIG)!=0)config=Base64.encodeToString(array,Base64.NO_WRAP);
          else if(array.length>0)packets.put(Base64.encodeToString(array,Base64.NO_WRAP));
          done=(info.flags&MediaCodec.BUFFER_FLAG_END_OF_STREAM)!=0;codec.releaseOutputBuffer(index,false);}
      }
      if(!done||config==null||packets.length()<18)throw new IllegalStateException("synthetic_aac_capture_incomplete");
      JSONObject result=new JSONObject();result.put("syntheticOnly",true);result.put("source","Android API30 MediaCodec AAC-LC software encoder");result.put("rate",rate);result.put("channels",1);result.put("audioSpecificConfig",config);result.put("packets",packets);System.out.println("PUTY_AAC_FIXTURE="+result.toString());
    } finally {try{codec.stop();}finally{codec.release();}}
  }
}
