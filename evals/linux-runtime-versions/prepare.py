#!/usr/bin/env python3
"""Run four pinned Linux/arm64 version checks in one bounded disposable container."""
from __future__ import annotations
import argparse, base64, hashlib, importlib.util, json, math, os, re, shutil, signal, sys, time, uuid
from datetime import datetime, timezone
from pathlib import Path

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
INV04 = REPO / "evals/inv04-rbac/capture.py"
_spec = importlib.util.spec_from_file_location("linux_versions_inv04", INV04)
_inv04 = importlib.util.module_from_spec(_spec); assert _spec and _spec.loader; _spec.loader.exec_module(_inv04)
IMAGE_ID = "sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5"
OWNER_LABEL = "io.confighub.cub-scout.linux-runtime-versions.owner"
CAP = 32768
EXECUTION_SECONDS, CLEANUP_SECONDS, TOTAL_SECONDS = 45, 30, 80
EXPECTED = {
 "claude": (230482160, "2db904daea17addff9de557ba26a725916888aa7b546e2c5dd989c20d9d49ab3"),
 "kubectl": (55640226, "9f9d9c44a7b5264515ac9da5991584e2395bd50662e651132337e7b4d0c56f8f"),
 "helm": (59048120, "4d6e3a69e6203094d564d5d4e94325b1f7209421dc7e832a96d2510295e03f1d"),
 "cub-scout": (75481252, "5dac8765612592b60ec076f102c1810fbbce94b52236b3577ab3f65219e17fa6"),
}
class CaptureError(RuntimeError): pass

def sha(data: bytes) -> str: return hashlib.sha256(data).hexdigest()
def sha_file(path: Path) -> str:
 h=hashlib.sha256()
 with path.open("rb") as f:
  for b in iter(lambda:f.read(1024*1024),b""): h.update(b)
 return h.hexdigest()
def now(): return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00","Z")
def strict_json(raw: bytes, label: str):
 def pairs(items):
  d={}
  for k,v in items:
   if k in d: raise CaptureError(label+" contains duplicate JSON keys")
   d[k]=v
  return d
 def finite_float(text):
  value=float(text)
  if not math.isfinite(value): raise ValueError("nonfinite")
  return value
 try: return json.loads(raw.decode("utf-8","strict"),object_pairs_hook=pairs,parse_constant=lambda _: (_ for _ in ()).throw(ValueError()),parse_float=finite_float)
 except (ValueError,UnicodeDecodeError): raise CaptureError(label+" is malformed JSON") from None
def docker_env(source=None):
 source=os.environ if source is None else source
 e={k:source[k] for k in ("PATH","HOME","DOCKER_CONFIG","TMPDIR","LANG") if source.get(k)}
 e.setdefault("PATH","/usr/bin:/bin"); return e
def validate_context(value):
 if not isinstance(value,str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,62}",value): raise CaptureError("Docker context name is malformed")
def docker_launcher(path: Path):
 requested=path.expanduser().absolute()
 try: target=requested.resolve(strict=True)
 except OSError: raise CaptureError("Docker launcher is unavailable") from None
 if not target.is_file() or not os.access(target,os.X_OK): raise CaptureError("Docker launcher is not executable")
 return requested,target
def validate_local_endpoint(raw):
 try: v=json.loads(raw)
 except (ValueError,UnicodeDecodeError): raise CaptureError("context endpoint response is malformed") from None
 if not isinstance(v,str) or not v.startswith("unix://") or any(c in v for c in "\r\n\x00"): raise CaptureError("Docker context must resolve to a local Unix socket")
 return v
def validate_asset(path: Path, name: str):
 if name not in EXPECTED: raise CaptureError("unknown pinned asset")
 if path.is_symlink() or not path.is_file(): raise CaptureError(name+" source is missing or unsafe")
 size,digest=EXPECTED[name]
 if path.stat().st_size!=size or sha_file(path)!=digest: raise CaptureError(name+" source size/hash differs from reviewed pin")
 with path.open("rb") as f: ident=f.read(20)
 if len(ident)<20 or ident[:4]!=b"\x7fELF" or ident[4:6]!=b"\x02\x01" or int.from_bytes(ident[18:20],"little")!=183:
  raise CaptureError(name+" is not ELF64 little-endian AArch64")
 return {"bytes":size,"sha256":digest,"elf":"ELF64 little-endian AArch64"}
