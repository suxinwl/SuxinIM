import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:pub_semver/pub_semver.dart';

class ReleasePolicy {
  static bool isNewer(String nextVersion, int nextBuild, String currentVersion,
      int currentBuild) {
    if (nextBuild < 0 || currentBuild < 0) return false;
    try {
      final comparison =
          Version.parse(nextVersion).compareTo(Version.parse(currentVersion));
      return comparison > 0 || comparison == 0 && nextBuild > currentBuild;
    } on FormatException {
      return false;
    }
  }

  static Future<void> verifyPackage(
      File file, int expectedSize, String expectedHash) async {
    if (expectedSize <= 0 ||
        !RegExp(r'^[a-fA-F0-9]{64}$').hasMatch(expectedHash)) {
      throw StateError('Missing package integrity metadata');
    }
    if (await file.length() != expectedSize)
      throw StateError('Package size mismatch');
    final digest = await sha256.bind(file.openRead()).first;
    if (digest.toString() != expectedHash.toLowerCase())
      throw StateError('Package SHA-256 mismatch');
  }
}
