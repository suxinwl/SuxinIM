package io.openim.flutter_openim_sdk;

import java.math.BigInteger;
import java.security.GeneralSecurityException;
import java.util.function.Consumer;
import org.json.JSONObject;

public final class NativeVaultSessionStateHarness {
  private static final long NOW = 1793730000000L;
  private static final String ACCOUNT = "emp_0123456789abcdef0123456789abcdef";
  private static int checks;

  private static JSONObject valid() {
    JSONObject sources = new JSONObject().put("login", "roles").put("save", "person");
    JSONObject policy = new JSONObject().put("zone", "lan").put("logicalClient", "android")
        .put("login", true).put("preview", true).put("save", true)
        .put("securityVersion", 3L).put("policyRevision", 0).put("networkRevision", 2L)
        .put("currentIP", "192.168.10.10").put("sources", sources);
    return new JSONObject().put("userID", ACCOUNT).put("sessionID", "synthetic-session")
        .put("epoch", 3L).put("platform", 2).put("logicalClient", "android")
        .put("expiresAt", NOW / 1000 + 60).put("loginAllowed", true).put("saveAllowed", true)
        .put("effectivePolicy", policy);
  }

  private static JSONObject policy(JSONObject value) { return (JSONObject) value.opt("effectivePolicy"); }
  private static void pass(boolean value) { if (!value) throw new AssertionError("Contract assertion failed"); checks++; }
  private static void reject(Consumer<JSONObject> mutation) throws Exception {
    JSONObject value = valid(); mutation.accept(value);
    try { NativeVaultSessionState.validate(value, ACCOUNT, NOW); }
    catch (GeneralSecurityException expected) {
      pass("Invalid native session state".equals(expected.getMessage())); return;
    }
    throw new AssertionError("Malformed state accepted");
  }

  public static void main(String[] args) throws Exception {
    NativeVaultSessionState.Result result = NativeVaultSessionState.validate(valid(), ACCOUNT, NOW);
    pass(result.epoch == 3 && result.expiresAt == NOW + 60000 && result.saveAllowed
        && result.previewAllowed && result.sessionID.equals("synthetic-session"));
    JSONObject milliseconds = valid().put("expiresAt", NOW + 12345);
    pass(NativeVaultSessionState.validate(milliseconds, ACCOUNT, NOW).expiresAt == NOW + 12345);
    JSONObject internet = valid().put("saveAllowed", false);
    policy(internet).put("zone", "internet").put("save", false);
    pass(!NativeVaultSessionState.validate(internet, ACCOUNT, NOW).saveAllowed);
    for (String ip : new String[]{"::1", "::", "2001:db8::1", "::ffff:192.168.10.1",
        "2001:db8:0000:0000:0000:0000:0000:1", "1:2:3:4:5:6:192.168.0.1"}) {
      JSONObject ipv6 = valid(); policy(ipv6).put("currentIP", ip);
      pass(NativeVaultSessionState.validate(ipv6, ACCOUNT, NOW).epoch == 3);
    }
    JSONObject safe = valid().put("epoch", 9007199254740991L).put("expiresAt", 9007199254740991L);
    policy(safe).put("securityVersion", 9007199254740991L);
    pass(NativeVaultSessionState.validate(safe, ACCOUNT, NOW).epoch == 9007199254740991L);

    for (String key : new String[]{"userID", "sessionID", "epoch", "platform", "logicalClient", "expiresAt", "loginAllowed", "saveAllowed", "effectivePolicy"})
      reject(v -> v.remove(key));
    for (String key : new String[]{"zone", "logicalClient", "login", "preview", "save", "securityVersion", "policyRevision", "networkRevision", "currentIP", "sources"})
      reject(v -> policy(v).remove(key));
    for (String key : new String[]{"login", "save"})
      reject(v -> ((JSONObject) policy(v).opt("sources")).remove(key));
    for (String key : new String[]{"loginAllowed", "saveAllowed"})
      for (Object invalid : new Object[]{"true", 1, 1L, 1.0, null}) reject(v -> v.put(key, invalid));
    for (String key : new String[]{"login", "preview", "save"})
      for (Object invalid : new Object[]{"true", 1, 1L, 1.0, null}) reject(v -> policy(v).put(key, invalid));
    for (String key : new String[]{"epoch", "platform", "expiresAt"})
      for (Object invalid : new Object[]{"3", 3.0, 3.0f, BigInteger.valueOf(3), true, -1, 9007199254740992L, Long.MAX_VALUE, null})
        reject(v -> v.put(key, invalid));
    for (String key : new String[]{"securityVersion", "policyRevision", "networkRevision"})
      for (Object invalid : new Object[]{"3", 3.0, 3.0f, BigInteger.valueOf(3), true, -1, 9007199254740992L, Long.MAX_VALUE, null})
        reject(v -> policy(v).put(key, invalid));
    reject(v -> v.put("epoch", 0));
    reject(v -> v.put("platform", 1));
    reject(v -> v.put("platform", 3));
    reject(v -> v.put("userID", "different-account"));
    reject(v -> v.put("logicalClient", "web_mobile"));
    reject(v -> v.put("loginAllowed", false));
    reject(v -> v.put("sessionID", ""));
    reject(v -> v.put("sessionID", "synthetic\nsecret"));
    reject(v -> v.put("sessionID", "x".repeat(513)));
    reject(v -> v.put("expiresAt", 0));
    reject(v -> v.put("expiresAt", NOW));
    reject(v -> v.put("expiresAt", NOW / 1000));
    reject(v -> v.put("expiresAt", 1L));
    reject(v -> policy(v).put("securityVersion", 4));
    reject(v -> policy(v).put("zone", "LAN"));
    reject(v -> policy(v).put("zone", ""));
    reject(v -> policy(v).put("logicalClient", "ios"));
    reject(v -> policy(v).put("login", false));
    reject(v -> policy(v).put("preview", false));
    reject(v -> policy(v).put("save", false));
    reject(v -> policy(v).put("currentIP", ""));
    reject(v -> policy(v).put("currentIP", " 192.168.10.10"));
    reject(v -> policy(v).put("currentIP", true));
    for (String badIP : new String[]{"host.example", "192.168.1.1:9885", "[::1]", "[::1]:9885",
        "fe80::1%wlan0", "999.1.2.3", "01.2.3.4", "1.2.3", ":1", "1:", ":::1",
        "1::2::3", "1:2:3:4:5:6:7:8:9", "1:2:3:4:5:6:7::8", "2001:xyz::1", "1.2.3.4::"})
      reject(v -> policy(v).put("currentIP", badIP));
    reject(v -> policy(v).put("sources", "roles"));
    reject(v -> ((JSONObject) policy(v).opt("sources")).put("login", ""));
    reject(v -> ((JSONObject) policy(v).opt("sources")).put("save", true));
    for (long invalidNow : new long[]{0, -1, 9007199254740992L}) {
      try { NativeVaultSessionState.validate(valid(), ACCOUNT, invalidNow); throw new AssertionError("Invalid clock accepted"); }
      catch (GeneralSecurityException expected) { pass(true); }
    }
    try { NativeVaultSessionState.validate(valid(), "", NOW); throw new AssertionError("Empty account accepted"); }
    catch (GeneralSecurityException expected) { pass(true); }
    try { NativeVaultSessionState.validate(null, ACCOUNT, NOW); throw new AssertionError("Null state accepted"); }
    catch (GeneralSecurityException expected) { pass(true); }
    System.out.println("{\"passed\":true,\"checks\":" + checks + ",\"networkRequests\":0,\"productionState\":false}");
  }
}