def validate_staged(path: Path, name: str): return validate_asset(path,name)
def validate_image_inspect(raw):
 try: actual=raw.decode("ascii","strict").strip()
 except UnicodeDecodeError: raise CaptureError("image inspect response is malformed") from None
 if actual!=IMAGE_ID: raise CaptureError("cached image differs from exact reviewed image ID")
 return actual
def build_create_args(name,owner,stage):
 if not re.fullmatch(r"scout-linux-ver-[0-9a-f]{16}",name) or not re.fullmatch(r"[0-9a-f]{32}",owner): raise CaptureError("owned container identity malformed")
 if stage.is_symlink() or not stage.is_dir(): raise CaptureError("staging path is unsafe")
 return ["container","create","--name",name,"--label",OWNER_LABEL+"="+owner,"--pull=never","--network=none","--read-only","--user","65534:65534","--cap-drop=ALL","--security-opt=no-new-privileges","--pids-limit=64","--memory=1g","--cpus=1","--tmpfs","/tmp:rw,nosuid,nodev,size=64m,uid=65534,gid=65534","--mount","type=bind,src="+str(stage)+",dst=/tools,readonly",IMAGE_ID,"python3","/tools/payload.py"]
def _one(raw):
 v=strict_json(raw,"container inspect")
 if isinstance(v,list) and len(v)==1 and isinstance(v[0],dict): return v[0]
 if isinstance(v,dict): return v
 raise CaptureError("container inspect did not identify one object")
def validate_identity(obj,name,owner):
 config=obj.get("Config")
 labels=config.get("Labels") if isinstance(config,dict) else None
 ident=obj.get("Id")
 if not isinstance(ident,str) or not re.fullmatch("[0-9a-f]{64}",ident) or obj.get("Name")!="/"+name or not isinstance(labels,dict) or labels.get(OWNER_LABEL)!=owner:
  raise CaptureError("container ownership did not match this invocation")
 return ident
def validate_inspect(raw,name,owner,stage,*,finished=False):
 o=_one(raw); ident=validate_identity(o,name,owner); c=o.get("Config"); h=o.get("HostConfig")
 if not isinstance(c,dict) or not isinstance(h,dict): raise CaptureError("container configuration malformed")
 if c.get("Image")!=IMAGE_ID or o.get("Image")!=IMAGE_ID or c.get("User")!="65534:65534" or h.get("ReadonlyRootfs") is not True or h.get("NetworkMode")!="none" or h.get("CapDrop")!=["ALL"] or h.get("SecurityOpt") not in (["no-new-privileges"],["no-new-privileges:true"]) or h.get("Privileged") is not False or h.get("CapAdd") not in (None,[]) or h.get("PidMode") not in (None,"","private") or h.get("IpcMode") not in (None,"","private") or h.get("PidsLimit")!=64 or h.get("Memory")!=1073741824 or h.get("NanoCpus")!=1000000000:
  raise CaptureError("container security/resource settings differ from configured bounds")
 tmp=h.get("Tmpfs")
 if not isinstance(tmp,dict) or tmp.get("/tmp") not in ("rw,nosuid,nodev,size=67108864,uid=65534,gid=65534","rw,nosuid,nodev,size=64m,uid=65534,gid=65534"):
  raise CaptureError("private /tmp tmpfs setting differs")
 mounts=o.get("Mounts")
 if not isinstance(mounts,list) or any(not isinstance(x,dict) for x in mounts): raise CaptureError("container mount list malformed")
 binds=[x for x in mounts if x.get("Type")=="bind"]
 tmpfs_mounts=[x for x in mounts if x.get("Type")=="tmpfs"]
 if (len(binds)!=1 or binds[0].get("Source")!=str(stage.resolve()) or binds[0].get("Destination")!="/tools" or binds[0].get("RW") is not False
     or len(tmpfs_mounts)>1 or any(x.get("Destination")!="/tmp" or x.get("RW") is not True for x in tmpfs_mounts)
     or any(x.get("Type") not in ("bind","tmpfs") for x in mounts)):
  raise CaptureError("container must have one read-only /tools bind and only the private /tmp tmpfs")
 if finished:
  state=o.get("State")
  if not isinstance(state,dict) or state.get("Status")!="exited" or type(state.get("ExitCode")) is not int: raise CaptureError("container completion state malformed")
 return ident,{"containerId":ident,"user":c["User"],"networkMode":h["NetworkMode"],"readOnlyRootfs":True,"configuredBoundsVerified":True,"exitCode":o.get("State",{}).get("ExitCode") if finished else None}
