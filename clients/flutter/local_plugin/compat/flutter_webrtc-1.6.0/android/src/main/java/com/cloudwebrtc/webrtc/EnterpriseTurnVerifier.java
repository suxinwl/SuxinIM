package com.cloudwebrtc.webrtc;
import android.content.Context;
import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.security.cert.*;
import java.util.*;
import org.webrtc.PeerConnection;
import org.webrtc.SSLCertificateVerifier;

/** Trust only the embedded enterprise root for the explicitly configured LAN
 * TURN host. Public TURN keeps WebRTC's normal verifier and public chain. */
final class EnterpriseTurnVerifier implements SSLCertificateVerifier {
  static final String LAN_HOST="192.168.10.11";
  private final X509Certificate root;
  EnterpriseTurnVerifier(X509Certificate root) { this.root=root; }
  static SSLCertificateVerifier forConfiguration(Context context, PeerConnection.RTCConfiguration config) {
    boolean privateTurn=false;
    for (PeerConnection.IceServer server:config.iceServers) for(String url:server.urls) {
      if(url.matches("^turns:192\\.168\\.10\\.11(?::[0-9]+)?(?:\\?.*)?$")) privateTurn=true;
    }
    if(!privateTurn) return null;
    try {
      int id=context.getResources().getIdentifier("puty_enterprise_root_ca","raw",context.getPackageName());
      try(InputStream in=context.getResources().openRawResource(id)) {
        return new EnterpriseTurnVerifier((X509Certificate)CertificateFactory.getInstance("X.509").generateCertificate(in));
      }
    } catch(Exception error) { throw new IllegalStateException("Enterprise TURN root unavailable",error); }
  }
  @Override public boolean verify(byte[] der) {
    try {
      CertificateFactory factory=CertificateFactory.getInstance("X.509");
      X509Certificate leaf=(X509Certificate)factory.generateCertificate(new ByteArrayInputStream(der));
      leaf.checkValidity(); root.checkValidity();
      if(leaf.getBasicConstraints()>=0) return false;
      List<String> usage=leaf.getExtendedKeyUsage();
      if(usage==null || !usage.contains("1.3.6.1.5.5.7.3.1")) return false;
      boolean host=false;
      Collection<List<?>> names=leaf.getSubjectAlternativeNames();
      if(names!=null) for(List<?> name:names) if(Integer.valueOf(7).equals(name.get(0)) && LAN_HOST.equals(name.get(1))) host=true;
      if(!host) return false;
      PKIXParameters params=new PKIXParameters(Collections.singleton(new TrustAnchor(root,null)));
      params.setRevocationEnabled(false);
      CertPath path=factory.generateCertPath(Collections.singletonList(leaf));
      CertPathValidator.getInstance("PKIX").validate(path,params);
      return true;
    } catch(Exception error) { return false; }
  }
}
