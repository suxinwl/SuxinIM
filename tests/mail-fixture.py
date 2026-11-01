"""Internal synthetic-mail sink. No host port, external forwarding or production mail."""
import email,email.policy,json,os,pathlib,re,socketserver,threading,time
os.umask(0o077)
MAILBOX=pathlib.Path('/fixtures/smtp-mailbox.json');LOCK=threading.Lock()
class Handler(socketserver.StreamRequestHandler):
 def reply(self,text):self.wfile.write((text+'\r\n').encode());self.wfile.flush()
 def handle(self):
  self.reply('220 openim38.test SMTP fixture');recipient=''
  while line:=self.rfile.readline(8192):
   text=line.decode(errors='replace').strip();verb=text.split(' ',1)[0].upper()
   if verb in ('EHLO','HELO'):self.reply('250-openim38.test');self.reply('250 SIZE 1048576')
   elif verb=='MAIL':self.reply('250 Sender accepted')
   elif verb=='RCPT':
    match=re.search(r'<([^>]+)>',text);recipient=match.group(1) if match else ''
    self.reply('250 Recipient accepted' if recipient.endswith('@openim38.test') else '550 Synthetic recipients only')
   elif verb=='DATA':
    if not recipient.endswith('@openim38.test'):self.reply('550 No accepted recipient');continue
    self.reply('354 End with dot');content=[]
    while chunk:=self.rfile.readline(8192):
     if chunk in (b'.\r\n',b'.\n'):break
     content.append(chunk[1:] if chunk.startswith(b'..') else chunk)
     if sum(map(len,content))>1048576:return
    message=email.message_from_bytes(b''.join(content),policy=email.policy.default)
    payload=message.get_body(preferencelist=('html','plain')) if message.is_multipart() else message
    body=payload.get_content();code=re.search(r'\b(\d{6})\b',body)
    if code:
     with LOCK:
      current=json.loads(MAILBOX.read_text()) if MAILBOX.exists() else {}
      current[recipient]={'code':code.group(1),'receivedAt':time.time()}
      temp=MAILBOX.with_suffix('.next');temp.write_text(json.dumps(current));temp.replace(MAILBOX)
    self.reply('250 Stored synthetic message')
   elif verb in ('RSET','NOOP'):self.reply('250 OK')
   elif verb=='QUIT':self.reply('221 Bye');break
   else:self.reply('502 Not implemented')
class Server(socketserver.ThreadingTCPServer):allow_reuse_address=True;daemon_threads=True
with Server(('0.0.0.0',2525),Handler) as server:server.serve_forever()
