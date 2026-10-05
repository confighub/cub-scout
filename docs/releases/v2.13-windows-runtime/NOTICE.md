# Accepted native Windows snapshot runtime

Both native Windows runtime jobs in [CI 37345184606](https://github.com/confighub/cub-scout/actions/runs/37345184606)
passed at clean source `34d0af49`: amd64 on windows-2022 and arm64 on windows-11-arm.
These six files are unedited harness-generated JSON receipts and GoReleaser
metadata, with byte counts/hashes in `capture-manifest.json`. Original downloaded
artifacts, four executables and build logs remain privately retained at the
manifest's path; their complete inventory is hashed there. No credentials appear.

Checksum-verified native GoReleaser 2.18.2 builds the configured standalone and
kubectl alias IDs with Go 1.24.0. Each executable embeds the exact revision,
modified=false, the /v2 module, Windows target architecture and CGO disabled.
Both binaries actually execute version and trace help on each native runner.
There is no configured Windows plugin build. This is eight actual command checks.

The first two attempts on each architecture were refused because tidy changed
the CRLF checkout's go.mod representation. Git showed a modification but no
normalized dependency diff. The accepted runners preserve LF Git text and prove
go.mod/go.sum checkout bytes match Git before building. The post-build clean
source and embedded-identity checks remain mandatory; nothing is reset to hide
changes. Earlier failed logs/artifacts remain retained separately.

These binaries are independent configured snapshot builds, not byte-identical
copies of the earlier Mac-built GoReleaser archives at `78d86d50`. This evidence
establishes native Windows basic runtime compatibility of the configured builds,
not live cluster/controller acceptance, Windows plugin support, public installs,
final tagged artifacts or publication. The complete workflow can still have
other pending or failing jobs; the two native runtime job outcomes are distinct.
