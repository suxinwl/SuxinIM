"""Enable only the explicit downloads index; keep unknown files returning 404."""
import hashlib,json,pathlib,subprocess
root=pathlib.Path('/srv/openim38-upgrade')
if root.resolve()!=root or not (root/'compose.yaml').is_file():raise SystemExit('Wrong isolated root')
path=root/'config/nginx.conf';before=path.read_text()
needle='  location /downloads/ { autoindex off; try_files $uri =404; }'
replacement='  location = /downloads { return 301 /downloads/; }\n  location = /downloads/ { try_files /downloads/index.html =404; }\n'+needle
if before.count(needle)!=1 or 'location = /downloads/' in before:raise SystemExit('Unexpected downloads config; inspect before changing')
backup=root/'.private/nginx-before-download-index.conf';backup.write_text(before);backup.chmod(0o600)
compose=['docker','compose','-p','openim38-upgrade','--project-directory',str(root),'-f',str(root/'compose.yaml'),'exec','-T','nginx','nginx']
after=before.replace(needle,replacement)
# Preserve the file inode used by the existing read-only container bind mount.
path.write_text(after)
try:
 subprocess.run(compose+['-t'],check=True)
 subprocess.run(compose+['-s','reload'],check=True)
except Exception:
 path.write_text(before);raise
print(json.dumps({'onlyIsolatedStack':True,'reloaded':True,'beforeSHA256':hashlib.sha256(before.encode()).hexdigest(),'afterSHA256':hashlib.sha256(after.encode()).hexdigest()}))
