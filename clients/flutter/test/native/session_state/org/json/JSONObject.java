package org.json;

import java.util.HashMap;
import java.util.Map;

/** Test-only value container. Production source is separately compiled against android.jar. */
public final class JSONObject {
  private final Map<String, Object> values = new HashMap<>();
  public JSONObject put(String key, Object value) { values.put(key, value); return this; }
  public Object opt(String key) { return values.get(key); }
  public Object remove(String key) { return values.remove(key); }
}