def _exact_missing(code,out,err,target):
 if type(code) is not int or code==0 or out not in (b"",b"\n"): return False
 try: lines=[x.strip() for x in err.decode("utf-8","strict").splitlines() if x.strip()]
 except UnicodeDecodeError:return False
 return lines in (["Error response from daemon: No such container: "+target],
                  ["Error: No such object: "+target])
def cleanup(docker,context,name,owner,env,deadline,run,known_id=""):
 result={"attempted":True,"verifiedAbsent":False,"errors":[]}
 call=lambda args,sec: run([str(docker),"--context",context,*args],min(sec,max(.001,deadline-time.monotonic())),env=env,max_output=CAP*4)
 try:
  target=known_id or name
  began=time.monotonic(); started=now(); code,out,err=call(["container","inspect","--format","{{json .}}",target],5)
  result.setdefault("operations",[]).append({"operation":"inspect-before-cleanup","startedAt":started,"endedAt":now(),"elapsedSeconds":time.monotonic()-began,"exitCode":code,"stdoutSha256":sha(out),"stderrSha256":sha(err)})
  if code and _exact_missing(code,out,err,target): result["verifiedAbsent"]=True; return result
  if code: raise CaptureError("owned container inspect failed; cleanup uncertain")
  ident=validate_identity(_one(out),name,owner)
  if known_id and ident!=known_id: raise CaptureError("owned container ID changed")
  result["containerId"]=ident
  if time.monotonic()>=deadline: raise CaptureError("cleanup deadline expired")
  began=time.monotonic(); started=now(); code,rm_out,err=call(["container","rm","--force",ident],10); result["removeExitCode"]=code
  result["operations"].append({"operation":"remove-owned-container","startedAt":started,"endedAt":now(),"elapsedSeconds":time.monotonic()-began,"exitCode":code,"stdoutSha256":sha(rm_out),"stderrSha256":sha(err)})
  began=time.monotonic(); started=now(); code,out,err=call(["container","inspect","--format","{{json .}}",ident],5)
  result["operations"].append({"operation":"verify-absent","startedAt":started,"endedAt":now(),"elapsedSeconds":time.monotonic()-began,"exitCode":code,"stdoutSha256":sha(out),"stderrSha256":sha(err)})
  result["verifiedAbsent"]=_exact_missing(code,out,err,ident)
  if not result["verifiedAbsent"]: raise CaptureError("owned container absence could not be verified")
 except BaseException as e: result["errors"].append(str(e)[:200] if isinstance(e,CaptureError) else type(e).__name__)
 return result

def safe_output(path):
 p=path.expanduser().absolute()
 if p.exists() or p.is_symlink(): raise CaptureError("output directory must be new")
 parent=p.parent.resolve(strict=True)
 if not any(parent==Path(root).resolve() or Path(root).resolve() in parent.parents for root in ("/tmp","/var/tmp")):
  raise CaptureError("output must be under /tmp or /var/tmp")
 p=parent/p.name
 p.mkdir(mode=0o700); os.chmod(p,0o700)
 return p
def write_new(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,"wb") as f: f.write(data)
def remove_stage(stage):
 expected=set(EXPECTED)|{"payload.py"}
 if stage.is_symlink() or not stage.is_dir() or {p.name for p in stage.iterdir()}!=expected:
  raise CaptureError("staging directory contents changed; refusing cleanup")
 os.chmod(stage,0o700)
 for child in stage.iterdir():
  if child.is_symlink() or not child.is_file(): raise CaptureError("staging entry changed; refusing cleanup")
  child.unlink()
 stage.rmdir()
