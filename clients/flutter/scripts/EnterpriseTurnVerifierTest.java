package com.cloudwebrtc.webrtc;
import java.io.*;
import java.security.cert.*;
public final class EnterpriseTurnVerifierTest {
 static X509Certificate read(String path)throws Exception{return (X509Certificate)CertificateFactory.getInstance("X.509").generateCertificate(new FileInputStream(path));}
 public static void main(String[] args)throws Exception {
  X509Certificate root=read(args[0]),leaf=read(args[1]),other=read(args[2]);
  EnterpriseTurnVerifier verifier=new EnterpriseTurnVerifier(root);
  if(!verifier.verify(leaf.getEncoded()))throw new AssertionError("Production LAN leaf rejected");
  if(verifier.verify(root.getEncoded()))throw new AssertionError("CA accepted as server leaf");
  if(verifier.verify(other.getEncoded()))throw new AssertionError("Public DNS certificate accepted for LAN IP");
  if(verifier.verify(new byte[]{1,2,3}))throw new AssertionError("Corrupt DER accepted");
  if(new EnterpriseTurnVerifier(other).verify(leaf.getEncoded()))throw new AssertionError("Unrelated root accepted");
  System.out.println("Enterprise TURN PKIX verifier: 5 cases passed");
 }
}
