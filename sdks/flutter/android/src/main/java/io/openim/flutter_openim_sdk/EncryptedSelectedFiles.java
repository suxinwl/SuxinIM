package io.openim.flutter_openim_sdk;

import android.content.Context;
import android.net.Uri;
import android.util.Base64;
import java.io.*;
import java.util.Arrays;
import org.json.JSONObject;
import open_im_sdk.Open_im_sdk;

/** Selected documents stream directly to SQLCipher, including Android 7.x.
 * ProxyFileDescriptor is deliberately needed only by native capture producers. */
public final class EncryptedSelectedFiles {
 private static volatile EncryptedScratchFiles.Authority authority;
 private static volatile java.util.function.BooleanSupplier preview;
 private EncryptedSelectedFiles(){}
 public static void initialize(EncryptedScratchFiles.Authority value,java.util.function.BooleanSupplier canPreview){authority=value;preview=canPreview;}
 private static final class Binding {
  final EncryptedScratchFiles.Authority owner;final String account;final long generation;
  Binding()throws IOException{owner=authority;if(owner==null)throw new IOException("Encrypted document authority unavailable");generation=owner.generation();account=owner.authorizedAccount();check();}
  void check()throws IOException{if(owner!=authority||account==null||account.isEmpty()||generation!=owner.generation()||!account.equals(owner.authorizedAccount())||generation!=owner.generation())throw new IOException("Encrypted document lease ended");}
 }
 public static String importUri(Context context,Uri uri,String name)throws IOException{
  if(context==null||uri==null||(!"content".equals(uri.getScheme())&&!"file".equals(uri.getScheme())))throw new IOException("Invalid selected document");
  try(InputStream input=context.getContentResolver().openInputStream(uri)){if(input==null)throw new IOException("Selected document unavailable");return importStream(input,name);}
 }
 public static String importStream(InputStream input,String name)throws IOException{
  Binding binding=new Binding();String id=Open_im_sdk.beginNativeScratch(binding.account,name);if(id==null||id.isEmpty())throw new IOException("Encrypted document allocation rejected");
  String registration=null;byte[] buffer=new byte[65536];long offset=0;
  try{
   registration=EncryptedScratchFiles.registerConsumer(()->{try{input.close();}catch(IOException ignored){}});
   for(int n;(n=input.read(buffer))!=-1;){binding.check();offset+=n;if(offset>1073741824L)throw new IOException("Selected document exceeds encrypted limit");byte[] chunk=Arrays.copyOf(buffer,n);try{if(!Open_im_sdk.writeNativeScratch(binding.account,id,offset-n,chunk))throw new IOException("Encrypted document write rejected");binding.check();}finally{Arrays.fill(chunk,(byte)0);}}
   binding.check();String path=Open_im_sdk.stageNativeScratch(binding.account,id);binding.check();if(path==null||path.isEmpty())throw new IOException("Encrypted document import rejected");return path;
  }finally{Arrays.fill(buffer,(byte)0);Open_im_sdk.closeNativeScratch(binding.account,id);if(registration!=null)EncryptedScratchFiles.unregisterConsumer(registration);}
 }
 public static String name(String path)throws IOException{return info(path).optString("name");}
 public static long size(String path)throws IOException{long size=info(path).optLong("size",-1);if(size<0)throw new IOException("Encrypted document size unavailable");return size;}
 private static JSONObject info(String path)throws IOException{Binding binding=new Binding();try{String data=Open_im_sdk.nativeSourceInfo(binding.account,path);binding.check();return new JSONObject(data);}catch(Exception rejected){throw new IOException("Encrypted document metadata unavailable");}}
 public static byte[] readBytes(String path,long limit)throws IOException{if(limit<0||limit>32L*1024*1024)throw new IOException("Encrypted document memory limit");Binding binding=new Binding();byte[] data=Open_im_sdk.readNativeSource(binding.account,path,limit);try{binding.check();if(data==null)throw new IOException("Encrypted document read rejected");return data;}catch(IOException denied){if(data!=null)Arrays.fill(data,(byte)0);throw denied;}}
 public static byte[] readRange(String path,long offset,int count)throws IOException{Binding binding=new Binding();byte[] data=Open_im_sdk.readNativeSourceRange(binding.account,path,offset,count);try{binding.check();if(data==null)throw new IOException("Encrypted document range rejected");return data;}catch(IOException denied){if(data!=null)Arrays.fill(data,(byte)0);throw denied;}}
 public static void discard(String path)throws IOException{Binding binding=new Binding();if(!Open_im_sdk.discardNativeSource(binding.account,path))throw new IOException("Encrypted source remains referenced");binding.check();}
 public static String resourceUri(String path)throws IOException{info(path);return new Uri.Builder().scheme("puty-local").authority("source").appendPath(Base64.encodeToString(path.getBytes(java.nio.charset.StandardCharsets.UTF_8),Base64.URL_SAFE|Base64.NO_WRAP|Base64.NO_PADDING)).build().toString();}
 public static String attachmentUri(String task)throws IOException{resourceSize(new Uri.Builder().scheme("puty-local").authority("attachment").appendPath(task).build().toString());return new Uri.Builder().scheme("puty-local").authority("attachment").appendPath(task).build().toString();}
 private static String resourceIdentity(Uri uri)throws IOException{
  if(!"puty-local".equals(uri.getScheme())||uri.getUserInfo()!=null||uri.getPort()!=-1||uri.getQuery()!=null||uri.getFragment()!=null||uri.getPathSegments().size()!=1)throw new IOException("Invalid local media identity");
  String part=uri.getLastPathSegment();if("source".equals(uri.getAuthority())){try{byte[] value=Base64.decode(part,Base64.URL_SAFE|Base64.NO_WRAP|Base64.NO_PADDING);String path=new String(value,java.nio.charset.StandardCharsets.UTF_8);if(!Base64.encodeToString(value,Base64.URL_SAFE|Base64.NO_WRAP|Base64.NO_PADDING).equals(part))throw new IOException("Invalid local source identity");return path;}catch(IllegalArgumentException denied){throw new IOException("Invalid local source identity");}}
  if(!"attachment".equals(uri.getAuthority())||!part.matches("[0-9a-f]{64}")||preview==null||!preview.getAsBoolean())throw new IOException("Local attachment preview rejected");return part;
 }
 public static long resourceSize(String value)throws IOException{Uri uri=Uri.parse(value);String identity=resourceIdentity(uri);if("source".equals(uri.getAuthority()))return size(identity);Binding binding=new Binding();try{JSONObject item=new JSONObject(Open_im_sdk.nativeAttachmentInfo(binding.account,identity));binding.check();long size=item.getLong("size");if(size<0)throw new IOException("Invalid local attachment size");return size;}catch(Exception denied){throw new IOException("Local attachment unavailable");}}
 public static byte[] readResourceRange(String value,long offset,int count)throws IOException{Uri uri=Uri.parse(value);String identity=resourceIdentity(uri);if("source".equals(uri.getAuthority()))return readRange(identity,offset,count);Binding binding=new Binding();byte[] data=Open_im_sdk.readNativeAttachmentRange(binding.account,identity,offset,count);try{binding.check();resourceIdentity(uri);if(data==null)throw new IOException("Local attachment read rejected");return data;}catch(IOException denied){if(data!=null)Arrays.fill(data,(byte)0);throw denied;}}
}
