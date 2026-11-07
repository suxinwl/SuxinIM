import 'dart:async';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/openim_common.dart';
import 'package:openim/utils/public_installer_update.dart';

void main() {
  late Directory cache;
  late PublicInstallerUpdate updater;
  final bytes = List<int>.generate(128, (i) => i);
  final expectedHash = sha256.convert(bytes).toString();
  const fileName = 'PUTY_IM-3.8.21-android-arm64.apk';

  setUp(() async {
    cache = await Directory.systemTemp.createTemp('public-update-test-');
    updater = PublicInstallerUpdate(cacheDirectory: () async => cache);
  });
  tearDown(() async {
    final temporaryRoot = await Directory.systemTemp.resolveSymbolicLinks();
    final resolved = await cache.resolveSymbolicLinks();
    expect(
        resolved.startsWith(
            '$temporaryRoot${Platform.pathSeparator}public-update-test-'),
        isTrue);
    await Directory(resolved).delete(recursive: true);
  });

  Future<String> run(
          {PublicInstallerDownload? download,
          PublicInstallerLaunch? launch,
          String? hash,
          int? size,
          String name = fileName}) =>
      updater.run(
        fileName: name,
        expectedSize: size ?? bytes.length,
        expectedSHA256: hash ?? expectedHash,
        download: download ??
            (p) async {
              await File(p).writeAsBytes(bytes);
            },
        launch: launch ?? (_) async {},
      );

  test('public package downloads before login without any archive channel',
      () async {
    String? downloadPath;
    String? installPath;
    final path = await run(download: (p) async {
      downloadPath = p;
      expect(p.endsWith('.apk.part'), isTrue);
      await File(p).writeAsBytes(bytes);
    }, launch: (p) async {
      installPath = p;
      expect(await File(p).readAsBytes(), bytes);
      expect(p.endsWith('/$fileName'), isTrue);
      expect(await File(downloadPath!).exists(), isFalse);
    });
    expect(path, installPath);
    final installerRoot = await Directory(
            '${cache.path}/puty-public-installers')
        .resolveSymbolicLinks();
    expect(await File(path).parent.parent.resolveSymbolicLinks(), installerRoot);
    // The installer may still be reading after its activity has been launched.
    expect(await File(path).readAsBytes(), bytes);
  });

  test(
      'old media temporary path reproduces reported error while public update succeeds',
      () async {
    expect(NativeLocalVault.unlocked, isFalse);
    await expectLater(
        IMUtils.createTempFile(dir: 'apk', name: fileName),
        throwsA(isA<StateError>()
            .having((e) => e.message, 'reported media gate', '请联网验证身份后创建媒体')));
    final path = await run();
    expect(await File(path).readAsBytes(), bytes);
    expect(NativeLocalVault.unlocked, isFalse);
  });

  test('concurrent updates never overwrite the APK the installer is reading',
      () async {
    final bothDownloaded = Completer<void>();
    var downloads = 0;
    Future<void> download(String path) async {
      await File(path).writeAsBytes(bytes);
      if (++downloads == 2) bothDownloaded.complete();
      await bothDownloaded.future;
    }

    final paths =
        await Future.wait([run(download: download), run(download: download)]);
    expect(paths[0], isNot(paths[1]));
    for (final path in paths) {
      expect(await File(path).readAsBytes(), bytes);
      expect(File(path).uri.pathSegments.last, fileName);
    }
  });

  test('hash mismatch blocks install and removes only its failed task',
      () async {
    var installed = false;
    final retained = await run();
    await expectLater(
        run(
            hash: '0' * 64,
            launch: (_) async {
              installed = true;
            }),
        throwsA(isA<PublicInstallerFailure>()
            .having((e) => e.code, 'stage', 'PUTY_UPDATE_VERIFY')));
    expect(installed, isFalse);
    expect(await File(retained).readAsBytes(), bytes);
    final tasks =
        await Directory('${cache.path}/puty-public-installers').list().toList();
    expect(tasks.length, 1);
  });

  test('incomplete download is never passed to the installer', () async {
    var installed = false;
    await expectLater(
        run(download: (p) async {
          await File(p).writeAsBytes([1]);
        }, launch: (_) async {
          installed = true;
        }),
        throwsA(isA<PublicInstallerFailure>()
            .having((e) => e.code, 'stage', 'PUTY_UPDATE_VERIFY')));
    expect(installed, isFalse);
    expect(
        await Directory('${cache.path}/puty-public-installers').list().isEmpty,
        isTrue);
  });

  test('download failure is redacted and the same version can retry', () async {
    await expectLater(run(download: (p) async {
      await File(p).writeAsBytes([1, 2]);
      throw StateError('synthetic-sensitive-transport-detail');
    }),
        throwsA(isA<PublicInstallerFailure>().having(
            (e) => e.toString(), 'fixed diagnostic', 'PUTY_UPDATE_DOWNLOAD')));
    final path = await run();
    expect(await File(path).readAsBytes(), bytes);
  });

  test('native installer rejection preserves retry and removes failed package',
      () async {
    await expectLater(run(launch: (_) async {
      throw StateError('synthetic rejection');
    }),
        throwsA(isA<PublicInstallerFailure>()
            .having((e) => e.code, 'stage', 'PUTY_UPDATE_INSTALL')));
    expect(
        await Directory('${cache.path}/puty-public-installers').list().isEmpty,
        isTrue);
    expect(await File(await run()).exists(), isTrue);
  });

  test('invalid file names and metadata fail before allocating cache',
      () async {
    var allocated = false;
    updater = PublicInstallerUpdate(cacheDirectory: () async {
      allocated = true;
      return cache;
    });
    for (final name in [
      '../package.apk',
      r'..\package.apk',
      '/package.apk',
      'package.zip'
    ]) {
      await expectLater(
          run(name: name), throwsA(isA<PublicInstallerFailure>()));
    }
    await expectLater(run(size: 0), throwsA(isA<PublicInstallerFailure>()));
    await expectLater(
        run(hash: 'invalid'), throwsA(isA<PublicInstallerFailure>()));
    expect(allocated, isFalse);
  });

  test('unavailable app cache gives a retryable fixed diagnostic', () async {
    updater = PublicInstallerUpdate(cacheDirectory: () async {
      throw FileSystemException('synthetic storage failure');
    });
    await expectLater(
        run(),
        throwsA(isA<PublicInstallerFailure>().having(
            (e) => e.toString(), 'fixed diagnostic', 'PUTY_UPDATE_CACHE')));
  });
}
