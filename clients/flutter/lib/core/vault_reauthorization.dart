import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:openim_common/openim_common.dart' show HrmSessionError;

/// A local lease/transport failure closes archives without proving logout.
/// Only a positive denial from an online authority clears session credentials.
bool isDefiniteSessionDenial(Object error) =>
    error is HrmSessionError && const {401, 403, 428}.contains(error.status) ||
    error is PlatformException && error.code == 'LOCAL_VAULT_ACCESS_DENIED';

bool isTemporarySessionFailure(Object error) =>
    error is TimeoutException ||
    error is IOException ||
    error is HrmSessionError && !const {401, 403, 428}.contains(error.status) ||
    error is PlatformException &&
        error.code == 'LOCAL_VAULT_REAUTHORIZATION_REQUIRED';
