package cn.rentsoft.flutter.openim.consumer;

import android.content.Context;
import java.io.InputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.security.cert.*;
import java.util.*;
import javax.net.ssl.*;

/** Development-only proof: normal native TLS and explicit pinned-root PKIX. */
public final class EnterpriseProbeVerifier {
  private static String hash(byte[] bytes)throws Exception{StringBuilder out=new StringBuilder();for(byte b:MessageDigest.getInstance("SHA-256").digest(bytes))out.append(String.format(Locale.ROOT,"%02x",b));return out.toString();}
  public static Map<String,Object> probe(Context context)throws Exception{
    final String host="192.168.10.11";final int port=9885;
    CertificateFactory factory=CertificateFactory.getInstance("X.509");X509Certificate root;
    int id=context.getResources().getIdentifier("puty_enterprise_root_ca","raw",context.getPackageName());
    try(InputStream in=context.getResources().openRawResource(id)){root=(X509Certificate)factory.generateCertificate(in);}
    if(!hash(root.getEncoded()).equals("836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69"))throw new CertificateException("Unexpected enterprise root");
    root.checkValidity();if(root.getBasicConstraints()!=0)throw new CertificateException("Expected root pathLen zero");root.verify(root.getPublicKey());
    KeyStore store=KeyStore.getInstance(KeyStore.getDefaultType());store.load(null);store.setCertificateEntry("puty",root);
    TrustManagerFactory managers=TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());managers.init(store);
    SSLContext tls=SSLContext.getInstance("TLS");tls.init(null,managers.getTrustManagers(),null);
    X509Certificate leaf;
    try(Socket transport=new Socket()){
      transport.connect(new InetSocketAddress(host,port),7000);transport.setSoTimeout(7000);
      try(SSLSocket socket=(SSLSocket)tls.getSocketFactory().createSocket(transport,host,port,true)){
        SSLParameters parameters=socket.getSSLParameters();parameters.setEndpointIdentificationAlgorithm("HTTPS");socket.setSSLParameters(parameters);socket.startHandshake();
        java.security.cert.Certificate[] chain=socket.getSession().getPeerCertificates();
        if(chain.length<1 || chain.length>2 || (chain.length==2 && !chain[1].equals(root)))throw new CertificateException("Unexpected chain shape");
        leaf=(X509Certificate)chain[0];leaf.checkValidity();if(leaf.getBasicConstraints()>=0)throw new CertificateException("Server is a CA");
        List<String> eku=leaf.getExtendedKeyUsage();if(eku==null || !eku.contains("1.3.6.1.5.5.7.3.1"))throw new CertificateException("Missing serverAuth");
        boolean exact=false;Collection<List<?>> names=leaf.getSubjectAlternativeNames();if(names!=null)for(List<?> name:names){if(!Integer.valueOf(7).equals(name.get(0)) || !host.equals(name.get(1)))throw new CertificateException("Unexpected server SAN");exact=true;}
        if(!exact)throw new CertificateException("Missing exact LAN IP");
        PKIXParameters policy=new PKIXParameters(Collections.singleton(new TrustAnchor(root,null)));policy.setRevocationEnabled(false);CertPathValidator.getInstance("PKIX").validate(factory.generateCertPath(Collections.singletonList(leaf)),policy);
      }
    }
    Map<String,Object> result=new HashMap<>();result.put("verified",true);result.put("host",host);result.put("port",port);result.put("rootSHA256",hash(root.getEncoded()));result.put("leafSHA256",hash(leaf.getEncoded()));result.put("validUntil",Math.min(root.getNotAfter().getTime(),leaf.getNotAfter().getTime()));return result;
  }
}

