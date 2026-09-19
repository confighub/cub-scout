# Bounded Exec-Auth Acceptance Contract

This contract defines the success proof for bounded Kubernetes reads that use a
kubeconfig `exec` credential plugin. The tests are local and deterministic; no
live cluster or credential service is required.

## Scope

- The bounded reader keeps its existing one-discovery/one-object request budget.
- A request context deadline or cancellation bounds the credential helper too.
- Helper cancellation must reap the helper and must not leave a process or pipe
  behind. The implementation must not run an unbounded helper in a goroutine and
  return early from the request.
- Client-go's credential API versions, token/certificate validation, expiration,
  and unauthorized refresh behavior are retained where the bounded reader needs
  them.
- Static bearer-token, basic-auth, and TLS credentials continue to use the
  client-go transport unchanged.
- Helper stdout, stderr, environment, and request errors must not expose a
  credential returned by the helper.
- Unix (Linux/macOS and the other Unix targets supported by the build tags)
  starts the helper in a private process group and kills that group on
  cancellation. Windows uses the portable direct-process kill supplied by
  `os/exec`; it does not promise to terminate a detached descendant tree, but
  `WaitDelay` still bounds the parent wait and closes inherited pipes. Windows
  therefore has an explicit weaker guarantee rather than a false tree-kill
  claim.

## Deterministic test cases

1. **Hung helper cancellation**: a fixture helper records its PID, waits
   indefinitely, and has a child inherit stdout. Cancel the bounded read
   context. The read returns the context cancellation promptly, the helper PID
   is no longer alive, and no helper process remains after the test's short
   reap window. No goroutine is left waiting on helper output.
2. **Helper timeout**: a helper that never returns is run with the reader's
   bounded request context. The read returns by the context deadline, not by an
   independent goroutine timeout, and the helper is terminated and reaped.
3. **Valid token**: a helper returns a supported
   `client.authentication.k8s.io` `ExecCredential` with a token and future
   expiration. The API server sees exactly that bearer token and the read
   succeeds.
4. **Malformed credentials**: helpers returning malformed JSON, an unsupported
   API version, an empty status, only one certificate/key half, or an invalid
   certificate fail closed. The API server is not called with an unauthenticated
   fallback, and the error does not contain credential payload data.
5. **No credential leakage**: a helper writes a sentinel token to stderr and
   returns a credential error. The returned error, bounded evidence, and test
   logs do not contain the sentinel. The helper environment includes only the
   configured exec inputs plus the normal process environment; no token is
   placed in command arguments or diagnostic text.
6. **Normal static credentials**: a reader configured with a static bearer token
   succeeds without starting any helper and sends the static token exactly once.
   Existing client-go basic-auth and TLS transport behavior remains covered by
   the same no-exec wiring path.
7. **Credential refresh**: a helper returns a short-lived token, the API server
   answers the first request with `401`, and the next request receives a newly
   issued token. A canceled refresh terminates the helper and returns the
   cancellation without serving stale credentials as a successful read.
8. **Oversized helper output**: a helper emits more than 1 MiB without a valid
   credential. The read fails closed without retaining or returning the excess
   output.
9. **TLS certificate rotation**: a helper returns a client certificate/key,
   the server requires mTLS, and a forced refresh returns a different
   certificate. The next request performs a new handshake and the server sees
   the new certificate. The bounded reader uses client-go's connection tracker
   to close active as well as idle connections on certificate rotation. An
   HTTP/2 streaming regression verifies the old connection is terminated.
10. **Explicit authorization**: a caller's Authorization header is preserved,
    while exec-provided mTLS credentials are still loaded before the handshake.
11. **Terminal safety**: even with a controlling terminal, IfAvailable helpers
    receive `interactive=false`; Always helpers fail before execution. A TTY
    regression prevents background-process-group reads from hanging on SIGTTIN.

## Deliberate client-go deviations

The installed client-go v0.33.1 exec authenticator does not accept a request
context and runs its helper with `exec.Command`. Reusing it would leave a hung
helper outside the bounded read. The bounded wrapper therefore mirrors its
supported v1/v1beta1 decoding, token/certificate-pair validation, expiration
cache, and 401 refresh shape, but deliberately differs in these ways:

- helper diagnostics are discarded instead of sent to process stderr, to avoid
  credential leakage;
- helper stdout is capped at 1 MiB;
- bounded reads are always non-interactive: IfAvailable receives
  `interactive=false`, and Always fails with advice to authenticate separately;
- Windows cannot provide Unix process-group descendant cleanup.

## Verification commands

```bash
go test ./pkg/agent -run 'TestBoundedExecAuth'
go test ./pkg/agent
go test ./...
```

The implementation is complete only when the focused tests and the repository
suite pass, with no unrelated files changed.
