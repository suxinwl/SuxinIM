package com.cloudwebrtc.webrtc.utils;
import java.util.HashMap;
import java.util.Map;
public final class ConstraintsMap {
  private final Map<String,Object> values = new HashMap<>();
  public void putString(String key, String value) { values.put(key, value); }
  public void putInt(String key, int value) { values.put(key, value); }
  public void putDouble(String key, double value) { values.put(key, value); }
  public Map<String,Object> toMap() { return values; }
}
