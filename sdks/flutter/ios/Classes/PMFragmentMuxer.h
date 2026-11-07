#ifndef PUTY_FRAGMENT_MUXER_H
#define PUTY_FRAGMENT_MUXER_H
#include <stdint.h>
#include <stddef.h>

/* A bounded ISO BMFF writer for VideoToolbox AVC without B frames and AAC-LC.
 * No filesystem API, allocator, logger or plaintext fallback exists here.
 * The adopter's callback writes synchronously to authorized encrypted scratch.
 * Return 0 to accept bytes, nonzero to permanently fail the writer. */
typedef int (*PMMuxEmit)(void *context,const uint8_t *bytes,size_t count);
typedef struct PMFragmentMuxer {
  PMMuxEmit emit;void *context;
  uint32_t width,height,sequence,audio_rate,audio_channels;
  uint8_t avcc[512],asc[64];size_t avcc_size,asc_size;
  uint64_t last_video,last_audio;int have_video,have_audio,started,failed;
} PMFragmentMuxer;
void PMMuxInitialize(PMFragmentMuxer *mux,PMMuxEmit emit,void *context);
int PMMuxConfigureVideo(PMFragmentMuxer *mux,uint32_t width,uint32_t height,const uint8_t *sps,size_t sps_count,const uint8_t *pps,size_t pps_count);
int PMMuxConfigureAudio(PMFragmentMuxer *mux,uint32_t rate,uint32_t channels,const uint8_t *asc,size_t count);
int PMMuxStart(PMFragmentMuxer *mux);
int PMMuxVideo(PMFragmentMuxer *mux,const uint8_t *sample,size_t count,uint64_t decode_time,uint32_t duration,int key_frame);
int PMMuxAudio(PMFragmentMuxer *mux,const uint8_t *sample,size_t count,uint64_t decode_time,uint32_t duration);
int PMMuxFinish(PMFragmentMuxer *mux);
#endif
