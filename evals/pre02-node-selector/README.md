# PRE-02 Kubernetes selector prerequisite

This case stages the first independently reviewed PRE-02 capture from
2026-10-01. It is a prepared, unrun benchmark case; the frozen comparison
remains non-executable and paid evaluation is not authorized. Both arms get
the same six unmodified API response bodies and the same factual capture-scope
index through the fixture-owned scaffold.

The capture used one fresh invocation-owned local kind cluster and two serial
read phases around a single node-label operation. The observations are not an
atomic snapshot. The before Pod and UID-correlated scheduler EventList record
an unschedulable state; the after Pod is scheduled to the labeled node. The
after EventList retains the earlier FailedScheduling event as well as a
Scheduled event. The Pod's Ready condition in the after response is false, so
scheduling must not be described as application readiness or health.

The capture did not access a cloud API or read a Secret, and it does not
establish a capacity explanation. It records one local fixture lifecycle, not
a general topology result or Kubara replay. `fixtures/capture-scope.json`
contains the capture source revision and helper hash, timings, context type,
request paths/statuses, raw-file hashes, and the bounded pre-observation poll
(its body hashes match the retained before Pod/EventList bodies; poll bodies
are not separately staged). It excludes setup RBAC, admin and
observer kubeconfigs, credentials, raw provenance, and derived answer
conclusions. The local primary evidence archive is
`evals/results/pre02-node-selector-20261001`; the committed case fixtures are
the staged review package.

The answer prompt defines the output enums and canonical key order. The strict
grader accepts exactly the evidence-backed JSON object, with string values,
no whitespace, duplicate or extra properties, or surrounding prose. It
requires both Pod UID observations, scheduling facts, selector evidence,
node identity, historical-event treatment, and explicit scope limits.

Run source and packaging guards offline:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/pre02-node-selector -v
KUBECONFIG=/tmp/scout-offline-validation.kubeconfig go test ./test/unit -run 'TestPRE02|TestDeliveryCaseMappingsKeepBenchmarkUnexecutable' -count=1
```

The Python lifecycle test is mocked. No test creates a cluster, uses a network,
or invokes a model. These checks do not establish harness tool parity, admit
the case to the paid benchmark, or measure savings.
