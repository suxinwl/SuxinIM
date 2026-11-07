import 'package:app_im/app_im.dart';
import 'dart:io';
import 'dart:convert';
import 'dart:ui';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:image_gallery_saver/image_gallery_saver.dart';
import 'package:openim_common/openim_common.dart';
import 'package:uuid/uuid.dart';

var dio = Dio();

class HttpUtil {
  HttpUtil._();

  static void init() {
    AppIm.enterprise.configure(
        baseUrl: () => Config.enterpriseUrl,
        imToken: () => DataSp.imToken,
        chatToken: () => DataSp.chatToken,
        platform: () => IMUtils.getPlatform(),
        ready: () => AppIm.iMManager.isLogined,
        userID: () => DataSp.userID);
    // add interceptors
    dio
      // ..interceptors.add(PrettyDioLogger(
      //   requestHeader: kDebugMode,
      //   requestBody: kDebugMode,
      //   responseBody: kDebugMode,
      //   responseHeader: kDebugMode,
      // ))
      ..interceptors.add(InterceptorsWrapper(onRequest: (options, handler) {
        // Do something before request is sent
        return handler.next(options); //continue
        // 如果你想完成请求并返回一些自定义数据，你可以resolve一个Response对象 `handler.resolve(response)`。
        // 这样请求将会被终止，上层then会被调用，then中返回的数据将是你的自定义response.
        //
        // 如果你想终止请求并触发一个错误,你可以返回一个`DioError`对象,如`handler.reject(error)`，
        // 这样请求将被中止并触发异常，上层catchError会被调用。
      }, onResponse: (response, handler) {
        // Do something with response data
        return handler.next(response); // continue
        // 如果你想终止请求并触发一个错误,你可以 reject 一个`DioError`对象,如`handler.reject(error)`，
        // 这样请求将被中止并触发异常，上层catchError会被调用。
      }, onError: (DioError e, handler) {
        // Do something with response error
        return handler.next(e); //continue
        // 如果你想完成请求并返回一些自定义数据，可以resolve 一个`Response`,如`handler.resolve(response)`。
        // 这样请求将会被终止，上层then会被调用，then中返回的数据将是你的自定义response.
      }));

    // 配置dio实例
    dio.options.baseUrl = Config.imApiUrl;
    dio.options.connectTimeout = const Duration(seconds: 30); //30s
    dio.options.receiveTimeout = const Duration(seconds: 30);
  }

  static String get operationID =>
      DateTime.now().millisecondsSinceEpoch.toString();

  ///
  static Future post(
    String path, {
    dynamic data,
    bool showErrorToast = true,
    Map<String, dynamic>? queryParameters,
    Options? options,
    CancelToken? cancelToken,
    ProgressCallback? onSendProgress,
    ProgressCallback? onReceiveProgress,
  }) async {
    try {
      data ??= {};
      options ??= Options();
      options.headers ??= {};
      options.headers!['operationID'] = operationID;
      final parsed = Uri.tryParse(path);
      if (parsed != null &&
          (parsed.path.contains('/office/') || parsed.path.contains('/rtc/'))) {
        final at = parsed.path.indexOf(RegExp('/(?:office|rtc)/'));
        path = '${Config.enterpriseUrl}${parsed.path.substring(at)}';
        options.headers!.addAll({
          'token': DataSp.imToken,
          'chatToken': DataSp.chatToken,
          'platformID': IMUtils.getPlatform()
        });
      }

      var result = await dio.post<Map<String, dynamic>>(
        path,
        data: data,
        queryParameters: queryParameters,
        options: options,
        cancelToken: cancelToken,
        onSendProgress: onSendProgress,
        onReceiveProgress: onReceiveProgress,
      );
      var resp = ApiResp.fromJson(result.data!);
      if (resp.errCode == 0) {
        return resp.data;
      } else {
        if (showErrorToast) {
          IMViews.showToast(resp.errDlt);
          // IMViews.showToast(ApiError.getMsg(resp.errCode));
        }

        return Future.error(resp.errMsg);
      }
    } catch (error) {
      if (error is DioException) {
        final errorMsg = '接口：$path  信息：${error.message}';
        if (showErrorToast) IMViews.showToast(errorMsg);
        return Future.error(errorMsg);
      }
      final errorMsg = '接口：$path  信息：${error.toString()}';
      if (showErrorToast) IMViews.showToast(errorMsg);
      return Future.error(error);
    }
  }

