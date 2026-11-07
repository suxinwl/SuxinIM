package io.openim.flutter_openim_sdk;

import java.security.GeneralSecurityException;
import org.json.JSONObject;

/** Strict authority response validation. This class neither issues nor caches a lease. */
public final class NativeVaultSessionState {
  private static final long MAX_SAFE_INTEGER = 9007199254740991L;
  private static final long SECONDS_CUTOFF = 100000000000L;

  private NativeVaultSessionState() {}

  public static final class Result {
    public final String sessionID;
    public final long epoch;
    /** Absolute expiry in milliseconds, including when the wire value was seconds. */
    public final long expiresAt;
    public final boolean saveAllowed;
    public final boolean previewAllowed;

    private Result(String sessionID, long epoch, long expiresAt, boolean saveAllowed) {
      this.sessionID = sessionID;
      this.epoch = epoch;
      this.expiresAt = expiresAt;
      this.saveAllowed = saveAllowed;
      this.previewAllowed = true;
    }
  }

  public static Result validate(JSONObject state, String expectedAccount, long nowMillis)
      throws GeneralSecurityException {
    if (state == null || !validText(expectedAccount, 512) || nowMillis <= 0
        || nowMillis > MAX_SAFE_INTEGER) throw rejected();
    if (!expectedAccount.equals(text(state, "userID", 512))
        || integer(state, "platform") != 2
        || !"android".equals(text(state, "logicalClient", 32))
        || !bool(state, "loginAllowed")) throw rejected();
    String sessionID = text(state, "sessionID", 512);
    long epoch = integer(state, "epoch");
    if (epoch < 1) throw rejected();
    boolean save = bool(state, "saveAllowed");
    long expiry = integer(state, "expiresAt");
    if (expiry <= 0) throw rejected();
    if (expiry < SECONDS_CUTOFF) {
      if (expiry > MAX_SAFE_INTEGER / 1000L) throw rejected();
      expiry *= 1000L;
    }
    if (expiry <= nowMillis || expiry > MAX_SAFE_INTEGER) throw rejected();

    JSONObject policy = object(state, "effectivePolicy");
    String zone = text(policy, "zone", 16);
    if ((!"lan".equals(zone) && !"internet".equals(zone))
        || !"android".equals(text(policy, "logicalClient", 32))
        || !bool(policy, "login") || !bool(policy, "preview")
        || bool(policy, "save") != save
        || integer(policy, "securityVersion") != epoch) throw rejected();
    integer(policy, "policyRevision");
    integer(policy, "networkRevision");
    if (!bareIP(text(policy, "currentIP", 128))) throw rejected();
    JSONObject sources = object(policy, "sources");
    text(sources, "login", 64);
    text(sources, "save", 64);
    return new Result(sessionID, epoch, expiry, save);
  }

  private static JSONObject object(JSONObject value, String key) throws GeneralSecurityException {
    Object raw = value.opt(key);
    if (!(raw instanceof JSONObject)) throw rejected();
    return (JSONObject) raw;
  }

  private static boolean bool(JSONObject value, String key) throws GeneralSecurityException {
    Object raw = value.opt(key);
    if (!(raw instanceof Boolean)) throw rejected();
    return (Boolean) raw;
  }

  private static long integer(JSONObject value, String key) throws GeneralSecurityException {
    Object raw = value.opt(key);
    // JSONObject coercive getters would also accept strings and floating-point values.
    if (!(raw instanceof Integer) && !(raw instanceof Long)) throw rejected();
    long number = ((Number) raw).longValue();
    if (number < 0 || number > MAX_SAFE_INTEGER) throw rejected();
    return number;
  }

  private static String text(JSONObject value, String key, int maximum)
      throws GeneralSecurityException {
    Object raw = value.opt(key);
    if (!(raw instanceof String) || !validText((String) raw, maximum)) throw rejected();
    return (String) raw;
  }

  private static boolean validText(String value, int maximum) {
    if (value == null || value.isEmpty() || value.length() > maximum || !value.equals(value.trim()))
      return false;
    for (int i = 0; i < value.length(); i++) {
      char c = value.charAt(i);
      if (Character.isISOControl(c) || Character.isSurrogate(c)) return false;
    }
    return true;
  }

  // Parse literals locally: never perform DNS, accept ports, zones or bracketed URLs.
  private static boolean bareIP(String address) {
    if (address.indexOf(':') < 0) return ipv4(address);
    String value = address;
    if (value.indexOf('.') >= 0) {
      int last = value.lastIndexOf(':');
      if (last < 0 || !ipv4(value.substring(last + 1))) return false;
      value = value.substring(0, last + 1) + "0:0";
    }
    String[] halves = value.split("::", -1);
    if (halves.length > 2) return false;
    int groups = 0;
    for (String half : halves) {
      if (half.isEmpty()) continue;
      String[] parts = half.split(":", -1);
      for (String part : parts) {
        if (part.isEmpty() || part.length() > 4) return false;
        for (int i = 0; i < part.length(); i++) {
          char c = part.charAt(i);
          if (!((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
              || (c >= 'A' && c <= 'F'))) return false;
        }
        groups++;
      }
    }
    return halves.length == 2 ? groups < 8 : groups == 8;
  }

  private static boolean ipv4(String address) {
    String[] parts = address.split("\\.", -1);
    if (parts.length != 4) return false;
    for (String part : parts) {
      if (part.isEmpty() || part.length() > 3 || (part.length() > 1 && part.charAt(0) == '0'))
        return false;
      int number = 0;
      for (int i = 0; i < part.length(); i++) {
        char c = part.charAt(i);
        if (c < '0' || c > '9') return false;
        number = number * 10 + c - '0';
      }
      if (number > 255) return false;
    }
    return true;
  }

  private static GeneralSecurityException rejected() {
    return new GeneralSecurityException("Invalid native session state");
  }
}
