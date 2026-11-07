import 'package:openim_common/openim_common.dart';

class DirectoryDepartment {
  final String id, parentID, name, leaderUserID, leaderName;
  final bool groupEnabled;
  DirectoryDepartment.fromJson(Map<String, dynamic> value)
      : id = '${value['id'] ?? ''}',
        parentID = '${value['parentID'] ?? ''}',
        name = value['name']?.toString() ?? '',
        leaderUserID = value['leaderUserID']?.toString() ?? '',
        leaderName = value['leaderName']?.toString() ?? '',
        groupEnabled = value['groupEnabled'] == true;
}

class DirectoryPerson {
  final String userID, nickname, employeeNo, department, departmentID, position, faceURL;
  final bool canDirectChat;
  DirectoryPerson.fromJson(Map<String, dynamic> value)
      : userID = value['userID']?.toString() ?? '',
        nickname = value['nickname']?.toString() ?? '',
        employeeNo = value['employeeNo']?.toString() ?? '',
        department = value['department']?.toString() ?? '',
        departmentID = '${value['departmentID'] ?? ''}',
        position = value['position']?.toString() ?? '',
        faceURL = value['faceURL']?.toString() ?? '',
        canDirectChat = value['canDirectChat'] == true;
}

class DirectoryPage {
  final List<DirectoryPerson> items;
  final List<DirectoryDepartment> departments;
  final int total;
  DirectoryPage.fromJson(Map<String, dynamic> value)
      : items = (value['items'] as List? ?? [])
            .whereType<Map>().map((item) => DirectoryPerson.fromJson(Map<String, dynamic>.from(item))).toList(),
        departments = (value['departments'] as List? ?? [])
            .whereType<Map>().map((item) => DirectoryDepartment.fromJson(Map<String, dynamic>.from(item))).toList(),
        total = (value['total'] as num?)?.toInt() ?? 0;
}

/// Every request is authorized by HRM's opaque session. Directory membership is
/// never inferred from a user ID or an SDK cache entry.
class HrmDirectory {
  static final instance = HrmDirectory();
  final Future<Map<String, dynamic>> Function(String, Map<String, dynamic>) _request;
  final LoginCertificate? Function() _session;
  HrmDirectory({
    Future<Map<String, dynamic>> Function(String, Map<String, dynamic>)? request,
    LoginCertificate? Function()? session,
  }) : _request = request ?? ((operation, body) => HrmSSO.request(operation, body)),
       _session = session ?? DataSp.getLoginCertificate;

  Future<Map<String, dynamic>> _authorized(String operation, Map<String, dynamic> body) async {
    final session = _session();
    if (session == null || session.sessionToken.isEmpty) {
      throw HrmSessionError(401, '请使用统一账号重新登录');
    }
    final result = await _request(operation, {...body, 'sessionToken': session.sessionToken});
    final current = _session();
    if (current?.userID != session.userID || current?.sessionToken != session.sessionToken) {
      throw HrmSessionError(409, '账号会话已改变，请重试');
    }
    return result;
  }

  Future<DirectoryPage> list({String keyword = '', String departmentID = '', int page = 1, int pageSize = 50}) async =>
      DirectoryPage.fromJson(await _authorized('directory', {
        'keyword': keyword.trim(), 'departmentID': departmentID,
        'page': page < 1 ? 1 : page, 'pageSize': pageSize.clamp(1, 100),
      }));

  Future<DirectoryPerson> person(String userID) async {
    final person = DirectoryPerson.fromJson(await _authorized('person', {'userID': userID}));
    if (person.userID != userID) throw HrmSessionError(503, '员工资料不匹配，请重试');
    return person;
  }
}