def _validate_results(raw):
 v=strict_json(raw,"version payload")
 if not isinstance(v,dict) or set(v)!={"schema","results","complete"} or v.get("schema")!="linux-runtime-versions.v1" or v.get("complete") is not True or not isinstance(v.get("results"),list): raise CaptureError("version payload schema or completion invalid")
 results=v["results"]
 names=["claude","kubectl","helm","cub-scout"]
 if len(results)!=4 or [r.get("name") if isinstance(r,dict) else None for r in results]!=names: raise CaptureError("version payload is incomplete or out of order")
 for r in results:
  required={"name","exitCode","elapsedSeconds","stdoutBytes","stderrBytes","stdoutSha256","stderrSha256","stdout","stderr","stdoutBase64","stderrBase64","outputTruncated","versionMatched","status"}
  if set(r)!=required or isinstance(r.get("elapsedSeconds"),bool) or not isinstance(r.get("elapsedSeconds"),(int,float)) or not math.isfinite(r["elapsedSeconds"]) or r["elapsedSeconds"]<0:
   raise CaptureError("version result fields or timing are malformed")
  if (r.get("status")!="passed" or type(r.get("exitCode")) is not int or r["exitCode"]!=0 or r.get("versionMatched") is not True or r.get("outputTruncated") is not False):
   raise CaptureError("one or more pinned version checks failed")
  for stream in ("stdout","stderr"):
   val=r.get(stream)
   if not isinstance(val,str) or len(val.encode("utf-8"))>CAP: raise CaptureError("version output is malformed or over cap")
  for stream in ("stdout","stderr"):
   encoded=r.get(stream+"Base64")
   try: raw=base64.b64decode(encoded,validate=True)
   except (ValueError,TypeError): raise CaptureError("raw version output encoding is malformed") from None
   digest=r.get(stream+"Sha256")
   if (type(r.get(stream+"Bytes")) is not int or len(raw)!=r.get(stream+"Bytes")
       or not isinstance(digest,str) or not re.fullmatch("[0-9a-f]{64}",digest) or sha(raw)!=digest
       or r.get(stream)!=raw.decode("utf-8","replace")):
    raise CaptureError("raw version output length/hash mismatch")
 return results

