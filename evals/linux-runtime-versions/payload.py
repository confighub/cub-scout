#!/usr/bin/env python3
"""Authored in-container version-only check; never contacts a cluster or provider."""
import hashlib
import base64
import json
import os
import re
import selectors
import signal
import subprocess
import time
from pathlib import Path

COMMANDS = [
    ("claude", ["/tools/claude", "--version"], r"(?<![0-9])2\.1\.274(?![0-9])"),
    ("kubectl", ["/tools/kubectl", "version", "--client", "--output=json"], r'"gitVersion"\s*:\s*"v1\.36\.0"'),
    ("helm", ["/tools/helm", "version", "--short"], r"(?<![0-9])v4\.1\.4(?![0-9])"),
    ("cub-scout", ["./cub-scout", "version"], None),
]
CAP = 32768
TIMEOUT = 8


def scout_version_matches(text):
    # This exact pinned binary was built without release linker flags.
    # cmd/cub-scout/main.go includes both BuildTag and BuildDate.
    return text == "cub-scout version dev (built unknown)\n"


def run_one(name, argv, pattern):
    began = time.monotonic()
    proc = subprocess.Popen(argv, cwd="/tools", stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, env={"PATH": "/usr/bin:/bin", "HOME": "/tmp/private-home",
                            "TMPDIR": "/tmp", "LANG": "C.UTF-8"}, start_new_session=True)
    assert proc.stdout and proc.stderr
    sel = selectors.DefaultSelector()
    sel.register(proc.stdout, selectors.EVENT_READ, "stdout")
    sel.register(proc.stderr, selectors.EVENT_READ, "stderr")
    raw = {"stdout": bytearray(), "stderr": bytearray()}
    digests = {k: hashlib.sha256() for k in raw}
    sizes = {k: 0 for k in raw}
    error = None
    try:
        deadline = began + TIMEOUT
        while sel.get_map() or proc.poll() is None:
            left = deadline - time.monotonic()
            if left <= 0:
                error = "timeout"
                break
            for key, _ in sel.select(min(left, 0.1)):
                chunk = os.read(key.fileobj.fileno(), 8192)
                if not chunk:
                    sel.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                stream = key.data
                digests[stream].update(chunk)
                sizes[stream] += len(chunk)
                if len(raw[stream]) < CAP:
                    raw[stream].extend(chunk[:CAP-len(raw[stream])])
                if sizes[stream] > CAP:
                    error = "output_limit"
                    break
            if error:
                break
        if error:
            try: os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError: pass
            proc.wait(timeout=1)
        else:
            proc.wait(timeout=max(0.1, deadline-time.monotonic()))
    finally:
        sel.close()
        for stream in (proc.stdout, proc.stderr):
            if stream and not stream.closed: stream.close()
    out, err = bytes(raw["stdout"]), bytes(raw["stderr"])
    try: text = out.decode("utf-8", "strict")
    except UnicodeDecodeError: text = ""
    if name == "cub-scout":
        matched = scout_version_matches(text)
    else:
        matched = bool(re.search(pattern, (out+b"\n"+err).decode("utf-8", "replace")))
    code = None if error else proc.returncode
    passed = error is None and code == 0 and matched
    return {"name":name,"exitCode":code,"elapsedSeconds":time.monotonic()-began,"stdoutBytes":sizes["stdout"],
            "stderrBytes":sizes["stderr"],"stdoutSha256":digests["stdout"].hexdigest(),"stderrSha256":digests["stderr"].hexdigest(),
            "stdout":out.decode("utf-8","replace"),"stderr":err.decode("utf-8","replace"),
            "stdoutBase64":base64.b64encode(out).decode("ascii"),"stderrBase64":base64.b64encode(err).decode("ascii"),
            "outputTruncated":sizes["stdout"]>CAP or sizes["stderr"]>CAP,"versionMatched":matched,
            "status":"passed" if passed else "failed", **({"error":error} if error else {})}


def main():
    Path("/tmp/private-home").mkdir(mode=0o700, parents=True, exist_ok=True)
    results = []
    for name, argv, pattern in COMMANDS:
        try: result = run_one(name, argv, pattern)
        except OSError as exc:
            result = {"name":name,"exitCode":None,"elapsedSeconds":0,"stdoutBytes":0,"stderrBytes":0,
                      "stdoutSha256":hashlib.sha256(b"").hexdigest(),"stderrSha256":hashlib.sha256(b"").hexdigest(),
                      "stdout":"","stderr":"","stdoutBase64":"","stderrBase64":"","outputTruncated":False,"versionMatched":False,
                      "status":"failed","error":type(exc).__name__}
        results.append(result)
        if result["status"] != "passed":
            break
    print(json.dumps({"schema":"linux-runtime-versions.v1", "results":results,
                      "complete":len(results)==len(COMMANDS) and all(r["status"]=="passed" for r in results)},
                     sort_keys=True, separators=(",", ":")))
    return 0 if len(results)==len(COMMANDS) and all(r["status"]=="passed" for r in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
