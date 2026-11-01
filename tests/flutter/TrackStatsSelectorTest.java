package com.cloudwebrtc.webrtc;

import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;

/** Disjoint tracks with shared transport, RID layers, remote RTP and cycles. */
public final class TrackStatsSelectorTest {
  static final class Node {
    final String type;
    final Map<String,Object> members = new LinkedHashMap<>();
    Node(String type, Object... values) {
      this.type=type;
      for(int i=0;i<values.length;i+=2) members.put((String)values[i],values[i+1]);
    }
  }
  static void check(boolean valid,String message) { if(!valid)throw new AssertionError(message); }
  static Set<String> select(Map<String,Node> nodes,String track) {
    return TrackStatsSelector.select(nodes,track,n->n.type,n->n.members).keySet();
  }
  public static void main(String[] args) {
    Map<String,Node> nodes=new LinkedHashMap<>();
    nodes.put("videoSource",new Node("media-source","trackIdentifier","camera"));
    nodes.put("audioSource",new Node("media-source","trackIdentifier","microphone"));
    nodes.put("otherSource",new Node("media-source","trackIdentifier","otherCamera"));
    nodes.put("videoLow",new Node("outbound-rtp","mediaSourceId","videoSource","codecId","videoCodec","transportId","transport","remoteId","videoRemote"));
    nodes.put("videoHigh",new Node("outbound-rtp","mediaSourceId","videoSource","codecId","videoCodec","transportId","transport"));
    nodes.put("videoRemote",new Node("remote-inbound-rtp","localId","videoLow"));
    nodes.put("audio",new Node("outbound-rtp","mediaSourceId","audioSource","transportId","transport"));
    nodes.put("otherVideo",new Node("outbound-rtp","mediaSourceId","otherSource","transportId","transport"));
    nodes.put("inbound",new Node("inbound-rtp","trackIdentifier","remoteCamera","transportId","transport","remoteId","remoteOutbound"));
    nodes.put("remoteOutbound",new Node("remote-outbound-rtp","localId","inbound"));
    nodes.put("collision",new Node("inbound-rtp","trackIdentifier","camera","transportId","transport"));
    nodes.put("videoCodec",new Node("codec","mimeType","video/VP9"));
    nodes.put("transport",new Node("transport","selectedCandidatePairId","pair","localCertificateId","certificate"));
    nodes.put("pair",new Node("candidate-pair","localCandidateId","local","remoteCandidateId","remote"));
    nodes.put("local",new Node("local-candidate"));nodes.put("remote",new Node("remote-candidate"));
    nodes.put("certificate",new Node("certificate","issuerCertificateId","certificate"));
    nodes.put("unrelated",new Node("data-channel"));
    Set<String> camera=select(nodes,"camera");
    check(camera.containsAll(Arrays.asList("videoLow","videoHigh","videoSource","videoRemote","videoCodec","transport","pair","local","remote","certificate")),"lost RID or referenced stats");
    check(!camera.contains("audio")&&!camera.contains("otherVideo")&&!camera.contains("inbound")&&!camera.contains("collision")&&!camera.contains("unrelated"),"selector mixed tracks through shared transport");
    Set<String> inbound=select(nodes,"remoteCamera");
    check(inbound.contains("inbound")&&inbound.contains("remoteOutbound")&&!inbound.contains("videoLow"),"inbound selector failed");
    check(select(nodes,"missing").isEmpty(),"unknown track leaked full report");
    nodes.put("legacyTrack",new Node("track","trackIdentifier","legacy"));
    nodes.put("legacyRtp",new Node("outbound-rtp","trackId","legacyTrack"));
    check(select(nodes,"legacy").equals(Set.of("legacyTrack","legacyRtp")),"legacy track reference failed");
    nodes.put("multi",new Node("outbound-rtp","trackIdentifier","multi","associatedIds",new String[]{"videoCodec","local"}));
    check(select(nodes,"multi").containsAll(Set.of("multi","videoCodec","local")),"Id array dependency failed");
    check(nodes.size()==21&&nodes.get("videoSource").members.get("trackIdentifier").equals("camera"),"report mutated");
    System.out.println("PASS: sender-first, simulcast RID, local/remote RTP, codec/transport/candidate/certificate dependency closure, cycle, track isolation, unknown empty, legacy, Id arrays, immutable input");
  }
}
