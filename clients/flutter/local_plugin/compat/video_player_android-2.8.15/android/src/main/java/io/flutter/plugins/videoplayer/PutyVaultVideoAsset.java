package io.flutter.plugins.videoplayer;
import android.content.Context;
import androidx.media3.common.MediaItem;
import androidx.media3.exoplayer.source.MediaSource;
import androidx.media3.exoplayer.source.ProgressiveMediaSource;
final class PutyVaultVideoAsset extends VideoAsset {
 PutyVaultVideoAsset(String uri){super(uri);}
 @Override public MediaItem getMediaItem(){return MediaItem.fromUri(assetUrl);}
 @Override public MediaSource.Factory getMediaSourceFactory(Context context){return new ProgressiveMediaSource.Factory(PutyVaultDataSource::new);}
}
