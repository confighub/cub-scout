import hashlib
import base64
import importlib.util
import json
import os
import tempfile
import time
import unittest
from pathlib import Path

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location("linux_versions_prepare",HERE/"prepare.py")
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)

def elf():
 b=bytearray(64); b[:6]=b"\x7fELF\x02\x01"; b[18:20]=(183).to_bytes(2,"little"); return bytes(b)
def valid_payload():
 out=b"ok"; err=b""
 return {"schema":"linux-runtime-versions.v1","complete":True,"results":[{"name":n,"status":"passed","exitCode":0,"elapsedSeconds":0.001,"versionMatched":True,"outputTruncated":False,"stdout":out.decode(),"stderr":"","stdoutBytes":len(out),"stderrBytes":0,"stdoutBase64":base64.b64encode(out).decode(),"stderrBase64":"","stdoutSha256":hashlib.sha256(out).hexdigest(),"stderrSha256":hashlib.sha256(err).hexdigest()} for n in ("claude","kubectl","helm","cub-scout")]}
def load_payload():
 s=importlib.util.spec_from_file_location("linux_versions_payload",HERE/"payload.py")
 mod=importlib.util.module_from_spec(s); s.loader.exec_module(mod); return mod

class AssetTests(unittest.TestCase):
 def test_asset_pin_and_arm64_identity(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/"x"; data=elf(); p.write_bytes(data)
   m.EXPECTED["claude"]=(len(data),hashlib.sha256(data).hexdigest())
   self.assertEqual(m.validate_asset(p,"claude")["elf"],"ELF64 little-endian AArch64")
   p.write_bytes(data+b"x")
   with self.assertRaises(m.CaptureError):m.validate_asset(p,"claude")
 def test_asset_rejects_wrong_architecture_and_symlink(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/"a"; raw=bytearray(elf()); raw[18:20]=(62).to_bytes(2,"little"); p.write_bytes(raw)
   m.EXPECTED["kubectl"]=(len(raw),hashlib.sha256(raw).hexdigest())
   with self.assertRaises(m.CaptureError):m.validate_asset(p,"kubectl")
   q=Path(d)/"link"; q.symlink_to(p)
   with self.assertRaises(m.CaptureError):m.validate_asset(q,"kubectl")
 def test_image_and_context_are_pinned(self):
  with self.assertRaises(m.CaptureError):m.validate_context("../shared")
  with self.assertRaises(m.CaptureError):m.validate_image_inspect(b"sha256:other\n")
 def test_create_args_have_only_bounded_readonly_mount_and_no_pull(self):
  with tempfile.TemporaryDirectory() as d:
   args=m.build_create_args("scout-linux-ver-0123456789abcdef","a"*32,Path(d))
   joined=" ".join(args)
   for token in ("--pull=never","--network=none","--read-only","65534:65534","--cap-drop=ALL","--pids-limit=64","--memory=1g","--cpus=1","readonly","/tools"):
    self.assertIn(token,joined)
   self.assertNotIn("/var/run/docker.sock",joined)

class ReceiptTests(unittest.TestCase):
 def valid_results(self):
  return valid_payload()
 def test_results_require_all_four_in_order_and_passed(self):
  self.assertEqual(len(m._validate_results(json.dumps(self.valid_results()).encode())),4)
  v=self.valid_results(); v["results"][1]["status"]="failed"
  with self.assertRaises(m.CaptureError):m._validate_results(json.dumps(v).encode())
  v=self.valid_results(); v["results"].pop()
  with self.assertRaises(m.CaptureError):m._validate_results(json.dumps(v).encode())
 def test_results_reject_duplicates_and_unbounded_output(self):
  with self.assertRaises(m.CaptureError):m._validate_results(b'{"schema":"x","schema":"y"}')
  v=self.valid_results(); v["results"][0]["stdout"]="x"*(m.CAP+1)
  with self.assertRaises(m.CaptureError):m._validate_results(json.dumps(v).encode())
 def test_strict_json_rejects_nonfinite_numbers(self):
  for raw in (b'{"v":NaN}',b'{"v":1e999}'):
   with self.assertRaises(m.CaptureError):m.strict_json(raw,"test payload")
 def test_results_reject_wrong_version_and_boolean_timing(self):
  v=self.valid_results(); v["results"][0]["versionMatched"]=False
  with self.assertRaises(m.CaptureError):m._validate_results(json.dumps(v).encode())
  v=self.valid_results(); v["results"][0]["elapsedSeconds"]=True
  with self.assertRaises(m.CaptureError):m._validate_results(json.dumps(v).encode())
 def test_payload_contains_only_four_serial_version_commands(self):
  p=load_payload()
  self.assertEqual([name for name,_,_ in p.COMMANDS],["claude","kubectl","helm","cub-scout"])
  self.assertEqual(p.COMMANDS[0][1],["/tools/claude","--version"])
  self.assertEqual(p.COMMANDS[1][1],["/tools/kubectl","version","--client","--output=json"])
  self.assertEqual(p.COMMANDS[2][1],["/tools/helm","version","--short"])
  self.assertEqual(p.COMMANDS[3][1],["./cub-scout","version"])
 def test_source_built_scout_version_includes_build_date(self):
  p=load_payload()
  self.assertTrue(p.scout_version_matches("cub-scout version dev (built unknown)\n"))
  for text in ("cub-scout version dev\n", "cub-scout version v2.12.4 (built unknown)\n", "cub-scout version dev (built unknown)\nextra\n", ""):
   self.assertFalse(p.scout_version_matches(text))
 def test_docker_environment_drops_host_proxy_and_auth_variables(self):
  env=m.docker_env({"PATH":"/bin","HOME":"/private/home","DOCKER_HOST":"tcp://remote","HTTPS_PROXY":"https://proxy","KUBECONFIG":"/private/kubeconfig","AWS_SECRET_ACCESS_KEY":"secret"})
  self.assertEqual(env,{"PATH":"/bin","HOME":"/private/home"})
 def test_inspection_rejects_wrong_security_and_mount(self):
  name="scout-linux-ver-0123456789abcdef"; owner="a"*32; ident="b"*64
  obj={"Id":ident,"Name":"/"+name,"Image":m.IMAGE_ID,"Config":{"Image":m.IMAGE_ID,"User":"65534:65534","Labels":{m.OWNER_LABEL:owner}},
   "HostConfig":{"ReadonlyRootfs":True,"NetworkMode":"none","CapDrop":["ALL"],"SecurityOpt":["no-new-privileges"],"Privileged":False,"CapAdd":[],"PidsLimit":64,"Memory":1073741824,"NanoCpus":1000000000,"Tmpfs":{"/tmp":"rw,nosuid,nodev,size=64m,uid=65534,gid=65534"}},
   "Mounts":[{"Type":"bind","Source":"/definitely-wrong-stage","Destination":"/tools","RW":False}],"State":{"Status":"created"}}
  raw=json.dumps(obj).encode()
  with self.assertRaises(m.CaptureError):m.validate_inspect(raw,name,owner,Path("/tmp/stage"))
  obj["HostConfig"]["NetworkMode"]="bridge"
  with self.assertRaises(m.CaptureError):m.validate_inspect(json.dumps(obj).encode(),name,owner,Path("/tmp/stage"))
 def test_inspection_rejects_unexpected_mount(self):
  name="scout-linux-ver-0123456789abcdef"; owner="a"*32; ident="b"*64
  obj={"Id":ident,"Name":"/"+name,"Image":m.IMAGE_ID,"Config":{"Image":m.IMAGE_ID,"User":"65534:65534","Labels":{m.OWNER_LABEL:owner}},
   "HostConfig":{"ReadonlyRootfs":True,"NetworkMode":"none","CapDrop":["ALL"],"SecurityOpt":["no-new-privileges"],"Privileged":False,"CapAdd":[],"PidsLimit":64,"Memory":1073741824,"NanoCpus":1000000000,"Tmpfs":{"/tmp":"rw,nosuid,nodev,size=64m,uid=65534,gid=65534"}},
   "Mounts":[{"Type":"bind","Source":"/tmp/stage","Destination":"/tools","RW":False},{"Type":"volume","Source":"unexpected","Destination":"/data","RW":False}],"State":{"Status":"created"}}
  with self.assertRaises(m.CaptureError):m.validate_inspect(json.dumps(obj).encode(),name,owner,Path("/tmp/stage"))
 def test_output_and_mount_use_canonical_parent(self):
  with tempfile.TemporaryDirectory(dir="/tmp") as td:
   root=Path(td); real=root/"real"; real.mkdir(); alias=root/"alias"; alias.symlink_to(real, target_is_directory=True)
   output=m.safe_output(alias/"output")
   self.assertEqual(output,real.resolve()/"output")
   stage=output/"tools"; stage.mkdir()
   args=m.build_create_args("scout-linux-ver-0123456789abcdef","a"*32,stage)
   self.assertIn("type=bind,src="+str(stage.resolve())+",dst=/tools,readonly",args)
 def test_cleanup_missing_requires_exact_target_line(self):
  exact=b"Error response from daemon: No such container: x\n"
  self.assertTrue(m._exact_missing(1,b"",exact,"x"))
  self.assertTrue(m._exact_missing(1,b"",b"Error: No such object: x\n","x"))
  self.assertFalse(m._exact_missing(1,b"",exact,"x-extra"))
  self.assertFalse(m._exact_missing(1,b"unexpected",exact,"x"))
  self.assertFalse(m._exact_missing(1,b"",exact+b"warning\n","x"))
 def test_cleanup_removes_owned_container_even_when_settings_are_invalid(self):
  name="scout-linux-ver-0123456789abcdef"; owner="a"*32; ident="b"*64; removed=[False]
  def runner(argv,timeout,env=None,max_output=None):
   args=argv[3:]
   if args[:2]==["container","inspect"] and not removed[0]:
    obj={"Id":ident,"Name":"/"+name,"Config":{"Labels":{m.OWNER_LABEL:owner}}}
    return 0,json.dumps(obj).encode(),b""
   if args[:2]==["container","rm"]: removed[0]=True; return 0,b"",b""
   if args[:2]==["container","inspect"]: return 1,b"",("Error response from daemon: No such container: "+ident+"\n").encode()
   raise AssertionError(args)
  result=m.cleanup(Path("/bin/echo"),"local",name,owner,{},time.monotonic()+3,runner,known_id=ident)
  self.assertTrue(result["verifiedAbsent"])
  self.assertFalse(result["errors"])

class MockLifecycleTests(unittest.TestCase):
 def test_full_capture_lifecycle_uses_pins_and_cleans_owned_container(self):
  with tempfile.TemporaryDirectory(dir="/tmp") as td:
   root=Path(td); assets=root/"assets"; assets.mkdir(); data=elf()
   expected={}
   for n in m.EXPECTED:
    f=assets/n; f.write_bytes(data); expected[n]=(len(data),hashlib.sha256(data).hexdigest())
   old=m.EXPECTED.copy(); m.EXPECTED.clear(); m.EXPECTED.update(expected)
   image=m.IMAGE_ID; ident="c"*64; seen=[]; removed=[False]; start_failure=[None]
   def runner(argv,timeout,env=None,max_output=None):
    args=argv[3:]; seen.append((argv,dict(env or {})))
    if args[:2]==["context","inspect"]: return 0,b'"unix:///tmp/docker.sock"',b""
    if args[:2]==["image","inspect"]: return 0,(image+"\n").encode(),b""
    if args[:2]==["container","create"]: return 0,(ident+"\n").encode(),b""
    if args[:2]==["container","inspect"]:
     target=args[-1]
     if target==ident and not removed[0]:
      # Owned and correctly configured; cleanup path also uses this record.
      name=next(a for i,a in enumerate(args) if a=="--format")
      actual_name=created_name[0]
      obj={"Id":ident,"Name":"/"+actual_name,"Image":image,"Config":{"Image":image,"User":"65534:65534","Labels":{m.OWNER_LABEL:created_owner[0]}},
       "HostConfig":{"ReadonlyRootfs":True,"NetworkMode":"none","CapDrop":["ALL"],"SecurityOpt":["no-new-privileges"],"Privileged":False,"CapAdd":[],"PidsLimit":64,"Memory":1073741824,"NanoCpus":1000000000,"Tmpfs":{"/tmp":"rw,nosuid,nodev,size=64m,uid=65534,gid=65534"}},
       "Mounts":[{"Type":"bind","Source":str(stage_seen[0]),"Destination":"/tools","RW":False}],"State":{"Status":"exited","ExitCode":0}}
      return 0,json.dumps(obj).encode(),b""
     return 1,b"",("Error response from daemon: No such container: "+target+"\n").encode()
    if args[:2]==["container","rm"]: removed[0]=True; return 0,b"",b""
    if args[:1]==["start"]:
     if start_failure[0]: raise m._inv04.CaptureError(start_failure[0])
     return 0,payload_result,b""
    raise AssertionError(args)
   created_name=[""]; created_owner=[""]; stage_seen=[None]
   # Capture generates identifiers internally; observe create argv, then answer later calls.
   original=runner
   def recording_runner(argv,timeout,env=None,max_output=None):
    args=argv[3:]
    if args[:2]==["container","create"]:
     created_name[0]=args[args.index("--name")+1]; created_owner[0]=args[args.index("--label")+1].split("=",1)[1]
     stage_seen[0]=Path(args[args.index("--mount")+1].split("src=",1)[1].split(",dst=",1)[0]).resolve()
    return original(argv,timeout,env,max_output)
   payload_result=json.dumps(valid_payload()).encode()
   out=root/"result"
   try:
    rc=m.capture(assets,Path("/bin/echo"),"local",out,runner=recording_runner)
    receipt=json.loads((out/"receipt.json").read_text())
    self.assertEqual(rc,0,receipt)
    self.assertEqual(receipt["status"],"passed")
    self.assertTrue(receipt["containerCleanup"]["verifiedAbsent"])
    self.assertEqual(len(receipt["versionResults"]),4)
    self.assertEqual((out/"claude.stdout.bin").read_bytes(),b"ok")
    self.assertTrue(receipt["stagingCleanupVerified"])
    self.assertFalse((out/"tools").exists())
    self.assertTrue(all(call[0][1:3]==["--context","local"] for call in seen))
    self.assertTrue(all("DOCKER_HOST" not in call[1] for call in seen))
    for mode,message in (("timeout","command timed out: docker"),("overflow","command output exceeded limit: docker")):
     removed[0]=False; start_failure[0]=message
     failed_out=root/("result-"+mode)
     self.assertEqual(m.capture(assets,Path("/bin/echo"),"local",failed_out,runner=recording_runner),1)
     failed=json.loads((failed_out/"receipt.json").read_text())
     start_record=next(op for op in failed["commands"] if op["operation"]=="start --attach")
     self.assertEqual(start_record["outputCapture"],"unavailable")
     self.assertFalse(start_record["rawOutputRetained"])
     self.assertIsNone(start_record["stdoutBytes"])
     self.assertIsNone(start_record["stdoutSha256"])
     self.assertEqual(start_record["captureFailure"],message)
     self.assertTrue(failed["containerCleanup"]["verifiedAbsent"])
     self.assertFalse((failed_out/"version-payload.stdout.bin").exists())
   finally:
    m.EXPECTED.clear(); m.EXPECTED.update(old)

if __name__=="__main__": unittest.main()
