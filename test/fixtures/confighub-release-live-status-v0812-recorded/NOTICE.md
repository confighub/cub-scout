# Release live status, recorded from a real ConfigHub

Recorded on 2026-10-26 by `TestRecordReleaseLiveStatusOnARealServer` in the
Connected E2E lane (CI run 40396810015): a disposable ConfigHub v0.8.12 and
`cub` v0.8.12, on a throwaway kind cluster. Not a hosted server.

- `release-list.json`: `cub release list --space <space> -o json` after the
  test wrote a `LiveStatus` on the Release with `cub release update --patch
  <release-id> --from-stdin`, the merge patch argobot sends. The status values
  were written by the test, not by a deploying tool.
- `mcp-live-status.json`: the built binary's MCP `confighub_live_status`
  answer for that space.

IDs are those of the disposable server, which no longer exists.
