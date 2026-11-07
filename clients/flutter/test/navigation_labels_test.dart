import 'package:flutter_test/flutter_test.dart';
import 'package:openim_common/src/res/lang/en_US.dart';
import 'package:openim_common/src/res/lang/zh_CN.dart';

void main() {
  test('Chinese primary navigation identifies messages independently of app branding', () {
    expect(zh_CN['home'], '消息');
  });
  test('English primary navigation identifies messages independently of app branding', () {
    expect(en_US['home'], 'Messages');
  });
}
