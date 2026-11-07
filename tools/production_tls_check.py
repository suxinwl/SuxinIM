"""Validate real IP/SNI TLS entrypoints with certificate verification enabled."""
import hashlib
import json
import pathlib
import socket
import ssl
from cryptography import x509

ROOT=pathlib.Path(__file__).resolve().parents[1]
CA=ROOT/'deploy/certificates/Puty-Enterprise-Root-CA.crt'


def handshake(port, name, private, exclude_enterprise=False):
    context=ssl.create_default_context(cafile=str(CA) if private else None)
    if exclude_enterprise:
        # The normal Windows installer imports this root. Test absence with an
        # isolated trust store instead of assuming the operator OS lacks it.
        enterprise = x509.load_pem_x509_certificate(CA.read_bytes())
        from cryptography.hazmat.primitives import serialization
        excluded = hashlib.sha256(enterprise.public_bytes(serialization.Encoding.DER)).digest()
        roots = [der for der in context.get_ca_certs(binary_form=True)
                 if hashlib.sha256(der).digest() != excluded]
        isolated = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        if roots:
            isolated.load_verify_locations(cadata=''.join(ssl.DER_cert_to_PEM_cert(der) for der in roots))
        context = isolated
    with socket.create_connection(('192.168.10.11',port),timeout=10) as raw:
        with context.wrap_socket(raw,server_hostname=name) as connection:
            cert=x509.load_der_x509_certificate(connection.getpeercert(binary_form=True))
            return {'port':port,'name':name,'verified':True,'tls':connection.version(),'notAfter':cert.not_valid_after_utc.isoformat(),'certificateSHA256':hashlib.sha256(connection.getpeercert(binary_form=True)).hexdigest()}


def main():
    checks=[]
    for port,name,private in [(9880,'192.168.10.11',True),(9885,'192.168.10.11',True),(9880,'hr.szputy.com',False),(9885,'im.szputy.com',False),(25349,'192.168.10.11',True),(25349,'im.szputy.com',False)]:
        checks.append(handshake(port,name,private))
    for port,name,private,exclude in [(9880,'wrong.szputy.com',True,False),(9885,'hr.szputy.com',False,False),(9885,'192.168.10.11',False,True)]:
        try:
            handshake(port,name,private,exclude)
        except ssl.SSLCertVerificationError:
            checks.append({'port':port,'name':name,'negativeVerified':True,'enterpriseRootExcluded':exclude})
        else:
            raise RuntimeError('Unexpected certificate acceptance')
    result={'connectedIP':'192.168.10.11','externalNATVerified':False,'checks':checks}
    target=ROOT/'.private/production/acceptance/tls-check.json'
    target.parent.mkdir(parents=True,exist_ok=True)
    target.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(result,indent=2))


if __name__=='__main__':
    main()
