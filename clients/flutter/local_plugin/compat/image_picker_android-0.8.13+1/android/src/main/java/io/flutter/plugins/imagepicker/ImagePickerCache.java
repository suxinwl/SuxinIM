package io.flutter.plugins.imagepicker;
import android.content.Context;
import android.net.Uri;
import androidx.annotation.Nullable;
import java.util.*;

/** Pending picker state is process RAM only. An interrupted native import is
 * encrypted or abandoned; no URI, selected path or error payload is persisted. */
class ImagePickerCache {
 enum CacheType{IMAGE,VIDEO}
 static final String MAP_KEY_PATH_LIST="pathList",MAP_KEY_MAX_WIDTH="maxWidth",MAP_KEY_MAX_HEIGHT="maxHeight",MAP_KEY_IMAGE_QUALITY="imageQuality",MAP_KEY_TYPE="type",MAP_KEY_ERROR="error";
 static final String SHARED_PREFERENCES_NAME="flutter_image_picker_shared_preference";
 private static final Map<String,Object> pending=new HashMap<>();
 private static String cameraURI="";
 ImagePickerCache(Context context){context.getSharedPreferences(SHARED_PREFERENCES_NAME,Context.MODE_PRIVATE).edit().clear().commit();}
 void saveType(CacheType type){synchronized(pending){pending.put(MAP_KEY_TYPE,type==CacheType.VIDEO?Messages.CacheRetrievalType.VIDEO:Messages.CacheRetrievalType.IMAGE);}}
 void saveDimensionWithOutputOptions(Messages.ImageSelectionOptions options){synchronized(pending){if(options.getMaxWidth()!=null)pending.put(MAP_KEY_MAX_WIDTH,options.getMaxWidth());if(options.getMaxHeight()!=null)pending.put(MAP_KEY_MAX_HEIGHT,options.getMaxHeight());pending.put(MAP_KEY_IMAGE_QUALITY,options.getQuality().intValue());}}
 void savePendingCameraMediaUriPath(Uri uri){synchronized(pending){cameraURI=uri.getPath();}}
 String retrievePendingCameraMediaUriPath(){synchronized(pending){return cameraURI;}}
 void saveResult(@Nullable ArrayList<String> paths,@Nullable String code,@Nullable String message){synchronized(pending){if(paths!=null)pending.put(MAP_KEY_PATH_LIST,new ArrayList<>(paths));if(code!=null){Messages.CacheRetrievalError.Builder error=new Messages.CacheRetrievalError.Builder();error.setCode("ENCRYPTED_PICKER_REJECTED");error.setMessage("Encrypted picker operation rejected");pending.put(MAP_KEY_ERROR,error.build());}}}
 void clear(){synchronized(pending){pending.clear();cameraURI="";}}
 Map<String,Object> getCacheMap(){synchronized(pending){return pending.containsKey(MAP_KEY_PATH_LIST)||pending.containsKey(MAP_KEY_ERROR)?new HashMap<>(pending):new HashMap<>();}}
}
