# Trace case-sensitive ConfigHub slugs

This is an opt-in recorded interpretation case for the v2.13 Trace join
contract. Both eval arms receive the same files. The Trace payload is a
synthetic typed model fixture; the candidate rows are mocked bounded connected
inputs. Neither is a live server capture or a paid eval result.

The case checks that `PaymentsAPI` does not attach an event for `paymentsapi`,
and `West` does not attach a release for `west`. It preserves explicit
no-match omissions, ID precedence, and UUID case normalization. The product
behavior is covered by deterministic tests in
`cmd/cub-scout/trace_delivery_test.go`, including shared CLI/TUI projection.

Run the file-only scaffold from an eval workspace with:

```bash
bash scaffold.sh
```

No network, kubeconfig, ConfigHub credentials, or live reads are needed. Paid
model execution has not been run for this case.
