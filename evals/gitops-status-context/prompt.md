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
controller coverage, or ConfigHub connectivity from these records. Treat these
as synthetic contract fixtures, not evidence from a live cluster.
