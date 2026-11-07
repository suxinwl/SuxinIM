"""Compile the actual Android IMManager and exercise Flutter result liveness.

Only the synthetic platform boundary is replaced. Generated classes and doubles
live in a fresh, task-owned temporary directory; no APK, device or account used.
"""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
JAVA = Path(r'E:/AndroidDev/jdk/jdk-17.0.20.1+1/bin')
SOURCE = ROOT / 'sdks/flutter/android/src/main/java/io/openim/flutter_openim_sdk'
STUBS = {
    'io/flutter/plugin/common/MethodCall.java': '''package io.flutter.plugin.common;
public class MethodCall { public Object arguments; public MethodCall(Object a){arguments=a;}
@SuppressWarnings("unchecked") public <T>T argument(String k){return (T)((java.util.Map<?,?>)arguments).get(k);} }''',
    'io/flutter/plugin/common/MethodChannel.java': '''package io.flutter.plugin.common;
public class MethodChannel { public interface Result {void success(Object v);void error(String c,String m,Object d);void notImplemented();} }''',
    'io/flutter/Log.java': 'package io.flutter; public class Log {}',
    'io/openim/flutter_openim_sdk/FlutterOpenimSdkPlugin.java': '''package io.openim.flutter_openim_sdk;
public class FlutterOpenimSdkPlugin { public static volatile boolean isInitialized; }''',
    'io/openim/flutter_openim_sdk/util/CommonUtil.java': '''package io.openim.flutter_openim_sdk.util;
import io.flutter.plugin.common.MethodChannel;
public class CommonUtil { public static void runMainThreadReturn(MethodChannel.Result r,Object v){r.success(v);}
public static void runMainThreadReturnError(MethodChannel.Result r,String c,String m,Object d){r.error(c,m,d);}
public static void runMainThreadReturnError(MethodChannel.Result r,long c,String m,Object d){r.error(String.valueOf(c),m,d);} }''',
    'io/openim/flutter_openim_sdk/util/JsonUtil.java': '''package io.openim.flutter_openim_sdk.util;
public class JsonUtil { public static String toString(Object o){return "{}";} }''',
    'open_im_sdk_callback/Base.java': '''package open_im_sdk_callback;
public interface Base {void onError(int c,String m);void onSuccess(String s);}''',
    'open_im_sdk/Open_im_sdk.java': '''package open_im_sdk;
import java.util.concurrent.CountDownLatch;import java.util.concurrent.atomic.AtomicInteger;
public class Open_im_sdk {
public static volatile String failure; public static volatile boolean initValue=true;
public static volatile CountDownLatch uninitEntered,uninitRelease; public static AtomicInteger initCalls=new AtomicInteger();
private static void fail(String op){if((op+"Exception").equals(failure))throw new IllegalStateException("synthetic-secret");
if((op+"Linkage").equals(failure))throw new UnsatisfiedLinkError("synthetic-secret");}
public static boolean initSDK(Object... a){initCalls.incrementAndGet();fail("init");return initValue;}
public static void unInitSDK(String operationID){fail("uninit");if(uninitEntered!=null)uninitEntered.countDown();
try{if(uninitRelease!=null)uninitRelease.await();}catch(InterruptedException e){Thread.currentThread().interrupt();throw new IllegalStateException(e);}}
public static void login(Object... a){} public static void logout(Object... a){} public static int getLoginStatus(String operationID){return 1;}
public static void uploadFile(Object... a){} public static void updateFcmToken(Object... a){}
public static void uploadLogs(Object... a){} public static void logs(Object... a){}
public static void setAppBackgroundStatus(Object... a){} public static void networkStatusChanged(Object... a){}
}''',
}
for listener in ['OnConnListener', 'OnUploadFileListener', 'OnUploadLogsListener']:
    STUBS[f'io/openim/flutter_openim_sdk/listener/{listener}.java'] = (
        'package io.openim.flutter_openim_sdk.listener;'
        f'public class {listener} {{ public {listener}(Object... args){{}} }}'
    )


def main():
    task_tmp = Path(os.environ.get('TEMP', r'D:/puty-build-temp')).resolve()
    task_tmp.mkdir(parents=True, exist_ok=True)
    sources = [SOURCE / 'manager/IMManager.java', SOURCE / 'manager/BaseManager.java',
               SOURCE / 'listener/OnBaseListener.java',
               ROOT / 'clients/flutter/test/native/IMManagerLifecycleHarness.java']
    with tempfile.TemporaryDirectory(prefix='puty-im-lifecycle-', dir=task_tmp) as temporary:
        directory = Path(temporary).resolve()
        if directory.parent != task_tmp or not directory.name.startswith('puty-im-lifecycle-') or directory.is_symlink():
            raise RuntimeError('Synthetic lifecycle scratch scope rejected')
        for name, content in STUBS.items():
            path = directory / 'doubles' / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding='utf-8')
        classes = directory / 'classes'
        classes.mkdir()
        compile_result = subprocess.run([str(JAVA / 'javac.exe'), '-encoding', 'UTF-8', '--release', '11',
            '-d', str(classes), *map(str, sources),
            *map(str, sorted((directory / 'doubles').rglob('*.java')))], capture_output=True, text=True, timeout=30)
        print(compile_result.stdout, end='')
        print(compile_result.stderr, end='')
        if compile_result.returncode:
            return compile_result.returncode
        print(json.dumps({'productionSourceSHA256': hashlib.sha256(sources[0].read_bytes()).hexdigest(),
                          'nativeGoAndFlutterAreSynthetic': True}), flush=True)
        result = subprocess.run([str(JAVA / 'java.exe'), '-cp', str(classes),
            'io.openim.flutter_openim_sdk.manager.IMManagerLifecycleHarness'], capture_output=True, text=True, timeout=15)
        print(result.stdout, end='')
        print(result.stderr, end='')
        return result.returncode


if __name__ == '__main__':
    raise SystemExit(main())
