#include "PMFragmentMuxer.h"

typedef struct {uint8_t bytes[2048];size_t count;int bad;} Buffer;
static void copy(uint8_t *out,const uint8_t *in,size_t n){for(size_t i=0;i<n;i++)out[i]=in[i];}
static void zero(void *memory,size_t n){uint8_t *out=(uint8_t*)memory;for(size_t i=0;i<n;i++)out[i]=0;}
static void byte(Buffer *b,uint8_t v){if(b->count>=sizeof(b->bytes)){b->bad=1;return;}b->bytes[b->count++]=v;}
static void u16(Buffer *b,uint32_t v){byte(b,(uint8_t)(v>>8));byte(b,(uint8_t)v);}
static void u32(Buffer *b,uint32_t v){byte(b,(uint8_t)(v>>24));byte(b,(uint8_t)(v>>16));byte(b,(uint8_t)(v>>8));byte(b,(uint8_t)v);}
static void u64(Buffer *b,uint64_t v){u32(b,(uint32_t)(v>>32));u32(b,(uint32_t)v);}
static void raw(Buffer *b,const uint8_t *v,size_t n){for(size_t i=0;i<n;i++)byte(b,v[i]);}
static void zeros(Buffer *b,size_t n){for(size_t i=0;i<n;i++)byte(b,0);}
static size_t box(Buffer *b,const char *name){size_t start=b->count;u32(b,0);raw(b,(const uint8_t*)name,4);return start;}
static void end(Buffer *b,size_t start){size_t n=b->count-start;if(b->bad||n>UINT32_MAX||start+4>b->count){b->bad=1;return;}for(size_t i=0;i<4;i++)b->bytes[start+i]=(uint8_t)(n>>((3-i)*8));}
static size_t full(Buffer *b,const char *name,uint32_t flags){size_t n=box(b,name);u32(b,flags);return n;}
static void matrix(Buffer *b){u32(b,0x10000);u32(b,0);u32(b,0);u32(b,0);u32(b,0x10000);u32(b,0);u32(b,0);u32(b,0);u32(b,0x40000000);}
static int emit(PMFragmentMuxer *m,Buffer *b){if(!m||m->failed||!m->emit||b->bad||m->emit(m->context,b->bytes,b->count)){if(m)m->failed=1;return -1;}return 0;}
void PMMuxInitialize(PMFragmentMuxer *m,PMMuxEmit callback,void *context){if(!m)return;zero(m,sizeof(*m));m->emit=callback;m->context=context;}
int PMMuxConfigureVideo(PMFragmentMuxer *m,uint32_t width,uint32_t height,const uint8_t *sps,size_t ns,const uint8_t *pps,size_t np){
 if(!m||m->started||m->failed||!width||!height||width>8192||height>8192||!sps||!pps||ns<4||ns>240||np<1||np>240||(sps[0]&31)!=7||(pps[0]&31)!=8)return -1;
 Buffer b={0};byte(&b,1);byte(&b,sps[1]);byte(&b,sps[2]);byte(&b,sps[3]);byte(&b,255);byte(&b,225);u16(&b,(uint32_t)ns);raw(&b,sps,ns);byte(&b,1);u16(&b,(uint32_t)np);raw(&b,pps,np);if(b.bad)return -1;copy(m->avcc,b.bytes,b.count);m->avcc_size=b.count;m->width=width;m->height=height;return 0;
}
int PMMuxConfigureAudio(PMFragmentMuxer *m,uint32_t rate,uint32_t channels,const uint8_t *asc,size_t count){
 static const uint32_t rates[]={96000,88200,64000,48000,44100,32000,24000,22050,16000,12000,11025,8000,7350};
 if(!m||m->started||m->failed||rate<8000||rate>65535||channels<1||channels>2||!asc||count!=2)return -1;
 uint32_t index=((asc[0]&7u)<<1)|(asc[1]>>7);if((asc[0]>>3)!=2||index>=sizeof(rates)/sizeof(rates[0])||rates[index]!=rate||((asc[1]>>3)&15u)!=channels||(asc[1]&7u)!=0)return -1;
 m->audio_rate=rate;m->audio_channels=channels;copy(m->asc,asc,count);m->asc_size=count;return 0;
}
static void descriptor(Buffer *b,uint8_t tag,const Buffer *body){byte(b,tag);size_t n=body->count;uint8_t groups[4];int count=0;do{groups[count++]=(uint8_t)(n&127);n>>=7;}while(n&&count<4);if(n||body->bad){b->bad=1;return;}for(int i=count-1;i>=0;i--)byte(b,groups[i]|(i?128:0));raw(b,body->bytes,body->count);}
static void sample_entry(Buffer *b,PMFragmentMuxer *m,int audio){
 size_t entry=box(b,audio?"mp4a":"avc1");zeros(b,6);u16(b,1);
 if(audio){zeros(b,8);u16(b,m->audio_channels);u16(b,16);u16(b,0);u16(b,0);u32(b,m->audio_rate<<16);
  size_t esds=full(b,"esds",0);Buffer specific={0},decoder={0},es={0},sl={0};raw(&specific,m->asc,m->asc_size);byte(&decoder,0x40);byte(&decoder,0x15);zeros(&decoder,3);u32(&decoder,128000);u32(&decoder,128000);descriptor(&decoder,5,&specific);u16(&es,2);byte(&es,0);descriptor(&es,4,&decoder);byte(&sl,2);descriptor(&es,6,&sl);descriptor(b,3,&es);end(b,esds);
 }else{zeros(b,16);u16(b,m->width);u16(b,m->height);u32(b,0x480000);u32(b,0x480000);u32(b,0);u16(b,1);zeros(b,32);u16(b,24);u16(b,65535);size_t config=box(b,"avcC");raw(b,m->avcc,m->avcc_size);end(b,config);}
 end(b,entry);
}
static void track(Buffer *b,PMFragmentMuxer *m,int audio){
 uint32_t id=audio?2:1,scale=audio?m->audio_rate:90000;size_t trak=box(b,"trak"),tkhd=full(b,"tkhd",7);zeros(b,8);u32(b,id);u32(b,0);u32(b,0);zeros(b,8);u16(b,0);u16(b,0);u16(b,audio?0x100:0);u16(b,0);matrix(b);u32(b,audio?0:m->width<<16);u32(b,audio?0:m->height<<16);end(b,tkhd);
 size_t mdia=box(b,"mdia"),mdhd=full(b,"mdhd",0);zeros(b,8);u32(b,scale);u32(b,0);u16(b,0x55c4);u16(b,0);end(b,mdhd);size_t hdlr=full(b,"hdlr",0);u32(b,0);raw(b,(const uint8_t*)(audio?"soun":"vide"),4);zeros(b,13);end(b,hdlr);
 size_t minf=box(b,"minf"),header=full(b,audio?"smhd":"vmhd",audio?0:1);zeros(b,audio?4:8);end(b,header);size_t dinf=box(b,"dinf"),dref=full(b,"dref",0);u32(b,1);size_t url=full(b,"url ",1);end(b,url);end(b,dref);end(b,dinf);
 size_t stbl=box(b,"stbl"),stsd=full(b,"stsd",0);u32(b,1);sample_entry(b,m,audio);end(b,stsd);
 const char *empty[]={"stts","stsc","stco"};for(int i=0;i<3;i++){size_t at=full(b,empty[i],0);u32(b,0);end(b,at);}size_t stsz=full(b,"stsz",0);u32(b,0);u32(b,0);end(b,stsz);end(b,stbl);end(b,minf);end(b,mdia);end(b,trak);
}
int PMMuxStart(PMFragmentMuxer *m){
 if(!m||m->failed||m->started||!m->avcc_size)return -1;Buffer b={0};size_t ftyp=box(&b,"ftyp");raw(&b,(const uint8_t*)"iso6",4);u32(&b,1);raw(&b,(const uint8_t*)"iso6isomavc1mp41",16);end(&b,ftyp);
 size_t moov=box(&b,"moov"),mvhd=full(&b,"mvhd",0);zeros(&b,8);u32(&b,1000);u32(&b,0);u32(&b,0x10000);u16(&b,0x100);zeros(&b,10);matrix(&b);zeros(&b,24);u32(&b,m->asc_size?3:2);end(&b,mvhd);track(&b,m,0);if(m->asc_size)track(&b,m,1);
 size_t mvex=box(&b,"mvex");for(uint32_t id=1;id<=(m->asc_size?2u:1u);id++){size_t trex=full(&b,"trex",0);u32(&b,id);u32(&b,1);zeros(&b,12);end(&b,trex);}end(&b,mvex);end(&b,moov);if(emit(m,&b))return -1;m->started=1;return 0;
}
static int sample(PMFragmentMuxer *m,uint32_t track_id,const uint8_t *body,size_t count,uint64_t time,uint32_t duration,int key){
 if(!m||m->failed||!m->started||!body||!count||count>(16u<<20)||!duration||duration>900000||time>UINT64_MAX-duration||m->sequence==UINT32_MAX)return -1;
 int *present=track_id==1?&m->have_video:&m->have_audio;uint64_t *prior=track_id==1?&m->last_video:&m->last_audio;if(*present&&time<=*prior)return -1;
 Buffer b={0};size_t moof=box(&b,"moof"),mfhd=full(&b,"mfhd",0);u32(&b,m->sequence+1);end(&b,mfhd);size_t traf=box(&b,"traf"),tfhd=full(&b,"tfhd",0x20000);u32(&b,track_id);end(&b,tfhd);size_t tfdt=full(&b,"tfdt",0x1000000);u64(&b,time);end(&b,tfdt);size_t trun=full(&b,"trun",0x701);u32(&b,1);size_t offset=b.count;u32(&b,0);u32(&b,duration);u32(&b,(uint32_t)count);u32(&b,key?0x2000000:0x1010000);end(&b,trun);end(&b,traf);end(&b,moof);uint32_t data_offset=(uint32_t)b.count+8;for(size_t i=0;i<4;i++)b.bytes[offset+i]=(uint8_t)(data_offset>>((3-i)*8));
 if(emit(m,&b))return -1;Buffer header={0};u32(&header,(uint32_t)count+8);raw(&header,(const uint8_t*)"mdat",4);if(emit(m,&header))return -1;if(m->emit(m->context,body,count)){m->failed=1;return -1;}m->sequence++;*present=1;*prior=time;return 0;
}
int PMMuxVideo(PMFragmentMuxer *m,const uint8_t *body,size_t count,uint64_t time,uint32_t duration,int key){
 if(!body||count<5)return -1;size_t at=0;while(at<count){if(count-at<4)return -1;uint32_t n=((uint32_t)body[at]<<24)|((uint32_t)body[at+1]<<16)|((uint32_t)body[at+2]<<8)|body[at+3];at+=4;if(!n||n>count-at)return -1;at+=n;}return sample(m,1,body,count,time,duration,key!=0);
}
int PMMuxAudio(PMFragmentMuxer *m,const uint8_t *body,size_t count,uint64_t time,uint32_t duration){if(!m||!m->asc_size)return -1;return sample(m,2,body,count,time,duration,1);}
int PMMuxFinish(PMFragmentMuxer *m){return m&&m->started&&!m->failed&&m->have_video?0:-1;}
