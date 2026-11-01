"""Resolve exact Linux/amd64 image manifests from their official registry."""
import concurrent.futures,json,pathlib,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
IMAGES={"mongo":"library/mongo:7.0","redis":"library/redis:7.0.0","kafka":"bitnamilegacy/kafka:3.5.1","minio":"bitnamilegacy/minio:2024.1.13","etcd":"bitnamilegacy/etcd:3.5.21","livekit":"livekit/livekit-server:v1.13.11","nginx":"library/nginx:1.28.0-alpine","runtime":"library/debian:bookworm-slim"}
ACCEPT=", ".join(["application/vnd.oci.image.index.v1+json","application/vnd.docker.distribution.manifest.list.v2+json","application/vnd.oci.image.manifest.v1+json","application/vnd.docker.distribution.manifest.v2+json"])
def resolve(item):
 key,image=item;repo,tag=image.rsplit(":",1)
 with urllib.request.urlopen(f"https://auth.docker.io/token?service=registry.docker.io&scope=repository:{repo}:pull",timeout=40) as response:token=json.load(response)["token"]
 headers={"Authorization":"Bearer "+token,"Accept":ACCEPT}
 with urllib.request.urlopen(urllib.request.Request(f"https://registry-1.docker.io/v2/{repo}/manifests/{tag}",headers=headers),timeout=60) as response:
  manifest=json.load(response);digest=response.headers.get("Docker-Content-Digest")
 if "manifests" in manifest:
  digest=next(x["digest"] for x in manifest["manifests"] if x.get("platform",{}).get("os")=="linux" and x.get("platform",{}).get("architecture")=="amd64")
 if not digest:raise RuntimeError("Registry did not provide an immutable digest")
 value={"tag":"docker.io/"+image,"image":"docker.io/"+repo+"@"+digest,"platform":"linux/amd64"}
 print(json.dumps({key:value}),flush=True);return key,value
if __name__=="__main__":
 with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:result=dict(pool.map(resolve,IMAGES.items()))
 (ROOT/"deploy").mkdir(exist_ok=True)
 (ROOT/"deploy/images.lock.json").write_text(json.dumps(result,indent=2)+"\n")
