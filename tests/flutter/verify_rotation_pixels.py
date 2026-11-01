"""Compare the native compressed video and thumbnail against correctly rotated fixture pixels."""
import hashlib,json,subprocess,math
from pathlib import Path
from PIL import Image,ImageChops,ImageStat
ROOT=Path(__file__).absolute().parents[2]
report=json.loads((ROOT/'tests/flutter/native-sdk-report.json').read_text())
media=next(x for x in report['media'] if x['fixture']=='synthetic-rotate90.mp4')
out=ROOT/'tests/flutter/media-rotation';out.mkdir(exist_ok=True)
adb=['E:/AndroidDev/sdk/platform-tools/adb.exe','-s','emulator-5588']
for field,name in [('thumbnailPath','native-thumbnail.jpg'),('outputPath','native-compressed.mp4')]:
 remote=media[field]
 owned_prefixes=('/data/user/0/io.openim.upgrade38.test.dev/',
                 '/data/data/io.openim.upgrade38.test.dev/',
                 '/storage/emulated/0/Android/data/io.openim.upgrade38.test.dev/files/video_compress/')
 if not remote.startswith(owned_prefixes) or '..' in Path(remote).parts:
  raise RuntimeError('Refusing unexpected test media path')
 subprocess.run(adb+['pull',remote,str(out/name)],check=True,capture_output=True)
ffmpeg=ROOT/'.cache/media-tools/ffmpeg-win-x86_64-v7.1.exe'
fixture=Path(report.get('rotationFixture',ROOT/'.cache/media-fixtures/synthetic-rotate90.mp4'))
if not fixture.resolve().is_relative_to(ROOT.resolve()):raise RuntimeError('Fixture outside upgrade workspace')
for source,destination in [(fixture,out/'expected-frame.png'),(out/'native-compressed.mp4',out/'native-frame.png')]:
 subprocess.run([str(ffmpeg),'-hide_banner','-loglevel','error','-i',str(source),'-frames:v','1',str(destination),'-y'],check=True,capture_output=True)
expected=Image.open(out/'expected-frame.png').convert('RGB')
def compare(path):
 image=Image.open(path).convert('RGB');original=image.size
 image=image.resize(expected.size)
 def rmse(other):return math.sqrt(sum(x*x for x in ImageStat.Stat(ImageChops.difference(image,other)).rms)/3)
 correct=rmse(expected);inverted=rmse(expected.transpose(Image.Transpose.ROTATE_180))
 return {'decodedSize':original,'rmseCorrectOrientation':correct,'rmseUpsideDown':inverted,'passed':original[0]<original[1] and correct<30 and correct<inverted*.6}
checks={'compressedFrame':compare(out/'native-frame.png'),'nativeThumbnail':compare(out/'native-thumbnail.jpg')}
result={'applicationVersion':report.get('applicationVersion'),'applicationBuild':report.get('applicationBuild'),'apkSHA256':report.get('apkSHA256'),'runtimeAPKABI':report.get('runtimeAPKABI'),'fixtureSHA256':hashlib.file_digest(fixture.open('rb'),'sha256').hexdigest(),'fixture':str(fixture),'ffmpegDisplayMatrixCounterClockwiseDegrees':90,'androidClockwiseOrientation':media['inputOrientation'],'nativeMetadata':media,'pixelComparisons':checks,'references':['https://www.ffmpeg.org/doxygen/trunk/group__lavu__video__display.html','https://developer.android.com/reference/android/media/MediaFormat#KEY_ROTATION'],'passed':all(c['passed'] for c in checks.values()),'device':report['device'],'androidAPI':report['androidAPI'],'physicalDevice':False}
(out/'rotation-report.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result))
if not result['passed']:raise SystemExit(1)
