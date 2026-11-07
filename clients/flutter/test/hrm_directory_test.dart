import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_screenutil/flutter_screenutil.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:openim/pages/organization/organization_view.dart';
import 'package:openim_common/openim_common.dart';

LoginCertificate session([String id = 'employee-a']) => LoginCertificate.fromJson({'userID': id, 'sessionToken': 'opaque-$id'});
Map<String, dynamic> person(String id, String number, String departmentID, String department) => {'userID': id, 'nickname': '陈明', 'employeeNo': number, 'departmentID': departmentID, 'department': department, 'position': '工程师', 'faceURL': ''};
final departments = [
  {'id': '1', 'parentID': '', 'name': '总部'}, {'id': '2', 'parentID': '1', 'name': '研发部'}, {'id': '3', 'parentID': '1', 'name': '人事部'},
];
final people = [person('a', '1001', '2', '研发部'), person('b', '1002', '3', '人事部')];

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test('HRM directory sends trimmed phone/name/number search and bounded department pagination', () async {
    Map<String, dynamic>? captured;
    final client = HrmDirectory(session: session, request: (operation, body) async { expect(operation, 'directory'); captured = body; return {'items': people, 'departments': departments, 'total': 2}; });
    final page = await client.list(keyword: ' 1001 ', departmentID: '2', page: -1, pageSize: 1000);
    expect(captured, {'keyword': '1001', 'departmentID': '2', 'page': 1, 'pageSize': 100, 'sessionToken': 'opaque-employee-a'});
    expect(page.items.map((p) => p.employeeNo), ['1001', '1002']);
    expect(page.departments[1].parentID, '1');
  });
  test('missing account session never requests the directory', () async {
    var called = false;
    final client = HrmDirectory(session: () => null, request: (_, __) async { called = true; return {}; });
    await expectLater(client.list(), throwsA(isA<HrmSessionError>().having((e) => e.status, 'status', 401)));
    expect(called, false);
  });
  test('late directory response after account change is discarded', () async {
    LoginCertificate? current = session();
    final completer = Completer<Map<String, dynamic>>();
    final client = HrmDirectory(session: () => current, request: (_, __) => completer.future);
    final result = client.list(); current = session('employee-b');
    completer.complete({'items': people, 'total': 2});
    await expectLater(result, throwsA(isA<HrmSessionError>().having((e) => e.status, 'status', 409)));
  });
  test('person permission is strict server boolean, never an employee ID heuristic', () async {
    for (final permission in [null, false, 'true', 1]) {
      expect(DirectoryPerson.fromJson({'userID': 'emp_any', 'canDirectChat': permission}).canDirectChat, false);
    }
    expect(DirectoryPerson.fromJson({'canDirectChat': true}).canDirectChat, true);
    final client = HrmDirectory(session: session, request: (operation, body) async { expect(operation, 'person'); expect(body['userID'], 'a'); return {...people[0], 'canDirectChat': true}; });
    expect((await client.person('a')).canDirectChat, true);
  });
  test('mismatched person response fails closed', () async {
    final client = HrmDirectory(session: session, request: (_, __) async => {...people[1], 'canDirectChat': true});
    await expectLater(client.person('a'), throwsA(isA<HrmSessionError>()));
  });
  test('null empty collections remain valid on a new tenant', () {
    final page = DirectoryPage.fromJson({'items': null, 'departments': null, 'total': 0});
    expect(page.items, isEmpty); expect(page.departments, isEmpty);
  });
  Future<void> mount(WidgetTester tester, HrmDirectory client, {bool active = true}) async {
    tester.view.physicalSize = const Size(390, 844); tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize); addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(ScreenUtilInit(designSize: const Size(390, 844), builder: (_, __) => MaterialApp(home: OrganizationPage(directory: client, active: active))));
    await tester.pumpAndSettle();
  }
  testWidgets('mobile organization drills into departments and searches namesakes and phone', (tester) async {
    final requests = <Map<String, dynamic>>[];
    final client = HrmDirectory(session: session, request: (_, body) async {
      requests.add(body);
      final keyword = body['keyword'] as String, dept = body['departmentID'] as String;
      final filtered = people.where((p) => (dept.isEmpty || p['departmentID'] == dept) && (keyword.isEmpty || p['nickname'] == keyword || p['employeeNo'] == keyword || (keyword == '13800000002' && p['userID'] == 'b'))).toList();
      return {'items': filtered, 'total': filtered.length, 'departments': departments};
    });
    await mount(tester, client);
    expect(find.text('陈明'), findsNWidgets(2));
    await tester.tap(find.text('总部')); await tester.pumpAndSettle();
    await tester.tap(find.text('研发部')); await tester.pumpAndSettle();
    expect(requests.last['departmentID'], '2'); expect(find.text('陈明'), findsOneWidget);
    await tester.tap(find.text('全部同事')); await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField), '13800000002');
    await tester.testTextInput.receiveAction(TextInputAction.search); await tester.pumpAndSettle();
    expect(requests.last['keyword'], '13800000002');
    expect(find.textContaining('1002'), findsOneWidget); expect(find.textContaining('1001'), findsNothing);
    expect(tester.takeException(), isNull);
  });
  testWidgets('inactive organization tab makes no request and request errors show retry', (tester) async {
    var calls = 0;
    final client = HrmDirectory(session: session, request: (_, __) async { calls++; throw StateError('synthetic failure'); });
    await mount(tester, client, active: false); expect(calls, 0);
    await tester.tap(find.byTooltip('刷新组织架构')); await tester.pumpAndSettle();
    expect(calls, 1); expect(find.text('组织通讯录暂不可用，请重试'), findsOneWidget);
    expect(find.text('该部门暂无可显示的同事'), findsNothing);
  });
}
