#import <Foundation/Foundation.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>

NS_ASSUME_NONNULL_BEGIN
/// H.264/AAC encoder with a synchronous ciphertext sink. No URL/file writer is
/// used: the caller must append every emitted block to authorized SQLCipher.
@interface PMEncryptedEncoder : NSObject
- (nullable instancetype)initWithWidth:(int)width height:(int)height audio:(BOOL)audio
                              bitrate:(int)bitrate emit:(BOOL (^)(NSData *))emit;
- (BOOL)appendVideo:(CVPixelBufferRef)image time:(CMTime)time duration:(CMTime)duration;
- (BOOL)appendAudio:(CMSampleBufferRef)sample;
- (BOOL)finish;
- (void)cancel;
@end
NS_ASSUME_NONNULL_END
