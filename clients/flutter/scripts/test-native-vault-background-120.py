"""Run unmodified NativeVaultBroker on JVM with synthetic OS/HTTP/JNI boundaries.

This is a regression for the actual native lease state machine, not a device,
Android lifecycle, real JSON parser, TLS, server or employee-login acceptance.
No source hooks are inserted. All temporary files stay in this task's nonce.
"""
from pathlib import Path
import argparse
import hashlib
import importlib.util
import json
import os
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[3]
JAVA = Path('E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin')
SDK = Path('E:/AndroidDev/sdk')
PACKAGE = 'io/openim/flutter_openim_sdk'


def run(command):
    return subprocess.run([str(value) for value in command], capture_output=True, text=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path)
    parser.add_argument('--case', choices=['expired-renewal'])
    args = parser.parse_args()
    source = ROOT / 'sdks/flutter/android/src/main/java' / PACKAGE
    broker = args.source or source / 'NativeVaultBroker.java'
    harness = ROOT / 'clients/flutter/test/native/NativeVaultBackgroundHarness.java'
    raw = broker.read_bytes()
    spec = importlib.util.spec_from_file_location('snapshot_boundary_doubles', Path(__file__).with_name('test-native-vault-renew-snapshot.py'))
    prior = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(prior)
    stubs = {name: body for name, body in prior.STUBS.items()
             if not name.endswith(('/RenewSnapshotHooks.java', '/RenewSnapshotHarness.java'))}
    stubs.update(STUBS)
    parent = Path('D:/puty-build-temp/native-vault-background-120')
    parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='broker-', dir=parent) as directory:
        work = Path(directory)
        prod = work / 'production'; prod.mkdir()
        doubles = work / 'doubles'; doubles.mkdir()
        captured = work / 'source' / PACKAGE / 'NativeVaultBroker.java'
        captured.parent.mkdir(parents=True)
        captured.write_bytes(raw)
        jar = work / 'go-bindings.jar'
        with zipfile.ZipFile(ROOT / 'sdks/flutter/android/libs/puty-core-sdk-3.8.3-patch15-enterprise.aar') as archive:
            jar.write_bytes(archive.read('classes.jar'))
        embeddings = list((ROOT / '.cache/gradle/caches/modules-2/files-2.1/io.flutter/flutter_embedding_release').rglob('*.jar'))
        if len(embeddings) != 1:
            raise RuntimeError('Expected one pinned Flutter embedding')
        android = SDK / 'platforms/android-35/android.jar'
        if not android.is_file():
            android = SDK / 'platforms/android-34/android.jar'
        cp = os.pathsep.join(map(str, [android, jar, *embeddings]))
        native = ['NativeVaultRenewal', 'NativeVaultSessionState', 'NativeBackupPicker',
                  'EncryptedSelectedFiles', 'EncryptedScratchFiles', 'MobileLegacyQuarantine']
        result = run([JAVA / 'javac.exe', '-encoding', 'UTF-8', '-classpath', cp,
                      '-d', prod, captured, *[source / (name + '.java') for name in native]])
        if result.returncode:
            print(result.stdout, end=''); print(result.stderr, end='')
            return result.returncode
        files = []
        for name, body in stubs.items():
            path = work / 'test-source' / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(body, encoding='utf-8')
            files.append(path)
        result = run([JAVA / 'javac.exe', '-encoding', 'UTF-8', '-classpath',
                      os.pathsep.join([str(prod), cp]), '-d', doubles, *files, harness])
        if result.returncode:
            print(result.stdout, end=''); print(result.stderr, end='')
            return result.returncode
        runtime = work / 'runtime'; runtime.mkdir()
        command = [JAVA / 'java.exe', '-Djava.io.tmpdir=' + str(runtime), '-classpath',
                   os.pathsep.join([str(doubles), str(prod), cp]),
                   'io.openim.flutter_openim_sdk.NativeVaultBackgroundHarness']
        if args.case:
            command.append(args.case)
        result = run(command)
        print(result.stdout, end='')
        if result.returncode:
            print(result.stderr, end='')
        print(json.dumps({'productionBrokerSHA256': hashlib.sha256(raw).hexdigest(),
                          'harnessSHA256': hashlib.sha256(harness.read_bytes()).hexdigest(),
                          'wholeBrokerSourceCompiled': True, 'productionSourceHooks': 0,
                          'actualRenewUnlockCurrentAndLockExecuted': True,
                          'syntheticOSHTTPJSONAndJNIBoundaries': True,
                          'deviceLifecycleOrTLSOrEmployeeAcceptance': False,
                          'productionRequests': 0, 'exitCode': result.returncode}))
        return result.returncode


