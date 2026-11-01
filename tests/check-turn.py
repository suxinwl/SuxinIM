"""STUN Binding protocol probes for the three isolated TURN transports."""
import datetime,json,os,pathlib,socket,ssl,struct,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
HOST='192.168.10.11'
CA=ROOT/'certs/ca.crt' if (ROOT/'certs/ca.crt').is_file() else ROOT/'artifacts/dev-cert/OpenIM38-Development-CA.crt'
results=[]
def check(kind,address,port):
 tx=os.urandom(12);request=struct.pack('!HHI',1,0,0x2112A442)+tx
 if kind=='udp':sock=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);sock.settimeout(8);sock.sendto(request,(address,port));response=sock.recv(4096)
 else:
  sock=socket.socket();sock.settimeout(8)
  if os.environ.get('TURN_BRIDGE_TEST')=='1' and kind=='tcp':sock.bind((HOST,0))
  sock.connect((address,port))
  if kind=='tls':sock=ssl.create_default_context(cafile=str(CA)).wrap_socket(sock,server_hostname=HOST)
  sock.sendall(request);response=sock.recv(4096)
 sock.close()
 assert len(response)>=20 and response[8:20]==tx and struct.unpack('!H',response[:2])[0]==0x101,kind+' no successful STUN Binding response'
 results.append({'transport':kind,'port':port,'bindingResponse':True,'normalTLS':kind=='tls'})
check('udp',HOST,23478)
bridge=HOST
if os.environ.get('TURN_BRIDGE_TEST')=='1':bridge=subprocess.check_output(['docker','inspect','-f','{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}','openim38-upgrade-nginx-1'],text=True).strip()
check('tcp',bridge,23478);check('tls',HOST,25349)
report={'timeUTC':datetime.datetime.now(datetime.timezone.utc).isoformat(),'syntheticProtocolProbe':True,'results':results,'limit':'STUN Binding only; not proof of authenticated allocation or relayed media.'}
if not os.environ.get('TURN_BRIDGE_TEST'):
 output=ROOT/'tests/turn-protocol-report.json';output.parent.mkdir(parents=True,exist_ok=True);output.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
