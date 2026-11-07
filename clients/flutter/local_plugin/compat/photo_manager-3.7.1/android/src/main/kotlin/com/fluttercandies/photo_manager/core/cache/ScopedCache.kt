package com.fluttercandies.photo_manager.core.cache

import android.content.Context
import android.net.Uri
import android.os.Build
import androidx.annotation.RequiresApi
import com.fluttercandies.photo_manager.core.entity.AssetEntity
import com.fluttercandies.photo_manager.core.utils.AndroidQDBUtils
import com.fluttercandies.photo_manager.core.utils.AndroidQDBUtils.throwIdNotFound
import com.fluttercandies.photo_manager.util.LogUtils
import java.io.File
import java.io.FileOutputStream

@RequiresApi(Build.VERSION_CODES.Q)
class ScopedCache {
    companion object {
        private const val FILENAME_PREFIX = "pm_"
    }

    fun getCacheFileFromEntity(
        context: Context,
        assetEntity: AssetEntity,
        isOrigin: Boolean
    ): File {
        val assetId = assetEntity.id
        val uri=AndroidQDBUtils.getUri(assetId,assetEntity.type,isOrigin)
        if(uri==Uri.EMPTY)throwIdNotFound(assetId)
        val name=assetEntity.displayName
        val path=Class.forName("io.openim.flutter_openim_sdk.EncryptedSelectedFiles")
            .getMethod("importUri",Context::class.java,Uri::class.java,String::class.java)
            .invoke(null,context,uri,name) as String
        return File(path)
    }

    private fun getCacheFile(context: Context, assetEntity: AssetEntity, isOrigin: Boolean): File {
        val originString = if (isOrigin) "_o" else ""
        val name = "$FILENAME_PREFIX${assetEntity.id}${originString}_${assetEntity.displayName}"
        val dot=name.lastIndexOf('.')
        return Class.forName("io.openim.flutter_openim_sdk.EncryptedScratchFiles")
            .getMethod("createTempFile",String::class.java,String::class.java)
            .invoke(null,if(dot>0)name.substring(0,dot) else name,if(dot>0)name.substring(dot) else ".bin") as File
    }

    fun clearFileCache(context: Context) {
        val files = context.cacheDir?.listFiles()?.filterNotNull() ?: return
        for (file in files) {
            if (file.name.startsWith(FILENAME_PREFIX)) {
                file.delete()
            }
        }
    }
}
