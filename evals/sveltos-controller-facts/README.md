# Authored Sveltos controller-fact answer case

This opt-in case is outside the frozen 24-case paired baseline. Its authored objects follow field shapes inspected in reporter source `8187910f9fe226e109e55c4d9c7c0e21297ff424`; they are not genuine API recordings. Delivery reports `Provisioned`, while the separate health report has condition `False`. The answer must preserve both facts without joining identities or declaring current workload health/check freshness.

The scaffold installs only this case's two-object YAML file. Offline controls check exact scaffold bytes and strict accepted/rejected final answers. No model run or runtime admission is claimed.

```bash
python3 -m unittest discover -s evals/sveltos-controller-facts -p 'test_*.py' -v
GOPROXY=off GOTOOLCHAIN=local go test ./cmd/cub-scout -run 'TestSveltosController|TestSveltosHealth' -count=1
```

See the [product example](../../examples/sveltos-controller-facts/) for CLI/MCP/TUI replay. The generic Go scaffold validator also checks this authored fixture independently of the frozen benchmark.
