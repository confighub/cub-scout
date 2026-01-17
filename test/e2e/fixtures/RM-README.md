# Rendered Manifest Pattern with Sample Flux and Argo Repo Structures

This directory experiments with applying the rendered manifest pattern to some common git source layouts for IaC and GitOps with Flux and Argo. There are currently two sample directory structures:

* [flux-helm-kustomize](flux-helm-kustomize) is for Flux and uses Helm charts with kustomize overlays for each environment
* [argo-umbrella-charts](argo-umbrella-charts) is for Argo and uses Helm Umbrella charts with different Helm values files for each environment

Each of these are tested in two modes:

* Original: How developers would normally configure Flux and Argo to consume them as git repos
* Rendered: Bash scripts perform a full rendering of the IaC directories. The rendered file structure is then pushed to a local git server and consumed from Flux and Argo as plain yaml manifests. This tests the proposition of rendered manifests as the authoritative source.

ConfigHub is currently not involved in this example. That will be a later or separate step.

## Test Flux

### Original Mode

Start a cluster and install Flux with:

    ./scripts/setup-flux-cluster.sh

This will also install a gitea git server in the cluster.

Next, test the "original" way to configure Flux for this source repo:

    ./scripts/test-flux-original.sh

This will push the source to the git server and apply a set of Flux manifests to install the git repository as a source and to install the helm+kustomize components. In this configuration, Flux will run kustomize and then install the kustomized charts with the native Helm applier.

You can monitor progress with:

    flux get kustomizations --watch

or tools like `k9s` for direct kubernetes access.

### Rendered Mode

This tests the exact same original source repo but now we render the raw manifests using our own script and then Flux is only responsible for applying the rendered manifests directly.

Start by cleaning up:

    kind delete cluster --name flux-test

Then set up the cluster again like before

    ./scripts/setup-flux-cluster.sh

Now run:

    ./scripts/test-flux-rendered.sh

This will first use the `./scripts/flux-render.sh` script to render raw manifests from the helm+kustomize sources and save them in `./flux-rendered`. It will render a sub-directory for each environment, but we'll only use dev in this example.

As before you can monitor progress with:

    flux get kustomizations --watch

or tools like `k9s` for direct kubernetes access.

## Test Argo

### Original Mode

Start a cluster and install Argo CD with:

    ./scripts/setup-argo-cluster.sh

This will also install a gitea git server in the cluster.

Next, test the "original" way to configure Argo for this source repo:

    ./scripts/test-argo-original.sh

This will push the source to the git server and apply an ApplicationSet that uses the Helm source type. In this configuration, Argo CD uses its native Helm integration to install the umbrella charts directly, layering `base.yaml` and environment-specific values files.

Note: The ApplicationSet (`argo-umbrella-charts/clusters/applicationset.yaml`) uses a matrix generator that can target multiple environments, but only `dev` is enabled by default. To add staging or production environments, uncomment the corresponding entries in the environment generator and register those clusters with Argo CD.

You can monitor progress in the Argo CD UI at http://localhost:9080 (credentials are printed by the setup script) or with:

    argocd app list
    argocd app get dev-core

or tools like `k9s` for direct kubernetes access.

### Rendered Mode

This tests the exact same original source repo but now we render the raw manifests using our own script and then Argo CD is only responsible for applying the rendered manifests directly.

Start by cleaning up:

    kind delete cluster --name argo-test

Then set up the cluster again like before:

    ./scripts/setup-argo-cluster.sh

Now run:

    ./scripts/test-argo-rendered.sh

This will first use the `./scripts/argo-render.sh` script to render raw manifests from the umbrella chart sources and save them in `./argo-rendered`. It will render a sub-directory for each environment, but we'll only use dev in this example.

As before you can monitor progress in the Argo CD UI at http://localhost:9080 or with:

    argocd app list

or tools like `k9s` for direct kubernetes access.

## Findings

When rendering Helm charts to plain YAML for GitOps consumption, several challenges emerge that the native Helm integrations in Flux and Argo CD normally handle automatically. The render scripts (`flux-render.sh`, `argo-render.sh`, and `post-process.sh`) encode the solutions to these challenges.

### 1. CRD Ordering

**Problem**: CustomResourceDefinitions (CRDs) must be installed before any custom resources that use them. When Helm installs a chart, it handles this automatically. With plain YAML, we need to ensure ordering ourselves.

**Solution**:
- **Flux**: CRDs are split into a separate `crds/` subdirectory per group. Separate Kustomization resources are generated with `dependsOn` to ensure CRDs are applied and ready before the resources that use them.
- **Argo CD**: CRDs are annotated with `argocd.argoproj.io/sync-wave: "-1"` so they sync before regular resources (which default to wave 0).