def capture(assets:Path,docker_path:Path,context:str,output_path:Path,*,runner=None):
 validate_context(context); docker,docker_target=docker_launcher(docker_path)
 if not assets.is_dir() or assets.is_symlink(): raise CaptureError("pinned asset directory is unavailable or unsafe")
 source={name:validate_asset(assets/name,name) for name in EXPECTED}
 source_paths={name:assets/name for name in EXPECTED}
 payload=(HERE/"payload.py").read_bytes()
 if (HERE/"payload.py").is_symlink(): raise CaptureError("authored payload is unsafe")
 out=safe_output(output_path); stage=out/"tools"; stage.mkdir(mode=0o700)
 owner=uuid.uuid4().hex; name="scout-linux-ver-"+uuid.uuid4().hex[:16]
 for name_file in EXPECTED:
  dst=stage/name_file
  shutil.copyfile(assets/name_file,dst,follow_symlinks=False); os.chmod(dst,0o555)
  if validate_staged(dst,name_file)!=source[name_file]: raise CaptureError("staged asset differs from source pin")
 pfile=stage/"payload.py"; write_new(pfile,payload); os.chmod(pfile,0o444)
 os.chmod(stage,0o555)
 hashes={k:v["sha256"] for k,v in source.items()}; stage_hashes={p.name:sha_file(p) for p in stage.iterdir()}
 run=_inv04.run_bounded if runner is None else runner; env=docker_env()
 total_start=time.monotonic(); exec_deadline=total_start+EXECUTION_SECONDS; total_deadline=total_start+TOTAL_SECONDS
 inv04_hash=sha(INV04.read_bytes()); helper_hash=sha(Path(__file__).read_bytes())
 receipt={"schema":"linux-runtime-versions-capture.v1","status":"running","sourceAssets":source,
  "stagedSha256":stage_hashes,"payloadSha256":sha(payload),"expectedImageId":IMAGE_ID,
  "cubScoutSource":{"revision":"0d0fd7d54e5b3b1f16d27df6fa299950388873d6","buildTarget":"linux/arm64","cgo":False,"moduleNetwork":"off","installed":False},
  "helperSha256":helper_hash,"inv04RunnerSha256":inv04_hash,
  "requestedDockerContext":context,"dockerLauncher":{"requestedPath":str(docker),"resolvedPath":str(docker_target),"sha256":sha_file(docker_target)},
  "configuredBounds":{"network":"none","readOnlyRootfs":True,"user":"65534:65534","capDrop":["ALL"],"noNewPrivileges":True,"pidsLimit":64,"memoryBytes":1073741824,"cpus":1,"tmpfs":{"path":"/tmp","sizeBytes":67108864,"nosuid":True,"nodev":True}},
  "limits":{"perCommandSeconds":8,"perStreamBytes":CAP,"executionSeconds":EXECUTION_SECONDS,"cleanupSeconds":CLEANUP_SECONDS,"totalSeconds":TOTAL_SECONDS},
  "scope":"four serial version-only commands; no cluster, model, provider, login, doctor, MCP, package update, install, download, pull, or build",
  "containerConfigurationInspectVerified":False,"containerCleanup":{"attempted":False,"verifiedAbsent":False,"errors":[]},"commands":[]}
 container_id=""; create_attempted=False; error=None; results=None
 prior_signals={sig:signal.getsignal(sig) for sig in (signal.SIGINT,signal.SIGTERM)}
 def interrupted(signum,_frame): raise CaptureError("capture interrupted by "+signal.Signals(signum).name)
 for sig in prior_signals: signal.signal(sig,interrupted)
 def call(args,timeout=10):
  remaining=exec_deadline-time.monotonic()
  if remaining<=0: raise CaptureError("overall execution deadline expired")
  began=time.monotonic(); started=now(); code=out=err=None; failure=None
  try:
   code,out,err=run([str(docker),"--context",context,*args],min(timeout,remaining),env=env,max_output=CAP*32)
   if type(code) is not int or not isinstance(out,bytes) or not isinstance(err,bytes):
    raise CaptureError("bounded command runner returned an incomplete result")
   return code,out,err
  except BaseException as exc:
   failure=str(exc)[:200] if isinstance(exc,(_inv04.CaptureError,CaptureError)) else type(exc).__name__
   raise
  finally:
   output_available=code is not None and isinstance(out,bytes) and isinstance(err,bytes)
   record={"operation":" ".join(args[:2]),"arguments":args,"startedAt":started,"endedAt":now(),
    "elapsedSeconds":time.monotonic()-began,"exitCode":code,
    "stdoutBytes":len(out) if output_available else None,"stderrBytes":len(err) if output_available else None,
    "stdoutSha256":sha(out) if output_available else None,"stderrSha256":sha(err) if output_available else None,
    "outputCapture":"available-not-retained" if output_available else "unavailable",
    "rawOutputRetained":False}
   if failure: record["captureFailure"]=failure
   receipt["commands"].append(record)
 try:
  code,raw,err=call(["context","inspect",context,"--format","{{json .Endpoints.docker.Host}}"],8)
  if code: raise CaptureError("explicit Docker context inspection failed")
  endpoint=validate_local_endpoint(raw)
  code,raw,err=call(["image","inspect","--format","{{.Id}}",IMAGE_ID],8)
  if code: raise CaptureError("pinned image is not cached; pulls are disabled")
  image_id=validate_image_inspect(raw)
  create_attempted=True
  args=build_create_args(name,owner,stage)
  code,raw,err=call(args,10)
  if code: raise CaptureError("owned container creation failed")
  container_id=raw.decode("ascii","strict").strip()
  if not re.fullmatch("[0-9a-f]{64}",container_id): raise CaptureError("container create returned malformed ID")
  code,raw,err=call(["container","inspect","--format","{{json .}}",container_id],8)
  if code: raise CaptureError("could not inspect owned container before execution")
  write_new(out/"container-before.inspect.json",raw)
  receipt["commands"][-1].update({"outputCapture":"retained-stdout","rawOutputRetained":True,
   "rawOutputFiles":["container-before.inspect.json"]})
  _,inspected=validate_inspect(raw,name,owner,stage)
  receipt["containerConfigurationInspectVerified"]=True
  code,raw,err=call(["start","--attach",container_id],EXECUTION_SECONDS)
  write_new(out/"version-payload.stdout.bin",raw); write_new(out/"version-payload.stderr.bin",err)
  receipt["commands"][-1].update({"outputCapture":"retained","rawOutputRetained":True,
   "rawOutputFiles":["version-payload.stdout.bin","version-payload.stderr.bin"]})
  receipt["containerStartExitCode"]=code
  code2,inspect_bytes,inspect_err=call(["container","inspect","--format","{{json .}}",container_id],6)
  if code2: raise CaptureError("could not inspect completed container")
  _,final=validate_inspect(inspect_bytes,name,owner,stage,finished=True)
  if code or final.get("exitCode")!=0: raise CaptureError("version payload container did not exit successfully")
  results=_validate_results(raw)
  summaries=[]
  for r in results:
   summary={k:v for k,v in r.items() if k not in ("stdout","stderr","stdoutBase64","stderrBase64")}
   for stream in ("stdout","stderr"):
    raw_stream=base64.b64decode(r[stream+"Base64"],validate=True)
    write_new(out/(r["name"]+"."+stream+".bin"),raw_stream)
   summaries.append(summary)
  receipt.update({"imageId":image_id,"dockerEndpointKind":"local-unix-socket","container":final,"versionResults":summaries})
 except BaseException as exc:
  error=str(exc)[:300] if isinstance(exc,CaptureError) else type(exc).__name__
 finally:
  started=time.monotonic(); deadline=min(total_deadline,started+CLEANUP_SECONDS)
  if create_attempted: receipt["containerCleanup"]=cleanup(docker,context,name,owner,env,deadline,run,container_id)
  else: receipt["containerCleanup"]={"attempted":False,"verifiedAbsent":True,"errors":[]}
  clean=receipt["containerCleanup"].get("verifiedAbsent") is True and not receipt["containerCleanup"].get("errors")
  after={}
  try:
   after={p.name:sha_file(p) for p in stage.iterdir()}
   if after!=stage_hashes: error=error or "staged inputs changed during run"
  except OSError: error=error or "staged input verification failed"
  receipt["stagedAfterSha256"]=after
  try: source_after={name:sha_file(path) for name,path in source_paths.items()}
  except OSError: source_after={}
  if source_after!={name:item["sha256"] for name,item in source.items()}: error=error or "source asset changed during run"
  receipt["sourceAfterSha256"]=source_after
  try:
   receipt["payloadAfterSha256"]=sha(HERE.joinpath("payload.py").read_bytes())
   receipt["helperAfterSha256"]=sha(Path(__file__).read_bytes())
   receipt["inv04RunnerAfterSha256"]=sha(INV04.read_bytes())
  except OSError:
   receipt["payloadAfterSha256"]=receipt["helperAfterSha256"]=receipt["inv04RunnerAfterSha256"]=""
  if receipt["payloadAfterSha256"]!=receipt["payloadSha256"] or receipt["helperAfterSha256"]!=helper_hash or receipt["inv04RunnerAfterSha256"]!=inv04_hash:
   error=error or "helper or runner source changed during run"
  if clean:
   try:
    remove_stage(stage); receipt["stagingCleanupVerified"]=True
   except (OSError,CaptureError):
    receipt["stagingCleanupVerified"]=False; error=error or "owned staging cleanup failed"
  else:
   receipt["stagingCleanupVerified"]=False
   receipt["stagingRetainedBecauseContainerAbsenceUnverified"]=True
  for sig,handler in prior_signals.items(): signal.signal(sig,handler)
  receipt.update({"status":"passed" if error is None and results is not None and clean else "failed","error":error,
    "containerCleanupSeconds":time.monotonic()-started,"elapsedSeconds":time.monotonic()-total_start,"endedAt":now()})
  receipt_written=True
  try: write_new(out/"receipt.json",(json.dumps(receipt,sort_keys=True,indent=2)+"\n").encode())
  except OSError: receipt_written=False
 return 0 if receipt_written and receipt["status"]=="passed" else 1

def main(argv=None):
 p=argparse.ArgumentParser(description=__doc__); p.add_argument("--execute",action="store_true",help="required to create and remove one owned container")
 p.add_argument("--assets",type=Path,required=True); p.add_argument("--docker-binary",type=Path,required=True); p.add_argument("--docker-context",required=True); p.add_argument("--image-id",required=True); p.add_argument("--output-dir",type=Path,required=True)
 a=p.parse_args(argv)
 if not a.execute: p.error("--execute is required")
 if a.image_id!=IMAGE_ID: p.error("--image-id must match exact reviewed cached image pin")
 try:return capture(a.assets,a.docker_binary,a.docker_context,a.output_dir)
 except CaptureError as e:p.exit(1,"linux runtime version gate failed: "+str(e)+"\n")
if __name__=="__main__": raise SystemExit(main())
