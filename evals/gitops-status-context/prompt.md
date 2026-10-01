---
name: gitops-status-context
description: Interpret recorded GitOps status without treating a context label as cluster identity.
---

---
name: gitops-status-context
description: Interpret recorded GitOps summaries conservatively.
---

Read the two fixture summaries. Report which Kubernetes context label each
observation used, which endpoint marker it returned, and what the denied
controller coverage means. Do not claim a stable cluster identity, complete
controller coverage, complete runtime Pod evidence, or ConfigHub connectivity
from these records. Keep an Argo runtime omission separate from its reported
healthy controller status. Treat these as synthetic contract fixtures, not
evidence from a live cluster.
