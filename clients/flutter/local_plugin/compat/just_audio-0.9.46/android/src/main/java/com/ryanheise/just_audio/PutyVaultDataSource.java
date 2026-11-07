package com.ryanheise.just_audio;
import android.net.Uri;
import androidx.media3.common.C;
import androidx.media3.datasource.DataSource;
import androidx.media3.datasource.DataSpec;
import androidx.media3.datasource.TransferListener;
import java.io.IOException;
import java.util.Arrays;
final class PutyVaultDataSource implements DataSource {
 private Uri uri;private long position,remaining;private volatile boolean closed;private String registration;
 private final Class<?> bridge,consumers;
 PutyVaultDataSource(){try{bridge=Class.forName("io.openim.flutter_openim_sdk.EncryptedSelectedFiles");consumers=Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles");}catch(Exception e){throw new IllegalStateException("Encrypted media unavailable");}}
 @Override public void addTransferListener(TransferListener ignored){}
 @Override public long open(DataSpec spec)throws IOException{try{uri=spec.uri;position=spec.position;long size=(Long)bridge.getMethod("resourceSize",String.class).invoke(null,uri.toString());if(position<0||position>size)throw new IOException("Invalid local media offset");remaining=spec.length==C.LENGTH_UNSET?size-position:Math.min(spec.length,size-position);closed=false;registration=(String)consumers.getMethod("registerConsumer",Runnable.class).invoke(null,(Runnable)this::close);return remaining;}catch(Exception denied){close();throw new IOException("Encrypted media unavailable");}}
 @Override public int read(byte[] output,int offset,int length)throws IOException{if(closed||uri==null)throw new IOException("Encrypted media lease ended");if(length==0)return 0;if(remaining==0)return C.RESULT_END_OF_INPUT;int count=(int)Math.min(Math.min(length,remaining),1048576L);byte[] data=null;try{data=(byte[])bridge.getMethod("readResourceRange",String.class,long.class,int.class).invoke(null,uri.toString(),position,count);if(closed||data==null||data.length!=count)throw new IOException("Encrypted media read rejected");System.arraycopy(data,0,output,offset,count);position+=count;remaining-=count;return count;}catch(Exception denied){Arrays.fill(output,offset,offset+count,(byte)0);throw new IOException("Encrypted media read rejected");}finally{if(data!=null)Arrays.fill(data,(byte)0);}}
 @Override public Uri getUri(){return uri;}
 @Override public void close(){closed=true;String id=registration;registration=null;if(id!=null)try{consumers.getMethod("unregisterConsumer",String.class).invoke(null,id);}catch(Exception ignored){}}
}
