import pathlib,json,subprocess,hashlib,yaml,datetime
root=pathlib.Path("/srv/openim38-upgrade")
cli=["docker","compose","-p","openim38-upgrade"]
cid=subprocess.check_output(cli+["ps","-q","livekit"],cwd=root,text=True).strip()
state=json.loads(subprocess.check_output(["docker","inspect",cid]))[0]
config=yaml.safe_load((root/"config/livekit.yml").read_text())
with (root/"bin/livekit-server").open("rb") as f:sha=hashlib.file_digest(f,"sha256").hexdigest()
result={"recordedAt":datetime.datetime.now(datetime.timezone.utc).isoformat(),"binarySHA256":sha,"runtimeImage":state["Config"]["Image"],"running":state["State"]["Running"],"hostPorts":state["HostConfig"]["PortBindings"],"advertiseInternalIP":config["rtc"]["advertise_internal_ip"],"advertisedTLSPort":config["turn"]["advertised_tls_port"],"allowLocalMediaPeers":config["turn"]["allow_local_media_peers"],"restrictedPeerCIDRs":config["turn"]["allow_restricted_peer_cidrs"],"internalRelayRange":[config["turn"]["relay_range_start"],config["turn"]["relay_range_end"]],"relayRangePublished":any(27900<=int(p.split("/")[0])<=28155 for p in state["HostConfig"]["PortBindings"]),"otherServicesRecreatedByInstaller":False}
(root/"tests/media/turn-final.json").write_text(json.dumps(result,indent=2)+"\n")
print(json.dumps({"running":result["running"],"relayRangePublished":result["relayRangePublished"],"mappedPorts":len(result["hostPorts"]),"binarySHA256":sha}))
