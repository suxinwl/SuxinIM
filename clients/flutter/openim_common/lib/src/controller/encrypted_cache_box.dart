import 'package:openim_common/openim_common.dart';

/// Compatibility adapter for the two old Hive caches. Only encoded JSON is
/// persisted through native OS-protected settings; model objects stay in RAM.
class EncryptedCacheBox<T> {
  final String name;
  final Map<String,dynamic> Function(T) encode;
  final T Function(Map<String,dynamic>) decode;
  EncryptedCacheBox(this.name,{required this.encode,required this.decode});
  List<T> get(String key,{List<T>? defaultValue}) {
    final value=SpUtil().getObject('protected-box:$name:$key');
    if(value==null)return defaultValue??<T>[];
    return (value['items'] as List? ?? []).map((item)=>decode(Map<String,dynamic>.from(item as Map))).toList();
  }
  Future<void> put(String key,List<T> values) async {
    if(await SpUtil().putObject('protected-box:$name:$key',{'items':values.map(encode).toList()})!=true)throw StateError('Protected cache commit failed');
  }
}
