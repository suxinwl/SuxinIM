import pathlib,hashlib,tarfile,subprocess,os,json
root=pathlib.Path("/srv/openim38-upgrade/build/livekit")
if root.resolve()!=root:raise RuntimeError("unexpected build root")
checks={"source.tar.gz":"71099572908bb46d8e5afff88540c5547f3aea081e187f91b4904aa586ee7f76","go1.26.9.linux-amd64.tar.gz":"42d158b4d8f7b61ac0a830567c940a86098fb7aac52e467a5ebec03ef5cc2f8d","modules.tar":"7b3cc81b91378259809b04e02231e2eafc14b3c6e9896c7ece76a6f87ccb6ac3"}
for name,want in checks.items():
 with (root/name).open("rb") as stream:actual=hashlib.file_digest(stream,"sha256").hexdigest()
 if actual!=want:raise RuntimeError("Offline build input checksum mismatch")
suite=root/"suite";suite.mkdir(exist_ok=True)
with tarfile.open(root/"source.tar.gz") as t:t.extractall(suite,filter="data")
runtime=suite/"tools/go1.26.9";runtime.mkdir(parents=True,exist_ok=True)
with tarfile.open(root/"go1.26.9.linux-amd64.tar.gz") as t:
 for m in t:
  parts=pathlib.PurePosixPath(m.name).parts
  if not parts or parts[0]!="go" or ".." in parts:raise RuntimeError("Unexpected Go source path")
  m.name=str(pathlib.PurePosixPath(*parts[1:]));t.extract(m,runtime,filter="data")
cache=suite/".cache/go-mod/cache/download";cache.mkdir(parents=True,exist_ok=True)
with tarfile.open(root/"modules.tar") as t:t.extractall(cache,filter="data")
(runtime/"runtime.lock.json").write_text(json.dumps({"version":"go1.26.9","url":"https://go.dev/dl/go1.26.9.linux-amd64.tar.gz","sha256":checks["go1.26.9.linux-amd64.tar.gz"]},indent=2)+"\n")
env=os.environ.copy();env.update({"OPENIM38_LIVEKIT_MEMORY":"2GiB","GOPROXY":"off","GOSUMDB":"off"})
with (root/"build.log").open("w") as log:
 result=subprocess.run(["python3","tools/build_livekit.py"],cwd=suite,env=env,stdout=log,stderr=subprocess.STDOUT)
(root/"build.exit.json").write_text(json.dumps({"exit":result.returncode,"offline":True,"systemGoReplaced":False})+"\n")
print(json.dumps({"buildExit":result.returncode,"log":str(root/"build.log")}))
raise SystemExit(result.returncode)