STUBS = {
'android/os/SystemClock.java': '''package android.os; public final class SystemClock { public static long now=10000; public static long elapsedRealtime(){return now;} }''',
'android/os/Handler.java': '''package android.os; import java.util.*; public final class Handler { public static boolean immediate=true; private static final List<Runnable> pending=new ArrayList<>(); public Handler(Looper value){} public boolean post(Runnable value){if(immediate)value.run();else synchronized(pending){pending.add(value);}return true;} public static void reset(){synchronized(pending){pending.clear();}immediate=true;} public static void drain(){while(true){Runnable next;synchronized(pending){if(pending.isEmpty())return;next=pending.remove(0);}next.run();}} }''',
'android/content/pm/ApplicationInfo.java': '''package android.content.pm; public class ApplicationInfo { public String dataDir=android.content.Context.root.getAbsolutePath(); }''',
'org/json/JSONObject.java': '''package org.json; import java.util.*; public final class JSONObject { public static final ThreadLocal<JSONObject> envelope=new ThreadLocal<>(); private final Map<String,Object> values=new HashMap<>(); public JSONObject(){} public JSONObject(String raw){JSONObject source=envelope.get();if(source==null)throw new IllegalStateException("SYNTHETIC_ENVELOPE_REQUIRED");values.putAll(source.values);} public JSONObject put(String key,Object value){values.put(key,value);return this;} public Object opt(String key){return values.get(key);} public int optInt(String key,int fallback){Object value=values.get(key);return value instanceof Number?((Number)value).intValue():fallback;} public JSONObject getJSONObject(String key){return (JSONObject)values.get(key);} }''',
'org/json/JSONArray.java': '''package org.json; public final class JSONArray { public JSONArray(){} public JSONArray put(Object value){return this;} public String toString(){return "[]";} }''',
'io/flutter/plugin/common/MethodChannel.java': '''package io.flutter.plugin.common; import java.util.*; public final class MethodChannel { public interface Result {void success(Object value);void error(String code,String message,Object details);void notImplemented();} public final List<String> reasons=new ArrayList<>(); public void invokeMethod(String method,Object argument){if(!"onVaultLocked".equals(method))throw new AssertionError("UNEXPECTED_CHANNEL_METHOD");reasons.add((String)argument);} }''',
'io/openim/flutter_openim_sdk/EncryptedScratchFiles.java': '''package io.openim.flutter_openim_sdk; public final class EncryptedScratchFiles { public interface Authority {String authorizedAccount();long generation();} public static void closeAll(){} public static boolean owns(String path){return false;} }''',
'open_im_sdk/Open_im_sdk.java': '''package open_im_sdk; public final class Open_im_sdk { public static int locks,configurations,reads; public static String configuredAccount,lockedAccount; public static boolean rejectConfiguration; public static void lockNativeLocalStore(String account){locks++;lockedAccount=account;configuredAccount=null;} public static boolean recoverNativeStoreRoot(String root){return true;} public static boolean configureNativeLocalStore(String account,String root,byte[] key,long deadline){if(key==null||key.length!=32||deadline<=System.currentTimeMillis())throw new AssertionError("INVALID_NATIVE_AUTHORIZATION");if(rejectConfiguration)return false;configurations++;configuredAccount=account;return true;} public static boolean configureNativeLegacyQuarantine(String account,String root,byte[] key){return key.length==32;} public static boolean setNativeLegacyRoots(String account,String roots){return true;} public static byte[] readNativeSource(String account,String path,long max){reads++;return new byte[]{7};} public static void reset(){locks=0;configurations=0;reads=0;configuredAccount=null;lockedAccount=null;rejectConfiguration=false;} }''',
'io/openim/flutter_openim_sdk/MobileLegacyQuarantine.java': '''package io.openim.flutter_openim_sdk; import java.io.*; public final class MobileLegacyQuarantine { public static MobileLegacyQuarantine create(android.content.Context context){return new MobileLegacyQuarantine();} public byte[] masterCopy(){return new byte[32];} public File directory(){return android.content.Context.root;} public void sealRemaining(){} }''',
}

if __name__ == '__main__':
    raise SystemExit(main())
