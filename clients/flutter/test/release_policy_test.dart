import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim/utils/release_policy.dart';

void main() {
  test('semantic version ordering, build ordering, and downgrade rejection',
      () {
    expect(ReleasePolicy.isNewer('3.8.10', 1, '3.8.9', 100), isTrue);
    expect(ReleasePolicy.isNewer('3.8.3', 101, '3.8.3', 100), isTrue);
    expect(ReleasePolicy.isNewer('3.8.3', 100, '3.8.3', 100), isFalse);
    expect(ReleasePolicy.isNewer('3.8.2', 999, '3.8.3', 100), isFalse);
    expect(ReleasePolicy.isNewer('3.8.3-alpha.1', 101, '3.8.3', 100), isFalse);
    expect(ReleasePolicy.isNewer('invalid', 999, '3.8.3', 100), isFalse);
  });
  test('install requires complete size and SHA-256 validation', () async {
    final dir = await Directory.systemTemp.createTemp('openim38-release-test-');
    addTearDown(() => dir.delete(recursive: true));
    final file = File('${dir.path}/package.apk');
    await file.writeAsBytes([0, 1, 2, 3, 4]);
    final hash = sha256.convert([0, 1, 2, 3, 4]).toString();
    await ReleasePolicy.verifyPackage(file, 5, hash);
    await expectLater(
        ReleasePolicy.verifyPackage(file, 4, hash), throwsStateError);
    await expectLater(
        ReleasePolicy.verifyPackage(file, 5, '0' * 64), throwsStateError);
    await expectLater(
        ReleasePolicy.verifyPackage(file, 5, ''), throwsStateError);
  });
}
