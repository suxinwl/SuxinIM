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

/** Native PKIX authority for one enterprise endpoint. No HTTP request, redirects,
 * intermediate CAs, caller-supplied target, or trust outside the pinned root. */
public final class EnterpriseLanTls {
  public static final String HOST="192.168.10.11";
  public static final int PORT=9885;
  public static final String ROOT_SHA256="836c3d6c28d5078cc8da6ebeff8e5f9a7fb7f140554e6a7834011d72299c3d69";
  static String sha256(byte[] bytes)throws Exception {
    StringBuilder out=new StringBuilder();for(byte b:MessageDigest.getInstance("SHA-256").digest(bytes))out.append(String.format(Locale.ROOT,"%02x",b));return out.toString();
  }
  static X509Certificate validateChain(java.security.cert.Certificate[] chain,X509Certificate root,Date now)throws Exception {
    if(!sha256(root.getEncoded()).equals(ROOT_SHA256))throw new CertificateException("Unexpected enterprise root");
    root.checkValidity(now);if(root.getBasicConstraints()!=0)throw new CertificateException("Root must prohibit intermediate CAs");root.verify(root.getPublicKey());
    if(chain==null || chain.length<1 || chain.length>2 || (chain.length==2 && !chain[1].equals(root)))throw new CertificateException("Only leaf or leaf plus pinned root allowed");
    X509Certificate leaf=(X509Certificate)chain[0];leaf.checkValidity(now);
    if(leaf.getBasicConstraints()>=0 || !leaf.getIssuerX500Principal().equals(root.getSubjectX500Principal()))throw new CertificateException("Invalid direct server issuer");
    leaf.verify(root.getPublicKey());
    List<String> eku=leaf.getExtendedKeyUsage();if(eku==null || !eku.contains("1.3.6.1.5.5.7.3.1"))throw new CertificateException("Missing serverAuth");
    boolean[] usage=leaf.getKeyUsage();if(usage==null || usage.length==0 || !usage[0])throw new CertificateException("Missing digitalSignature");
    // PKIX does not apply an anchor's own name constraints. Enforce this pinned
    // root's exact IP namespace explicitly, including every advertised SAN.
    boolean exact=false;Collection<List<?>> names=leaf.getSubjectAlternativeNames();
    if(names!=null)for(List<?> name:names){if(!Integer.valueOf(7).equals(name.get(0)) || !HOST.equals(name.get(1)))throw new CertificateException("Unexpected server SAN");exact=true;}
    if(!exact)throw new CertificateException("Missing exact LAN IP SAN");
    CertificateFactory factory=CertificateFactory.getInstance("X.509");
    PKIXParameters policy=new PKIXParameters(Collections.singleton(new TrustAnchor(root,null)));policy.setDate(now);policy.setRevocationEnabled(false);
    CertPathValidator.getInstance("PKIX").validate(factory.generateCertPath(Collections.singletonList(leaf)),policy);
    return leaf;
  }
  public static Map<String,Object> preflight(Context context)throws Exception {
    CertificateFactory factory=CertificateFactory.getInstance("X.509");X509Certificate root;
    int id=context.getResources().getIdentifier("puty_enterprise_root_ca","raw",context.getPackageName());
    try(InputStream in=context.getResources().openRawResource(id)){root=(X509Certificate)factory.generateCertificate(in);}
    if(!sha256(root.getEncoded()).equals(ROOT_SHA256))throw new CertificateException("Unexpected enterprise root");
    root.checkValidity();
    KeyStore store=KeyStore.getInstance(KeyStore.getDefaultType());store.load(null);store.setCertificateEntry("puty",root);
    TrustManagerFactory managers=TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());managers.init(store);
    SSLContext tls=SSLContext.getInstance("TLS");tls.init(null,managers.getTrustManagers(),null);
    X509Certificate leaf;
    try(Socket transport=new Socket()){
      transport.connect(new InetSocketAddress(HOST,PORT),6000);transport.setSoTimeout(6000);
      try(SSLSocket socket=(SSLSocket)tls.getSocketFactory().createSocket(transport,HOST,PORT,true)){
        socket.setSoTimeout(6000);SSLParameters parameters=socket.getSSLParameters();parameters.setEndpointIdentificationAlgorithm("HTTPS");socket.setSSLParameters(parameters);
        socket.startHandshake();leaf=validateChain(socket.getSession().getPeerCertificates(),root,new Date());
      }
    }
    long validatedAt=System.currentTimeMillis(),notBefore=Math.max(root.getNotBefore().getTime(),leaf.getNotBefore().getTime()),notAfter=Math.min(root.getNotAfter().getTime(),leaf.getNotAfter().getTime());
    if(validatedAt<notBefore || validatedAt>=notAfter)throw new CertificateException("Certificate interval changed");
    Map<String,Object> result=new HashMap<>();result.put("host",HOST);result.put("port",PORT);result.put("rootSHA256",ROOT_SHA256);result.put("leafDER",leaf.getEncoded());result.put("notBefore",notBefore);result.put("notAfter",notAfter);result.put("validatedAt",validatedAt);return result;
  }
  /** One exact LAN authority for native vault HTTP. WAN keeps platform trust. */
  public static SSLSocketFactory verifiedSocketFactory(Context context)throws Exception {
    CertificateFactory factory=CertificateFactory.getInstance("X.509");
    int id=context.getResources().getIdentifier("puty_enterprise_root_ca","raw",context.getPackageName());
    final X509Certificate root;
    try(InputStream in=context.getResources().openRawResource(id)){root=(X509Certificate)factory.generateCertificate(in);}
    if(!sha256(root.getEncoded()).equals(ROOT_SHA256))throw new CertificateException("Unexpected enterprise root");
    X509TrustManager manager=new X509TrustManager(){
      public void checkClientTrusted(X509Certificate[] chain,String type)throws CertificateException{throw new CertificateException("Client certificate unsupported");}
      public void checkServerTrusted(X509Certificate[] chain,String type)throws CertificateException{try{validateChain(chain,root,new Date());}catch(Exception failure){throw new CertificateException("Enterprise server chain rejected",failure);}}
      public X509Certificate[] getAcceptedIssuers(){return new X509Certificate[]{root};}
    };
    SSLContext tls=SSLContext.getInstance("TLS");tls.init(null,new TrustManager[]{manager},null);return tls.getSocketFactory();
  }
}

