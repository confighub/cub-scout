I exported what my cluster runs into the `cluster` directory in the current working directory: Argo CD Applications, Flux Kustomizations and OCIRepositories as JSON, the `argocd-cm` keys, namespaces, workloads, and every Secret by namespace, name, type and labels (no Secret data). The same directory holds the long help text of `cub cluster attach` from cub v0.11.4. This is recorded evidence: any available cub-scout MCP responses are recordings from this cluster, not independent live confirmation. Use whatever tools you have available.

I am about to run `cub cluster attach staging --controller argo --no-install` against this cluster, without `--existing-secrets`. Will attach go ahead? If it will not, which object in the cluster decides that?

Finish with exactly these two lines:

`ATTACH: proceeds` or `ATTACH: refuses` or `ATTACH: unknown`
`BLOCKER: <namespace/name>` naming the object that makes attach refuse, or `BLOCKER: none`
