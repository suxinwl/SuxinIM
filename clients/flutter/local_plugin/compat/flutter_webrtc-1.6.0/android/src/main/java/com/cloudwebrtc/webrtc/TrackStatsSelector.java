package com.cloudwebrtc.webrtc;

import java.lang.reflect.Array;
import java.util.ArrayDeque;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.Map;
import java.util.Set;
import java.util.function.Function;

/** Immutable report selection; never enumerates/disposes live sender wrappers. */
final class TrackStatsSelector {
  private static boolean referencesTrack(Map<String, Object> members, Set<String> identities) {
    return identities.contains(members.get("mediaSourceId")) || identities.contains(members.get("trackId"));
  }

  private static void addReferences(Object value, ArrayDeque<String> pending) {
    if (value instanceof String) pending.add((String) value);
    else if (value instanceof Iterable<?>) {
      for (Object item : (Iterable<?>) value) addReferences(item, pending);
    } else if (value != null && value.getClass().isArray()) {
      for (int i = 0; i < Array.getLength(value); i++) addReferences(Array.get(value, i), pending);
    }
  }

  static <T> Map<String, T> select(Map<String, T> reports, String trackID,
      Function<T, String> type, Function<T, Map<String, Object>> members) {
    Set<String> identities = new HashSet<>();
    for (Map.Entry<String, T> entry : reports.entrySet()) {
      if (trackID.equals(members.apply(entry.getValue()).get("trackIdentifier"))) {
        identities.add(entry.getKey());
      }
    }
    Set<String> outbound = new LinkedHashSet<>();
    Set<String> inbound = new LinkedHashSet<>();
    for (Map.Entry<String, T> entry : reports.entrySet()) {
      T node = entry.getValue();
      if (!identities.contains(entry.getKey()) && !referencesTrack(members.apply(node), identities)) continue;
      if ("outbound-rtp".equals(type.apply(node))) outbound.add(entry.getKey());
      else if ("inbound-rtp".equals(type.apply(node))) inbound.add(entry.getKey());
    }
    // Preserve the original sender-first selector when identifiers collide.
    ArrayDeque<String> pending = new ArrayDeque<>(outbound.isEmpty() ? inbound : outbound);
    Set<String> included = new HashSet<>();
    while (!pending.isEmpty()) {
      String id = pending.removeFirst();
      T node = reports.get(id);
      if (node == null || !included.add(id)) continue;
      for (Map.Entry<String, Object> member : members.apply(node).entrySet()) {
        String key = member.getKey();
        if (key.endsWith("Id") || key.endsWith("Ids")) addReferences(member.getValue(), pending);
      }
    }
    Map<String, T> selected = new LinkedHashMap<>();
    for (Map.Entry<String, T> entry : reports.entrySet()) {
      if (included.contains(entry.getKey())) selected.put(entry.getKey(), entry.getValue());
    }
    // No matching RTP root means an empty report, never all other tracks.
    return selected;
  }
}
