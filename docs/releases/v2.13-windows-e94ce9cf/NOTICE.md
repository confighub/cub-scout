# Latest candidate native Windows runtime proof

Both native Windows jobs in [run 37358802632](https://github.com/confighub/cub-scout/actions/runs/37358802632)
pass at exact clean source `e94ce9cf`. Configured GoReleaser builds use Go 1.24.0
and preserve canonical source inputs. The two executable IDs on amd64/arm64
pass eight actual version/help commands, exact source stamps, modified=false,
module `/v2`, CGO disabled and target architecture checks.

Six unedited JSON records and hashes are public; logs, four actual executables
and complete artifact inventories remain privately at the capture manifest's
path. The historical [34d0af49 packet](../v2.13-windows-runtime/NOTICE.md) remains
intact. Eight consistency guards verify both packets.

These are independent native snapshot builds, separate from the Mac-built
archives of the same revision. They establish basic native runtime behavior,
not execution of those exact Mac-built Windows archive bytes, cluster behavior,
public installation, final-tag acceptance or publication. Windows plugin
execution is not configured. v2.13 remains unreleased.
