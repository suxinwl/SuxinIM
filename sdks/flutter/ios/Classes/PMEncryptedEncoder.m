#import "PMEncryptedEncoder.h"
#import "PMFragmentMuxer.h"
#import <VideoToolbox/VideoToolbox.h>
#import <AudioToolbox/AudioToolbox.h>
#include <stdlib.h>
#include <string.h>

static const OSStatus PMAudioNeedsInput = 0x70656d70;
typedef struct { AudioBufferList *buffers; UInt32 offset,frames,bytesPerFrame; BOOL end; } PMAudioInput;
static OSStatus PMAudioRead(AudioConverterRef converter,UInt32 *packets,AudioBufferList *output,
                           AudioStreamPacketDescription **descriptions,void *context) {
    PMAudioInput *input=context;
    if(!input||input->offset>=input->frames){*packets=0;return input&&input->end?noErr:PMAudioNeedsInput;}
    UInt32 count=MIN(*packets,input->frames-input->offset);output->mNumberBuffers=input->buffers->mNumberBuffers;
    for(UInt32 i=0;i<output->mNumberBuffers;i++) {
        AudioBuffer source=input->buffers->mBuffers[i];
        output->mBuffers[i]=(AudioBuffer){source.mNumberChannels,count*input->bytesPerFrame,
            (uint8_t*)source.mData+input->offset*input->bytesPerFrame};
    }
    input->offset+=count;*packets=count;if(descriptions)*descriptions=NULL;return noErr;
}

