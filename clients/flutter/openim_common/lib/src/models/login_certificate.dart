import 'dart:convert';
class LoginCertificate {
  String userID, imToken, chatToken, sessionToken, attachmentToken;
  int expiresAt;
  Map<String,dynamic> profile;
  String policyClient;
  Map<String,dynamic> effectivePolicy;
  LoginCertificate.fromJson(Map<String,dynamic> map)
      : userID=map['userID'] ?? '',imToken=map['imToken'] ?? '',chatToken=map['chatToken'] ?? '',
        sessionToken=map['sessionToken'] ?? '',attachmentToken=map['attachmentToken'] ?? '',
        expiresAt=(map['expiresAt'] as num?)?.toInt() ?? 0,
        profile=Map<String,dynamic>.from(map['profile'] is Map ? map['profile'] : {}),
        policyClient=map['policyClient'] is String?map['policyClient']:'',
        effectivePolicy=Map<String,dynamic>.from(map['effectivePolicy'] is Map?map['effectivePolicy']:{});
  Map<String,dynamic> toJson()=>{'userID':userID,'imToken':imToken,'chatToken':chatToken,'sessionToken':sessionToken,
    'expiresAt':expiresAt,'attachmentToken':attachmentToken,'profile':profile,'policyClient':policyClient,'effectivePolicy':effectivePolicy};
  @override String toString()=>jsonEncode({'userID':userID,'expiresAt':expiresAt});
}
