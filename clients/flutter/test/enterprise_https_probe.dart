import 'dart:convert';
import 'dart:io';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:openim_common/openim_common.dart';

// Standalone development-flavor diagnosis. No account, tokens, or trust bypass.
void main() => Config.init(() async {
      final results = <String, Object?>{'systemRootModified': false};
      try {
        results['origin'] = Config.serverBaseUrl;
        results['serverConfigType'] =
            DataSp.getServerConfig().runtimeType.toString();
        results['dioBaseConfigured'] = dio.options.baseUrl.isNotEmpty;
      } catch (error) {
        results['configurationErrorType'] = error.runtimeType.toString();
        print('PUTY_HTTPS_PROBE ' + jsonEncode(results));
        runApp(const MaterialApp(
            home: Scaffold(body: Text('Configuration failed'))));
        return;
      }
      final proxy =
          HttpClient.findProxyFromEnvironment(Uri.parse(Config.serverBaseUrl));
      results['proxyMode'] = proxy.startsWith('DIRECT') ? 'DIRECT' : 'PROXY';
      final client = HttpClient()
        ..connectionTimeout = const Duration(seconds: 15);
      try {
        final request = await client
            .getUrl(Uri.parse('${Config.serverBaseUrl}/sso/captcha'));
        final response =
            await request.close().timeout(const Duration(seconds: 20));
        final body =
            jsonDecode(await utf8.decoder.bind(response).join()) as Map;
        results.addAll({
          'passed': response.statusCode == 200 && body['code'] == 0,
          'httpStatus': response.statusCode,
          'code': body['code'],
          'show': (body['data'] as Map?)?['show'],
          'imageProvided':
              ((body['data'] as Map?)?['image'] as String? ?? '').isNotEmpty
        });
      } catch (error) {
        results.addAll({
          'passed': false,
          'errorType': error.runtimeType.toString(),
          'safePublicRequestError': error.toString()
        });
      } finally {
        client.close(force: true);
      }
      try {
        results['nativePKIX'] =
            await const MethodChannel('openim38/native_probe')
                .invokeMapMethod<String, dynamic>('enterpriseTlsProbe')
                .timeout(const Duration(seconds: 20));
      } catch (error) {
        results['nativePKIX'] = {
          'errorType': error.runtimeType.toString(),
          'safeAnonymousError': error.toString()
        };
      }
      try {
        final response = await dio
            .get<Map<String, dynamic>>('${Config.serverBaseUrl}/sso/captcha')
            .timeout(const Duration(seconds: 30));
        results['globalDio'] = {
          'httpStatus': response.statusCode,
          'code': response.data?['code'],
          'dataKeys': (response.data?['data'] as Map?)?.keys.toList()
        };
      } catch (error) {
        results['globalDio'] = {
          'errorType': error.runtimeType.toString(),
          if (error is DioException) 'dioType': error.type.name,
          if (error is DioException)
            'innerType': error.error.runtimeType.toString(),
          'safeAnonymousError': error.toString()
        };
      }
      try {
        final body = await HrmSSO.request('captcha')
            .timeout(const Duration(seconds: 30));
        results['hrmSSO'] = {
          'dataKeys': body.keys.toList(),
          'show': body['show'],
          'imageProvided': (body['image'] as String? ?? '').isNotEmpty
        };
      } catch (error) {
        results['hrmSSO'] = {
          'errorType': error.runtimeType.toString(),
          if (error is HrmSessionError) 'status': error.status
        };
      }
      // Only the public anonymous captcha transport is involved, never a session.
      // ignore: avoid_print
      print('PUTY_HTTPS_PROBE ' + jsonEncode(results));
      runApp(MaterialApp(
          home: Scaffold(body: SafeArea(child: Text(jsonEncode(results))))));
    });