@interface PMEncryptedEncoder () {
    PMFragmentMuxer _mux; VTCompressionSessionRef _video; AudioConverterRef _audio;
    AudioStreamBasicDescription _inputFormat; NSRecursiveLock *_lock;
    BOOL (^_emit)(NSData*); BOOL _closed,_failed,_wantAudio,_started,_haveInput;
    CMTime _base,_lastInput; uint64_t _audioTime; BOOL _haveAudioTime;
    NSMutableArray<NSArray*> *_earlyAudio; NSUInteger _earlyBytes;
}
- (void)compressed:(CMSampleBufferRef)sample status:(OSStatus)status;
- (BOOL)performEmit:(const uint8_t*)bytes count:(size_t)count;
@end
static int PMEncoderEmit(void *context,const uint8_t *bytes,size_t count) {
    PMEncryptedEncoder *encoder=(__bridge PMEncryptedEncoder*)context;
    return [encoder performEmit:bytes count:count]?0:-1;
}
static void PMVideoOutput(void *context,void *source,OSStatus status,VTEncodeInfoFlags flags,CMSampleBufferRef sample) {
    if(status==noErr&&(flags&kVTEncodeInfo_FrameDropped))return;
    [(__bridge PMEncryptedEncoder*)context compressed:sample status:status];
}
@implementation PMEncryptedEncoder
- (BOOL)performEmit:(const uint8_t*)bytes count:(size_t)count {
    if(_closed||_failed||!_emit||count>(16u<<20))return NO;
    // Copy lifetime is limited to the synchronous encrypted sink. NSData never
    // becomes an AVAssetWriter/file output or an application cache.
    NSMutableData *data=[NSMutableData dataWithBytes:bytes length:count];BOOL ok=_emit(data);
    memset(data.mutableBytes,0,data.length);if(!ok)_failed=YES;return ok;
}
- (instancetype)initWithWidth:(int)width height:(int)height audio:(BOOL)audio bitrate:(int)bitrate emit:(BOOL (^)(NSData *))emit {
    if(!(self=[super init]))return nil;if(width<2||height<2||width>4096||height>4096||!emit)return nil;
    _lock=[NSRecursiveLock new];_emit=[emit copy];_wantAudio=audio;_base=kCMTimeInvalid;_lastInput=kCMTimeInvalid;
    _earlyAudio=[NSMutableArray new];PMMuxInitialize(&_mux,PMEncoderEmit,(__bridge void*)self);
    if(audio){const uint8_t asc[]={0x12,0x08};if(PMMuxConfigureAudio(&_mux,44100,1,asc,2))return nil;}
    OSStatus status=VTCompressionSessionCreate(kCFAllocatorDefault,width,height,kCMVideoCodecType_H264,NULL,NULL,NULL,PMVideoOutput,(__bridge void*)self,&_video);
    if(status||!_video)return nil;
    NSDictionary *properties=@{(__bridge NSString*)kVTCompressionPropertyKey_RealTime:@YES,
      (__bridge NSString*)kVTCompressionPropertyKey_AllowFrameReordering:@NO,
      (__bridge NSString*)kVTCompressionPropertyKey_ProfileLevel:(__bridge NSString*)kVTProfileLevel_H264_Baseline_AutoLevel,
      (__bridge NSString*)kVTCompressionPropertyKey_AverageBitRate:@(MAX(256000,MIN(20000000,bitrate))),
      (__bridge NSString*)kVTCompressionPropertyKey_MaxKeyFrameInterval:@60,
      (__bridge NSString*)kVTCompressionPropertyKey_ExpectedFrameRate:@30};
    for(NSString *key in properties){status=VTSessionSetProperty(_video,(__bridge CFStringRef)key,(__bridge CFTypeRef)properties[key]);if(status){[self cancel];return nil;}}
    if(VTCompressionSessionPrepareToEncodeFrames(_video)){[self cancel];return nil;}return self;
}
- (BOOL)appendVideo:(CVPixelBufferRef)image time:(CMTime)time duration:(CMTime)duration {
    [_lock lock];BOOL valid=!_closed&&!_failed&&CMTIME_IS_NUMERIC(time)&&image;
    if(valid&&CMTIME_IS_NUMERIC(_lastInput)&&CMTimeCompare(time,_lastInput)<=0){[_lock unlock];return YES;}
    if(valid){if(!CMTIME_IS_NUMERIC(_base))_base=time;_lastInput=time;_haveInput=YES;}
    VTCompressionSessionRef session=_video;if(session)CFRetain(session);[_lock unlock];
    if(!valid||!session)return NO;CMTime relative=CMTimeSubtract(time,_base);
    OSStatus status=VTCompressionSessionEncodeFrame(session,image,relative,CMTIME_IS_NUMERIC(duration)&&duration.value>0?duration:CMTimeMake(1,30),NULL,NULL,NULL);
    CFRelease(session);if(status){[_lock lock];_failed=YES;[_lock unlock];}return status==noErr;
}
- (void)compressed:(CMSampleBufferRef)sample status:(OSStatus)status {
    [_lock lock];@try {
        if(_closed||_failed)return;if(status||!sample||!CMSampleBufferDataIsReady(sample)){_failed=YES;return;}
        CMFormatDescriptionRef format=CMSampleBufferGetFormatDescription(sample);if(!format){_failed=YES;return;}
        if(!_started){const uint8_t *sps=NULL,*pps=NULL;size_t spsCount=0,ppsCount=0,n=0;int length=0;
            if(CMVideoFormatDescriptionGetH264ParameterSetAtIndex(format,0,&sps,&spsCount,&n,&length)||length!=4||
               CMVideoFormatDescriptionGetH264ParameterSetAtIndex(format,1,&pps,&ppsCount,&n,&length)){_failed=YES;return;}
            CMVideoDimensions size=CMVideoFormatDescriptionGetDimensions(format);
            if(PMMuxConfigureVideo(&_mux,size.width,size.height,sps,spsCount,pps,ppsCount)||PMMuxStart(&_mux)){_failed=YES;return;}_started=YES;
            for(NSArray *packet in _earlyAudio){NSData *data=packet[0];if(PMMuxAudio(&_mux,data.bytes,data.length,[packet[1] unsignedLongLongValue],1024)){_failed=YES;return;}}
            [_earlyAudio removeAllObjects];_earlyBytes=0;
        }
        CMBlockBufferRef block=CMSampleBufferGetDataBuffer(sample);size_t count=block?CMBlockBufferGetDataLength(block):0;
        if(!count||count>(16u<<20)){_failed=YES;return;}NSMutableData *body=[NSMutableData dataWithLength:count];
        if(CMBlockBufferCopyDataBytes(block,0,count,body.mutableBytes)){_failed=YES;return;}
        CFArrayRef attachments=CMSampleBufferGetSampleAttachmentsArray(sample,false);BOOL key=YES;
        if(attachments&&CFArrayGetCount(attachments)>0)key=!CFDictionaryContainsKey(CFArrayGetValueAtIndex(attachments,0),kCMSampleAttachmentKey_NotSync);
        CMTime time=CMTimeConvertScale(CMSampleBufferGetPresentationTimeStamp(sample),90000,kCMTimeRoundingMethod_Default);
        CMTime duration=CMTimeConvertScale(CMSampleBufferGetDuration(sample),90000,kCMTimeRoundingMethod_Default);
        if(time.value<0||PMMuxVideo(&_mux,body.bytes,body.length,(uint64_t)time.value,(uint32_t)(CMTIME_IS_NUMERIC(duration)&&duration.value>0?duration.value:3000),key))_failed=YES;
        memset(body.mutableBytes,0,body.length);
    } @finally {[_lock unlock];}
}
- (BOOL)convertAudio:(PMAudioInput*)input {
    // One AAC-LC packet is at most a few KiB for this mono bitrate. The fixed
    // upper bound also makes a malformed converter response fail closed.
    uint8_t output[8192];AudioStreamPacketDescription packet;
    for(int iteration=0;iteration<4096;iteration++){
        UInt32 packets=1;AudioBufferList list={1,{1,sizeof(output),output}};
        OSStatus status=AudioConverterFillComplexBuffer(_audio,PMAudioRead,input,&packets,&list,&packet);
        if(packets){if(packets!=1||list.mBuffers[0].mDataByteSize>sizeof(output)){_failed=YES;break;}
            NSData *data=[NSData dataWithBytes:output length:list.mBuffers[0].mDataByteSize];
            if(_started){if(PMMuxAudio(&_mux,data.bytes,data.length,_audioTime,1024))_failed=YES;}
            else {if(_earlyBytes+data.length>(1u<<20)){_failed=YES;break;}[_earlyAudio addObject:@[data,@(_audioTime)]];_earlyBytes+=data.length;}
            _audioTime+=1024;
        }
        memset(output,0,sizeof(output));if(_failed)return NO;
        if(status==PMAudioNeedsInput||!packets)return YES;if(status){_failed=YES;return NO;}
    }
    _failed=YES;return NO;
}
- (BOOL)appendAudio:(CMSampleBufferRef)sample {
    [_lock lock];@try {
        if(_closed||_failed)return NO;if(!_wantAudio)return YES;
        if(!CMTIME_IS_NUMERIC(_base))return YES; // capture begins at the first video frame
        const AudioStreamBasicDescription *format=CMAudioFormatDescriptionGetStreamBasicDescription(CMSampleBufferGetFormatDescription(sample));
        if(!format||format->mFormatID!=kAudioFormatLinearPCM||!format->mBytesPerFrame)return NO;
        if(!_audio){_inputFormat=*format;AudioStreamBasicDescription out={0};out.mSampleRate=44100;out.mFormatID=kAudioFormatMPEG4AAC;out.mChannelsPerFrame=1;out.mFramesPerPacket=1024;
            if(AudioConverterNew(&_inputFormat,&out,&_audio)||!_audio){_failed=YES;return NO;}UInt32 rate=96000;AudioConverterSetProperty(_audio,kAudioConverterEncodeBitRate,sizeof(rate),&rate);
        }else if(format->mSampleRate!=_inputFormat.mSampleRate||format->mFormatID!=_inputFormat.mFormatID||format->mFormatFlags!=_inputFormat.mFormatFlags||format->mBytesPerFrame!=_inputFormat.mBytesPerFrame||format->mChannelsPerFrame!=_inputFormat.mChannelsPerFrame||format->mBitsPerChannel!=_inputFormat.mBitsPerChannel){_failed=YES;return NO;}
        if(!_haveAudioTime){CMTime relative=CMTimeConvertScale(CMTimeSubtract(CMSampleBufferGetPresentationTimeStamp(sample),_base),44100,kCMTimeRoundingMethod_Default);_audioTime=(uint64_t)MAX(0,relative.value);_haveAudioTime=YES;}
        size_t needed=0;CMBlockBufferRef retained=NULL;
        if(CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(sample,&needed,NULL,0,kCFAllocatorDefault,kCFAllocatorDefault,kCMSampleBufferFlag_AudioBufferList_Assure16ByteAlignment,&retained)!=noErr&&needed==0)return NO;
        if(retained){CFRelease(retained);retained=NULL;}if(needed<sizeof(AudioBufferList)||needed>4096)return NO;
        AudioBufferList *list=calloc(1,needed);if(!list)return NO;
        OSStatus status=CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(sample,NULL,list,needed,kCFAllocatorDefault,kCFAllocatorDefault,kCMSampleBufferFlag_AudioBufferList_Assure16ByteAlignment,&retained);
        BOOL ok=NO;if(!status&&list->mNumberBuffers>0&&list->mNumberBuffers<=8){UInt32 frames=(UInt32)CMSampleBufferGetNumSamples(sample);BOOL valid=frames>0&&frames<=1048576;for(UInt32 i=0;i<list->mNumberBuffers;i++)if(!list->mBuffers[i].mData||(uint64_t)frames*format->mBytesPerFrame>list->mBuffers[i].mDataByteSize)valid=NO;if(valid){PMAudioInput input={list,0,frames,format->mBytesPerFrame,NO};ok=[self convertAudio:&input];}}
        if(retained)CFRelease(retained);free(list);return ok;
    } @finally {[_lock unlock];}
}
- (BOOL)finish {
    [_lock lock];VTCompressionSessionRef session=_video;if(session)CFRetain(session);BOOL valid=!_closed&&!_failed&&_haveInput;[_lock unlock];if(!valid||!session)return NO;
    OSStatus status=VTCompressionSessionCompleteFrames(session,kCMTimeInvalid);CFRelease(session);
    [_lock lock];@try {if(status)_failed=YES;if(_audio&&!_failed){PMAudioInput input={NULL,0,0,0,YES};[self convertAudio:&input];}BOOL ok=!_failed&&PMMuxFinish(&_mux)==0;_closed=YES;return ok;}@finally{[_lock unlock];[self releaseCodecs];}
}
- (void)releaseCodecs {[_lock lock];VTCompressionSessionRef video=_video;_video=NULL;AudioConverterRef audio=_audio;_audio=NULL;[_earlyAudio removeAllObjects];_emit=nil;[_lock unlock];if(video){VTCompressionSessionInvalidate(video);CFRelease(video);}if(audio)AudioConverterDispose(audio);}
- (void)cancel {[_lock lock];_closed=YES;_failed=YES;[_lock unlock];[self releaseCodecs];}
- (void)dealloc {[self cancel];}
@end
