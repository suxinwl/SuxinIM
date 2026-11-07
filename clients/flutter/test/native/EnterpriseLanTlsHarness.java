package cn.rentsoft.flutter.openim.consumer;
import java.io.*;
import java.security.cert.*;
import java.util.*;
public final class EnterpriseLanTlsHarness {
  interface Check {void run()throws Exception;}
  static int passed;
  static void accept(String name,Check check)throws Exception{check.run();passed++;System.out.println("PASS "+name);}
  static void reject(String name,Check check)throws Exception{try{check.run();}catch(Exception expected){passed++;System.out.println("PASS "+name);return;}throw new AssertionError("Unsafe acceptance: "+name);}
  static X509Certificate read(String path)throws Exception{try(InputStream in=new FileInputStream(path)){return (X509Certificate)CertificateFactory.getInstance("X.509").generateCertificate(in);}}
  public static void main(String[] args)throws Exception{
    X509Certificate root=read(args[0]),leaf=read(args[1]),other=read(args[2]);Date now=new Date();
    accept("direct signed production leaf",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf},root,now));
    accept("leaf plus identical pinned root",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf,root},root,now));
    reject("root is not server leaf",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{root},root,now));
    reject("wrong trust root pin",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf},other,now));
    reject("unrelated peer certificate",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{other},root,now));
    reject("unrelated appended root",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf,other},root,now));
    reject("intermediate or extra chain",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf,root,root},root,now));
    reject("expired production leaf",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf},root,new Date(leaf.getNotAfter().getTime()+1)));
    reject("not yet valid root",()->EnterpriseLanTls.validateChain(new java.security.cert.Certificate[]{leaf},root,new Date(root.getNotBefore().getTime()-1)));
    System.out.println("ENTERPRISE_LAN_PKIX_TESTS="+passed);
  }
}

