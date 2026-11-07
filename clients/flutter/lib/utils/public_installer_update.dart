import 'dart:io';

import 'package:path_provider/path_provider.dart';

import 'release_policy.dart';

typedef PublicInstallerDownload = Future<void> Function(String path);
typedef PublicInstallerLaunch = Future<void> Function(String path);

class PublicInstallerFailure implements Exception {
  PublicInstallerFailure(this.code, this.cause);
  final String code;
  final Object cause;
  @override
  String toString() => code;
}

/// Public, signed application packages are independent of account archives.
/// Only these packages use plaintext app cache; chat media never enters it.
class PublicInstallerUpdate {
  PublicInstallerUpdate({Future<Directory> Function()? cacheDirectory})
      : _cacheDirectory = cacheDirectory ?? getTemporaryDirectory;

  final Future<Directory> Function() _cacheDirectory;

  Future<String> run({
    required String fileName,
    required int expectedSize,
    required String expectedSHA256,
    required PublicInstallerDownload download,
    required PublicInstallerLaunch launch,
  }) async {
    Directory? task;
    var stage = 'PUTY_UPDATE_CACHE';
    try {
      if (!RegExp(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,175}\.apk$')
              .hasMatch(fileName) ||
          expectedSize <= 0 ||
          !RegExp(r'^[a-fA-F0-9]{64}$').hasMatch(expectedSHA256)) {
        throw StateError('Invalid public installer metadata');
      }
      final cache = await _cacheDirectory();
      final root = Directory('${cache.path}/puty-public-installers');
      await root.create(recursive: true);
      task = await root.createTemp('download-');
      final partial = File('${task.path}/$fileName.part');
      stage = 'PUTY_UPDATE_DOWNLOAD';
      await download(partial.path);
      stage = 'PUTY_UPDATE_VERIFY';
      await ReleasePolicy.verifyPackage(partial, expectedSize, expectedSHA256);
      final handle = await partial.open(mode: FileMode.append);
      try {
        await handle.flush();
      } finally {
        await handle.close();
      }
      final package = await partial.rename('${task.path}/$fileName');
      stage = 'PUTY_UPDATE_INSTALL';
      await launch(package.path);
      // The system installer reads asynchronously after its activity opens.
      // Keep this unique cache task; do not delete or overwrite its APK here.
      return package.path;
    } catch (error, stack) {
      if (task != null) {
        try {
          for (final name in [fileName, '$fileName.part']) {
            final file = File('${task.path}/$name');
            if (await FileSystemEntity.type(file.path, followLinks: false) ==
                FileSystemEntityType.file) {
              await file.delete();
            }
          }
          await task.delete();
        } catch (_) {
          // A failed cleanup must not hide the fixed original diagnostic.
        }
      }
      Error.throwWithStackTrace(PublicInstallerFailure(stage, error), stack);
    }
  }
}