**Architectural difference**:
- **Flux**: `dependsOn` operates at the Kustomization/HelmRelease level, so you must create separate Flux Kustomizations (or HelmReleases) to establish dependencies. This requires structural separation (separate directories and Flux Kustomizations for CRDs).
- **Argo CD**: Sync-waves are annotations on individual Kubernetes resources, allowing ordering control within a single Application without creating additional Applications.

### 2. Helm Hooks Must Be Removed

**Problem**: Helm hooks (`helm.sh/hook` annotations) are meant for Helm's lifecycle events (pre-install, post-install, test, etc.). These don't make sense in a GitOps context where there's no "install" event - just continuous reconciliation.

**Solution**: The post-process script strips all Helm hook annotations:
- `helm.sh/hook`
- `helm.sh/hook-weight`
- `helm.sh/hook-delete-policy`

It also filters out hook Jobs entirely (identified by `app.kubernetes.io/component: hooks` label), as these are typically one-time setup tasks that Helm would run and then delete.

**Potential implications**: Removing hooks means losing functionality that some charts depend on:
- **Database migrations**: Charts using pre-upgrade hooks for migrations will need an alternative approach (init containers, separate migration jobs, or manual processes)
- **Secret generation**: Charts that auto-generate passwords/keys via hooks will need pre-created secrets or external secret management
- **One-time setup**: Initial admin user creation, bootstrap data, etc. may need manual handling
- **Cleanup tasks**: Post-delete hooks won't run, potentially leaving orphaned resources

The fundamental issue is that hooks represent **imperative operations** ("run this once when X happens") in an otherwise declarative GitOps model. Converting to rendered manifests may require rethinking how these operations are handled.

### 3. Namespace Handling

**Problem**: Helm's `--namespace` flag serves two purposes: (1) with `--create-namespace`, it creates the target namespace, and (2) it sets the namespace for resources at install time without requiring it in the YAML. With plain YAML, we need to handle both: ensuring namespaces exist before resources are applied, and ensuring all resources have explicit `metadata.namespace` since there's no install-time injection.

**Solution**:
- **Namespace creation**:
  - **Flux**: A separate `namespaces/` directory is generated containing Namespace resources. A Kustomization with no dependencies applies these first.
  - **Argo CD**: The `CreateNamespace=true` sync option handles this automatically.
- **Explicit namespace in resources**: The render scripts pass `--namespace` to `helm template`, which populates `metadata.namespace` in most resources. However, some charts don't include namespace in all their templates. The Flux render script post-processes rendered YAML to add the namespace to namespaced resource kinds (Deployments, Services, ConfigMaps, etc.) that are missing it.

### 4. Cross-Group CRD Dependencies

**Problem**: Some charts in one group may use CRDs defined in another group. A common example is ServiceMonitor resources—many charts create ServiceMonitors for Prometheus scraping, but the ServiceMonitor CRD is defined by kube-prometheus-stack.

**Design decision**: In this example, kube-prometheus-stack is placed in the **core** group rather than observability. This is because core components (cert-manager, external-secrets, traefik, etc.) create ServiceMonitor resources. If prometheus were in observability, core would depend on observability for CRDs, creating an awkward dependency where "core" infrastructure depends on "observability" infrastructure.

**Solution in original mode**: This is handled at the Application/HelmRelease level through coarse-grained ordering:
- **Argo CD**: Sync-waves on Applications ensure groups deploy in order (core → security → observability → operations).
- **Flux**: `dependsOn` on HelmReleases ensures the same ordering.

**Solution in rendered mode**: Since CRDs are split into separate files, we need finer-grained control. The render scripts detect cross-group CRD dependencies and add appropriate `dependsOn` entries or sync-wave annotations.

**Cross-namespace service references**: When prometheus lives in a different namespace than the components that query it (e.g., Grafana in observability querying Prometheus in core), service URLs must use fully-qualified names like `prometheus-prometheus.core:9090`.

### 5. Component Splitting (Argo Only)

**Problem**: The Argo example uses umbrella charts that bundle multiple sub-charts into groups. Running `helm template` on an umbrella chart outputs ALL sub-charts combined into one giant YAML file (often 5,000-15,000+ lines). Giant YAML files create several problems:
- **Difficult PR reviews**: Changes to one component are buried in thousands of lines, making it hard to see what actually changed
- **Poor debuggability**: When something breaks, finding the relevant resources in a massive file is tedious
- **Noisy git diffs**: Even small changes produce diffs that are hard to reason about

**Solution**: The Argo render script parses the combined output and splits it by component. It does this by examining the `# Source:` comments that Helm includes in its output (e.g., `# Source: core/charts/cert-manager/templates/...`) and using AWK to separate resources into per-component files.

**Architectural note**: Splitting files improves the developer experience but doesn't restore per-component deployment control—we already gave that up by bundling multiple charts into a single Argo Application. An alternative approach would be to create one Application per component (similar to how Flux uses individual HelmReleases), trading fewer Applications for finer-grained control. The umbrella chart pattern prioritizes simpler Application management at the cost of per-component granularity.
