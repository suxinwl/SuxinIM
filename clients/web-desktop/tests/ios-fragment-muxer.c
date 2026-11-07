#include "PMFragmentMuxer.h"
void *memset(void *memory,int value,size_t count){uint8_t *out=(uint8_t*)memory;for(size_t i=0;i<count;i++)out[i]=(uint8_t)value;return memory;}
static uint8_t result[8u<<20],input[1u<<20];static size_t length;static int reject;static PMFragmentMuxer mux;
static int collect(void *context,const uint8_t *bytes,size_t count){(void)context;if(reject||count>sizeof(result)-length)return -1;for(size_t i=0;i<count;i++)result[length+i]=bytes[i];length+=count;return 0;}
static uint8_t sps[]={0x67,0x42,0,0x1f,0xe5,0x88},pps[]={0x68,0xce,0x3c,0x80},asc[]={0x12,0x08},video[]={0,0,0,3,0x65,0x88,0x80},audio[]={0x21,0x10,0x04};
int build_fixture(void){length=0;reject=0;PMMuxInitialize(&mux,collect,0);if(PMMuxConfigureVideo(&mux,640,480,sps,sizeof(sps),pps,sizeof(pps))||PMMuxConfigureAudio(&mux,44100,1,asc,2)||PMMuxStart(&mux)||PMMuxVideo(&mux,video,sizeof(video),0,3000,1)||PMMuxAudio(&mux,audio,sizeof(audio),0,1024)||PMMuxVideo(&mux,video,sizeof(video),3000,3000,0)||PMMuxFinish(&mux))return -1;return 0;}
uintptr_t bytes(void){return (uintptr_t)result;}size_t size(void){return length;}
int reject_invalid(void){uint8_t broken[]={0,0,0,9,0x65};size_t before=length;if(PMMuxVideo(&mux,broken,sizeof(broken),6000,3000,0)!=-1||length!=before)return -1;if(PMMuxVideo(&mux,video,sizeof(video),1,3000,0)!=-1||length!=before)return -2;if(PMMuxConfigureAudio(&mux,44100,1,asc,2)!=-1)return -3;return 0;}
int reject_output(void){reject=1;if(PMMuxVideo(&mux,video,sizeof(video),6000,3000,0)!=-1||!mux.failed)return -1;reject=0;return PMMuxFinish(&mux)==-1?0:-2;}
int reject_bounds(void){if(build_fixture())return -1;size_t before=length;if(PMMuxVideo(&mux,video,sizeof(video),3000,3000,0)!=-1||length!=before)return -2;if(PMMuxVideo(&mux,video,sizeof(video),UINT64_MAX,3000,0)!=-1||length!=before)return -3;PMFragmentMuxer next;PMMuxInitialize(&next,collect,0);if(PMMuxStart(&next)!=-1||PMMuxFinish(&next)!=-1)return -4;uint8_t wrong[]={0x12,0x10};if(PMMuxConfigureAudio(&next,44100,1,wrong,2)!=-1||PMMuxConfigureAudio(&next,48000,1,asc,2)!=-1)return -5;return 0;}
int video_only(void){length=0;reject=0;PMMuxInitialize(&mux,collect,0);return PMMuxConfigureVideo(&mux,1280,720,sps,sizeof(sps),pps,sizeof(pps))||PMMuxStart(&mux)||PMMuxVideo(&mux,video,sizeof(video),0,3000,1)||PMMuxFinish(&mux);}
/* Public-fixture adapter for real WebCodecs elementary samples. The caller
 * copies SPS+PPS / ASC / each AVCC or raw AAC packet into input(), then invokes
 * these functions. All input/output is synthetic and bounded; C is unchanged. */
uintptr_t input_bytes(void){return (uintptr_t)input;}size_t input_capacity(void){return sizeof(input);}
void reset_mux(void){length=0;reject=0;PMMuxInitialize(&mux,collect,0);}
int configure_video(uint32_t width,uint32_t height,size_t sps_count,size_t pps_count){if(sps_count>sizeof(input)||pps_count>sizeof(input)-sps_count)return -1;return PMMuxConfigureVideo(&mux,width,height,input,sps_count,input+sps_count,pps_count);}
int configure_audio(uint32_t rate,uint32_t channels,size_t count){if(count>sizeof(input))return -1;return PMMuxConfigureAudio(&mux,rate,channels,input,count);}
int start_mux(void){return PMMuxStart(&mux);}
int video_sample(size_t count,uint64_t time,uint32_t duration,int key){if(count>sizeof(input))return -1;return PMMuxVideo(&mux,input,count,time,duration,key);}
int audio_sample(size_t count,uint64_t time,uint32_t duration){if(count>sizeof(input))return -1;return PMMuxAudio(&mux,input,count,time,duration);}
int finish_mux(void){return PMMuxFinish(&mux);}
