"""Compile actual Broker/JNI transition races in an isolated no-INTERNET APK."""
from pathlib import Path
import importlib.util,os,subprocess,tempfile,zipfile,time,hashlib
ROOT=Path(__file__).resolve().parents[3]
spec=importlib.util.spec_from_file_location('scratch_probe',ROOT/'tools/android_encrypted_scratch_probe.py')
base=importlib.util.module_from_spec(spec);spec.loader.exec_module(base)
run,SDK,JDK,BUILD=base.run,base.SDK,base.JDK,base.BUILD
PACKAGE='com.puty.test.vaulttransitions'

def main():
    output=ROOT/'.cache/android-native-vault-races';output.mkdir(parents=True,exist_ok=True)
    temporary=Path('D:/PUTY-local-build/mobile-temp');temporary.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='puty-vault-races-',dir=temporary) as work:
        work=Path(work);classes=work/'classes';classes.mkdir();jars=[];native={}
        for name in ('puty-core-sdk-3.8.3-patch15-enterprise.aar','puty-sqlcipher-android-4.19.1.aar'):
            with zipfile.ZipFile(ROOT/'sdks/flutter/android/libs'/name) as archive:
                jar=work/(name+'.jar');jar.write_bytes(archive.read('classes.jar'));jars.append(jar)
                for entry in archive.namelist():
                    if entry.startswith('jni/x86_64/') and entry.endswith('.so'):native[entry.replace('jni/','lib/',1)]=archive.read(entry)
        embedding=list((ROOT/'.cache/gradle/caches/modules-2/files-2.1/io.flutter/flutter_embedding_release').rglob('*.jar'))
        if len(embedding)!=1:raise RuntimeError('Expected one fixed local Flutter embedding')
        jars+=embedding;android=SDK/'platforms/android-34/android.jar'
        manifest=work/'AndroidManifest.xml'
        manifest.write_text('<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="'+PACKAGE+'"><uses-sdk android:minSdkVersion="26" android:targetSdkVersion="28"/><application android:label="Offline vault transition races" android:debuggable="true" android:allowBackup="false"><activity android:name=".NativeVaultRaceActivity" android:exported="true"/></application></manifest>',encoding='utf8')
        source=ROOT/'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk'
        files=[source/(name+'.java') for name in ('NativeVaultBroker','NativeVaultRenewal','NativeVaultSessionState','NativeBackupPicker','EncryptedSelectedFiles','EncryptedScratchFiles','MobileLegacyQuarantine')]
        files+=[ROOT/'clients/flutter/test/native/NativeVaultRaceActivity.java']
        boot=os.pathsep.join(map(str,(android,BUILD/'core-lambda-stubs.jar')))
        run([JDK/'bin/javac.exe','-encoding','UTF-8','-source','8','-target','8','-bootclasspath',boot,'-classpath',os.pathsep.join(map(str,jars)),'-d',classes,*files])
        dex=work/'dex';dex.mkdir()
        run([BUILD/'d8.bat','--min-api','26','--lib',android,'--output',dex,*jars,*classes.rglob('*.class')])
        unsigned=work/'unsigned.apk';run([BUILD/'aapt.exe','package','-f','-M',manifest,'-I',android,'-F',unsigned])
        with zipfile.ZipFile(unsigned,'a',compression=zipfile.ZIP_DEFLATED) as archive:
            for file in dex.glob('*.dex'):archive.write(file,file.name)
            for name,data in native.items():archive.writestr(name,data)
        key=work/'synthetic.jks'
        run([JDK/'bin/keytool.exe','-genkeypair','-noprompt','-keystore',key,'-storepass','android','-keypass','android','-alias','synthetic','-dname','CN=Offline Synthetic Vault Test','-keyalg','RSA','-validity','2'])
        apk=output/'offline-native-vault-races.apk'
        run([BUILD/'apksigner.bat','sign','--ks',key,'--ks-pass','pass:android','--key-pass','pass:android','--out',apk,unsigned])
        print('offlineAPK_SHA256='+hashlib.sha256(apk.read_bytes()).hexdigest())
        adb=[SDK/'platform-tools/adb.exe','-s','emulator-5582']
        subprocess.run([str(x) for x in [*adb,'uninstall',PACKAGE]],capture_output=True,check=False)
        run([*adb,'install','-r',apk]);run([*adb,'logcat','-c'])
        run([*adb,'shell','am','start','-n',PACKAGE+'/.NativeVaultRaceActivity'])
        try:
            for _ in range(60):
                time.sleep(1);lines=run([*adb,'logcat','-d','-s','PutyVaultRace:I','*:S']).stdout
                if 'COMPLETE' in lines or 'FAIL ' in lines:
                    safe='\n'.join(line for line in lines.splitlines() if 'PutyVaultRace' in line)
                    (output/'result.txt').write_text(safe,encoding='utf8');print(safe)
                    if 'FAIL ' in lines:raise RuntimeError('Actual native Broker transition race failed')
                    return
            raise RuntimeError('Offline transition probe timed out')
        finally:run([*adb,'shell','am','force-stop',PACKAGE])

if __name__=='__main__':main()
