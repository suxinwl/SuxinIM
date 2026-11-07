package io.openim.flutter_openim_sdk;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.Binder;
import android.os.ParcelFileDescriptor;
import android.os.Process;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;
import java.io.FileNotFoundException;
import java.io.IOException;

/** Same-UID, read-only virtual content. No grant to other applications and no
 * file/path fallback. Every FUSE callback rechecks the original online actor. */
public final class EncryptedScratchProvider extends ContentProvider {
  private static void caller(){if(Binder.getCallingUid()!=Process.myUid())throw new SecurityException("Encrypted preview is private");}
  @Override public boolean onCreate(){return true;}
  @Override public ParcelFileDescriptor openFile(Uri uri,String mode)throws FileNotFoundException {
    caller();if(!"r".equals(mode))throw new FileNotFoundException("Encrypted preview is read only");
    try{return EncryptedScratchFiles.openPreview(uri);}catch(IOException rejected){throw new FileNotFoundException("Encrypted preview unavailable");}
  }
  @Override public Cursor query(Uri uri,String[] projection,String selection,String[] selectionArgs,String order){
    caller();if(selection!=null||selectionArgs!=null||order!=null)throw new IllegalArgumentException("Encrypted preview query rejected");
    String[] columns=projection==null?new String[]{OpenableColumns.DISPLAY_NAME,OpenableColumns.SIZE}:projection;
    Object[] values=new Object[columns.length];try{for(int i=0;i<columns.length;i++){if(OpenableColumns.DISPLAY_NAME.equals(columns[i]))values[i]=EncryptedScratchFiles.previewName(uri);else if(OpenableColumns.SIZE.equals(columns[i]))values[i]=EncryptedScratchFiles.previewSize(uri);else throw new IllegalArgumentException("Encrypted preview column rejected");}}catch(IOException denied){return null;}
    MatrixCursor result=new MatrixCursor(columns,1);result.addRow(values);return result;
  }
  @Override public String getType(Uri uri){caller();try{String name=EncryptedScratchFiles.previewName(uri);int dot=name.lastIndexOf('.');String type=dot<0?null:MimeTypeMap.getSingleton().getMimeTypeFromExtension(name.substring(dot+1).toLowerCase(java.util.Locale.ROOT));return type==null?"application/octet-stream":type;}catch(IOException denied){return null;}}
  @Override public Uri insert(Uri uri,ContentValues values){throw new SecurityException("Encrypted preview is read only");}
  @Override public int delete(Uri uri,String selection,String[] args){throw new SecurityException("Encrypted preview is read only");}
  @Override public int update(Uri uri,ContentValues values,String selection,String[] args){throw new SecurityException("Encrypted preview is read only");}
}