  /// fileType: file = "1",video = "2",picture = "3"
  static Future<String> uploadImageForMinio({
    required String path,
    bool compress = true,
  }) async {
    String? compressPath;
    if (compress) {
      File? compressFile = await IMUtils.compressImageAndGetFile(File(path));
      compressPath = compressFile?.path;
      Logger.print('compressPath: $compressPath');
    }
    final source = File(compressPath ?? path);
    final uploaded = await AppIm.iMManager.uploadFile(
      id: const Uuid().v4(),
      filePath: source.path,
      fileName: source.uri.pathSegments.last,
    );
    final result = uploaded is String ? jsonDecode(uploaded) : uploaded;
    if (result is! Map ||
        result['url'] is! String ||
        Uri.tryParse(result['url'])?.scheme != 'https') {
      throw StateError('File upload did not return a valid HTTPS URL');
    }
    return result['url'] as String;
  }

  static Future download(
    String url, {
    required String cachePath,
    CancelToken? cancelToken,
    Function(int count, int total)? onProgress,
  }) async {
    final sourceName=cachePath.replaceAll('\\','/').split('/').last;
    return NativeLocalVault.save(url,sourceName);
  }

  /// Public release packages must remain downloadable before account login.
  /// Use an isolated HTTPS transport so session headers and attachment grants
  /// cannot be mixed into the signed-package update flow.
  static Future downloadPublicInstaller(
    String url, {
    required String cachePath,
    Function(int count, int total)? onProgress,
    Dio? client,
  }) async {
    final uri = Uri.tryParse(url);
    if (uri == null || uri.scheme != 'https' || uri.userInfo.isNotEmpty ||
        ![Config.lanOrigin, Config.wanOrigin].contains(uri.origin) ||
        uri.pathSegments.length != 2 || uri.pathSegments.first != 'downloads' ||
        !RegExp(r'^[A-Za-z0-9][A-Za-z0-9._-]*\.apk$').hasMatch(uri.pathSegments.last)) {
      throw StateError('安装包地址必须是受信服务器的公开 HTTPS 下载地址');
    }
    final transport = client ?? Dio(BaseOptions(
      connectTimeout: const Duration(seconds: 30),
      receiveTimeout: const Duration(minutes: 10),
    ));
    try {
      return await transport.download(url, cachePath,
        options: Options(followRedirects: false),
        onReceiveProgress: onProgress);
    } finally {
      if (client == null) transport.close();
    }
  }

  static Future saveUrlPicture(
    String url, {
    CancelToken? cancelToken,
    Function(int count, int total)? onProgress,
  }) async {
    final result=await NativeLocalVault.save(url,Uri.parse(url).pathSegments.last);
    IMViews.showToast('已加密保存：${result['name']}');return result;
  }

  static Future saveImage(Image image) async {
    throw StateError('请从消息的加密保存入口保存原附件');
  }

  static Future saveUrlVideo(
    String url, {
    CancelToken? cancelToken,
    Function(int count, int total)? onProgress,
  }) async {
    final result=await NativeLocalVault.save(url,Uri.parse(url).pathSegments.last);
    IMViews.showToast('已加密保存：${result['name']}');return result;
  }

  static void saveFileToGallerySaver(File file) async {
    IMViews.showToast('请从消息的加密保存入口保存原附件');
  }
}
