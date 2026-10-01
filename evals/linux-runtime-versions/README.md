# Linux arm64 runtime version gate

This source-only packet prepares a single local, offline container gate for four pinned Linux/arm64 executables. It is a runtime prerequisite check, not ordinary-tool parity, a benchmark result, or evidence that an asset is installed. The helper has **not** been run against Docker or the target binaries as part of this source change; a reviewer must approve and invoke it separately.

The gate checks these archived inputs by byte count, SHA-256, ELF class/endianness, and AArch64 machine ID before staging and again after the container exits:

| File | Bytes | SHA-256 | Expected report |
| --- | ---: | --- | --- |
| `claude` | 230482160 | `2db904daea17addff9de557ba26a725916888aa7b546e2c5dd989c20d9d49ab3` | Claude Code 2.1.274 |
| `kubectl` | 55640226 | `9f9d9c44a7b5264515ac9da5991584e2395bd50662e651132337e7b4d0c56f8f` | client v1.36.0 |
| `helm` | 59048120 | `4d6e3a69e6203094d564d5d4e94325b1f7209421dc7e832a96d2510295e03f1d` | Helm v4.1.4 |
| `cub-scout` | 75481252 | `5dac8765612592b60ec076f102c1810fbbce94b52236b3577ab3f65219e17fa6` | source-built version string |

The expected archived source-built cub-scout revision is `0d0fd7d54e5b3b1f16d27df6fa299950388873d6`. Its binary was built for Linux/arm64 with CGO disabled and module/toolchain network access disabled. A source-built version string is not a published release-version claim. The assets and their acquisition/signature/checksum records are in the primary ignored archive at `/Users/alexis/code/cub-scout/evals/results/linux-runtime-assets-20261001`; this helper neither downloads nor re-verifies the upstream signatures.

The execution creates one newly named and labeled container from cached image ID `sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5`, after confirming that the explicitly named Docker context resolves to a local Unix socket. Every Docker request uses that context. Pulls/builds are disabled. The container has network mode `none`, read-only root filesystem, UID/GID 65534, dropped capabilities, no-new-privileges, 64 process limit, 1 GiB memory, one CPU, one read-only `/tools` bind mount, and a private 64 MiB `/tmp` tmpfs with `nosuid,nodev`. A private HOME/config/TMPDIR and minimal environment are set inside the payload; host HOME, credentials, kubeconfig, proxy, and Docker socket are not mounted or passed into the container. These are configured/inspected limits; they are not stress tests of resource exhaustion.

Only the following serial commands run, each with an 8-second timeout and 32 KiB per-stream cap:

1. `/tools/claude --version`
2. `/tools/kubectl version --client --output=json`
3. `/tools/helm version --short`
4. from `/tools`, `./cub-scout version`

The outer execution limit is 45 seconds, cleanup is independently limited to 30 seconds, and the entire operation is capped at 80 seconds. A nonzero exit, timeout, truncated output, malformed/incomplete report, or version mismatch fails the gate. It stops on the first failed version command. Each successful command's exact stdout and stderr bytes are retained as `<name>.stdout.bin` and `<name>.stderr.bin`, with per-command byte counts, SHA-256 hashes, and elapsed time in the receipt. The in-container payload stops a command if either stream exceeds 32 KiB; in that failure case, the aggregate payload output retains the observed capped prefix and reports truncation. Cleanup addresses only this invocation's exact container ID/name and ownership label, then verifies absence. Cleanup uncertainty fails the gate. It does not inspect or terminate unrelated containers or modify global Docker context state. The staged binaries are removed after their post-run hashes are checked.

After separate review, the intended local invocation is:

```sh
python3 evals/linux-runtime-versions/prepare.py \
  --execute \
  --assets /Users/alexis/code/cub-scout/evals/results/linux-runtime-assets-20261001/assets \
  --docker-binary /usr/local/bin/docker \
  --docker-context orbstack \
  --image-id sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5 \
  --output-dir /tmp/cub-scout-linux-runtime-versions-20261001-r1
```

The output path must be new and under `/tmp` or `/var/tmp`; it contains a receipt and bounded raw version output. The payload is authored source, not an external dependency. Offline source tests can be run without Docker or target binary execution:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest evals/linux-runtime-versions/test_prepare.py
```

The helper imports the repository's existing bounded command runner from `evals/inv04-rbac/capture.py`; its source hash is recorded in the receipt only after a real invocation. No package installation is needed for the unit tests.
