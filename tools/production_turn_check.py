"""Check real TURN listener/proxy paths without issuing any media credentials."""
import json,os,pathlib,socket,ssl,struct
ROOT=pathlib.Path(__file__).resolve().parents[1]
CA=ROOT/'deploy/certificates/Puty-Enterprise-Root-CA.crt'


def packet():
    transaction=os.urandom(12)
    attribute=struct.pack('!HHBBBB',0x0019,4,17,0,0,0)
    return struct.pack('!HHI',3,len(attribute),0x2112a442)+transaction+attribute,transaction


def receive(s,length):
    data=b''
    while len(data)<length:
        block=s.recv(length-len(data))
        if not block: raise RuntimeError('TURN closed before response')
        data+=block
    return data


def validate(data,transaction):
    kind,size,magic=struct.unpack('!HHI',data[:8])
    assert data[8:20]==transaction and magic==0x2112a442 and len(data)==20+size
    assert kind==0x0113,'Expected unauthenticated TURN allocation challenge'
    attrs={}
    offset=20
    while offset<len(data):
        k,n=struct.unpack('!HH',data[offset:offset+4]);attrs[k]=data[offset+4:offset+4+n];offset+=4+((n+3)//4)*4
    code=attrs[9][2]*100+attrs[9][3]
    assert code==401 and 0x0014 in attrs and 0x0015 in attrs
    return {'challenge':code,'realmPresent':True,'noncePresent':True}


def main():
    results=[]
    for port,name in [(25349,'192.168.10.11'),(25349,'im.szputy.com'),(23478,None)]:
        request,txn=packet()
        with socket.create_connection(('192.168.10.11',port),timeout=8) as raw:
            s=raw
            if name:
                context=ssl.create_default_context(cafile=str(CA) if name=='192.168.10.11' else None)
                s=context.wrap_socket(raw,server_hostname=name)
            try:
                s.sendall(request);head=receive(s,20);data=head+receive(s,struct.unpack('!H',head[2:4])[0]);r=validate(data,txn)
            finally:
                if s is not raw:s.close()
        results.append({'transport':'tls' if name else 'tcp','port':port,'sni':name,**r})
    request,txn=packet()
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as s:
        s.settimeout(8);s.sendto(request,('192.168.10.11',23478));data,_=s.recvfrom(4096)
    results.append({'transport':'udp','port':23478,**validate(data,txn)})
    out=ROOT/'.private/production/acceptance/turn-check.json';out.write_text(json.dumps(results,indent=2)+'\n')
    print(json.dumps({'turnPathsVerified':len(results),'results':results}))


if __name__=='__main__':main()
