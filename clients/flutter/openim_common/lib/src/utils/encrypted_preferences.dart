import 'dart:async';
import 'dart:convert';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Synchronous reads from RAM, serialized authenticated native persistence.
/// Android Keystore/iOS Keychain own the key; no key crosses this channel.
class EncryptedPreferences {
  static const _channel = MethodChannel('puty/local_store');
  final Map<String, Object> _values;
  Future<void> _pending = Future.value();
  EncryptedPreferences._(this._values);
  static Future<EncryptedPreferences> open() async {
    final encoded = await _channel.invokeMethod<String>('preferences/read');
    final decoded = jsonDecode(encoded ?? '{}');
    if (decoded is! Map<String, dynamic>) {
      throw const FormatException('Invalid protected preferences');
    }
    final result = EncryptedPreferences._(Map<String, Object>.from(decoded));
    // Legacy migration is committed encrypted before the old plaintext store is
    // cleared. Never silently return empty preferences after a decryption error.
    final legacy = await SharedPreferences.getInstance();
    if (legacy.getKeys().isNotEmpty) {
      for (final key in legacy.getKeys()) {
        final value = legacy.get(key);
        if (value != null) result._values.putIfAbsent(key, () => value);
      }
      await result._persist();
      if (!await legacy.clear()) throw StateError('Legacy settings migration failed');
    }
    return result;
  }
  Future<bool> _persist() {
    final snapshot = jsonEncode(_values);
    final operation = _pending.then((_) async {
      if (await _channel.invokeMethod<bool>('preferences/write', snapshot) != true) {
        throw StateError('Protected settings commit failed');
      }
    });
    // A failed commit must not poison the queue and silently skip later writes.
    _pending = operation.catchError((Object _) {});
    return operation.then((_) => true);
  }
  Object? get(String key) => _values[key];
  String? getString(String key) => _values[key] as String?;
  bool? getBool(String key) => _values[key] as bool?;
  int? getInt(String key) => (_values[key] as num?)?.toInt();
  double? getDouble(String key) => (_values[key] as num?)?.toDouble();
  List<String>? getStringList(String key) => (_values[key] as List?)?.cast<String>();
  Set<String> getKeys() => _values.keys.toSet();
  bool containsKey(String key) => _values.containsKey(key);
  Future<bool> _set(String key, Object value) { _values[key] = value; return _persist(); }
  Future<bool> setString(String key,String value) => _set(key,value);
  Future<bool> setBool(String key,bool value) => _set(key,value);
  Future<bool> setInt(String key,int value) => _set(key,value);
  Future<bool> setDouble(String key,double value) => _set(key,value);
  Future<bool> setStringList(String key,List<String> value) => _set(key,List<String>.from(value));
  Future<bool> remove(String key) { _values.remove(key); return _persist(); }
  Future<bool> clear() { _values.clear(); return _persist(); }
  Future<void> reload() async { await _pending; }
}
