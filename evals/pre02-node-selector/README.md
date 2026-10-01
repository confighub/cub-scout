# PRE-02 node-selector source capture

This is a source-only helper for the frozen PRE-02 prerequisite question. The
authored fixture creates one Pod with an explicit `nodeSelector`,
`imagePullPolicy: Never`, and a cached pause image on a newly owned, pinned kind
cluster. It captures sequential raw Pod, NodeList, and UID-filtered EventList
responses before adding one label to the owned node, then captures the same
three resources after the scheduler binds the same Pod UID to that node.

The before phase must show no matching node, `PodScheduled=False` with reason
`Unschedulable`, and a UID-correlated `FailedScheduling` event naming a selector
mismatch. The after phase must show the same Pod spec and UID, the one added
node label, `PodScheduled=True`, and a matching `nodeName`. Historical failure
Events are retained but do not override current scheduling evidence. This does
not test node readiness, workload health, cloud APIs, credentials, or capacity.

The observer can only GET the authored Pod, list Nodes, and list Events in the
fixture namespace. Setup and the one node-label change use a private admin
kubeconfig and the explicit context of the generated cluster. Cleanup is
limited to a cluster name and owner marker created by this invocation. Shared
kubeconfig bytes are hashed before and after but never used for cluster access.
The capture is bounded to 120 seconds of setup/observation, 30 seconds of
cleanup, 2 MiB per command stream and API body, and a 150-second overall
ceiling. Credentials and raw kubeconfigs are never written to the output.

This authored selector experiment is inspired by the topology prerequisite
lesson; it is not a replay of Kubara live evidence. No raw capture has been run
or reviewed. PRE-02 remains planned until a successful source-reviewed capture
and separate equal-arm packaging/strict grading are complete. No benchmark
mapping or paid evaluation admission is implied.

Run pure guards offline:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest evals/pre02-node-selector/test_capture.py -v
```

Actual capture requires lead review and an explicit `--execute` invocation with
a new output directory:

```sh
python3 evals/pre02-node-selector/capture.py \
  --execute \
  --shared-kubeconfig /absolute/path/to/existing-kubeconfig \
  --output-dir /tmp/new-pre02-capture
```
