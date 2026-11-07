"""Build/run an isolated, no-INTERNET, synthetic API26+ FUSE probe APK.

No production Activity, signing key, account, server, or credential is used.
Build inputs are existing local AARs and the actual production Java bridge.
"""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

ROOT=Path(__file__).resolve().parents[1]
SDK=Path('E:/AndroidDev/sdk')
JDK=Path('E:/AndroidDev/jdk/jdk-17.0.20.1+1')
BUILD=SDK/'build-tools/34.0.0'
PACKAGE='com.puty.test.encryptedscratch'

def run(argv, **kwargs):
    env=dict(os.environ, JAVA_HOME=str(JDK))
    env['PATH']=str(JDK/'bin')+os.pathsep+env.get('PATH','')
    result=subprocess.run([str(x) for x in argv],env=env,capture_output=True,text=True,**kwargs)
    if result.returncode:
        # These commands contain only public build inputs and synthetic keys.
        print(result.stderr[-12000:])
        raise RuntimeError('Offline probe command failed: '+Path(argv[0]).name)
    return result

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--serial',default='emulator-5582')
    parser.add_argument('--run',action='store_true')
    args=parser.parse_args()
    output=ROOT/'.cache/android-encrypted-scratch-probe';output.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='puty-scratch-build-') as temporary:
        work=Path(temporary);classes=work/'classes';classes.mkdir()
        jars=[];native={}
        for name in ('puty-core-sdk-3.8.3-patch15-enterprise.aar','puty-sqlcipher-android-4.19.1.aar'):
            with zipfile.ZipFile(ROOT/'sdks/flutter/android/libs'/name) as archive:
                jar=work/(name+'.jar');jar.write_bytes(archive.read('classes.jar'));jars.append(jar)
                for entry in archive.namelist():
                    if entry.startswith('jni/x86_64/') and entry.endswith('.so'):native[entry.replace('jni/','lib/',1)]=archive.read(entry)
        android=SDK/'platforms/android-34/android.jar'
        manifest=work/'AndroidManifest.xml'
        manifest.write_text('<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="'+PACKAGE+'"><uses-sdk android:minSdkVersion="26" android:targetSdkVersion="28"/><application android:label="Offline encrypted scratch probe" android:debuggable="true" android:allowBackup="false"><activity android:name=".ProbeActivity" android:exported="true"/><provider android:name="io.openim.flutter_openim_sdk.EncryptedScratchProvider" android:authorities="'+PACKAGE+'.puty-encrypted-scratch" android:exported="false" android:grantUriPermissions="false"/></application></manifest>')
        source=ROOT/'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk/EncryptedScratchFiles.java'
        probe=ROOT/'tools/android-scratch-probe/ProbeActivity.java'
        boot=os.pathsep.join(map(str,(android,BUILD/'core-lambda-stubs.jar')))
        provider=source.with_name('EncryptedScratchProvider.java')
        run([JDK/'bin/javac.exe','-encoding','UTF-8','-source','8','-target','8','-bootclasspath',boot,'-classpath',os.pathsep.join(map(str,jars)),'-d',classes,source,provider,probe])
        dex=work/'dex';dex.mkdir()
        run([BUILD/'d8.bat','--min-api','26','--lib',android,'--output',dex,*jars,*classes.rglob('*.class')])
        unsigned=work/'unsigned.apk';run([BUILD/'aapt.exe','package','-f','-M',manifest,'-I',android,'-F',unsigned])
        with zipfile.ZipFile(unsigned,'a',compression=zipfile.ZIP_DEFLATED) as archive:
            for file in dex.glob('*.dex'):archive.write(file,file.name)
            for name,body in native.items():archive.writestr(name,body)
        key=work/'synthetic.jks'
        run([JDK/'bin/keytool.exe','-genkeypair','-noprompt','-keystore',key,'-storepass','android','-keypass','android','-alias','synthetic','-dname','CN=Offline Synthetic Test','-keyalg','RSA','-validity','2'])
        apk=output/'offline-encrypted-scratch.apk'
        run([BUILD/'apksigner.bat','sign','--ks',key,'--ks-pass','pass:android','--key-pass','pass:android','--out',apk,unsigned])
        print('apkSHA256='+hashlib.sha256(apk.read_bytes()).hexdigest())
        if args.run:
            adb=[SDK/'platform-tools/adb.exe','-s',args.serial]
            # This exact test package has no account or production state.
            subprocess.run([str(x) for x in [*adb,'uninstall',PACKAGE]],capture_output=True,check=False)
            run([*adb,'install','-r',apk])
            run([*adb,'logcat','-c'])
            run([*adb,'shell','am','start','-n',PACKAGE+'/.ProbeActivity'])
            import time
            for _ in range(45):
                time.sleep(1)
                lines=run([*adb,'logcat','-d','-s','PutyScratchProbe:I','*:S']).stdout
                if 'COMPLETE' in lines or 'FAIL ' in lines:
                    safe='\n'.join(line for line in lines.splitlines() if 'PutyScratchProbe' in line)
                    print(safe)
                    (output/'result.txt').write_text(safe,encoding='utf-8')
                    if 'FAIL ' in lines:raise RuntimeError('Offline scratch probe failed')
                    break
            else:raise RuntimeError('Offline scratch probe timeout')
            run([*adb,'shell','am','force-stop',PACKAGE])

if __name__=='__main__':main()
