eu-central-test1  mer-kyverno-eu-central-test1   OutOfSync  Progressing  sha256:3e39eaa74376  yes      release 3, created 2026-09-28T11:03:43Z, not applied yet
eu-central-uat1   mer-kyverno-eu-central-uat1    Synced     Healthy      sha256:e2b3ed3756b1  yes
```

ConfigHub's component map shows the same readings on each cluster's variant.
Each is marked Live and Synced, and is marked behind while a rollout still
has a release to bring it:

![The Meridian slice in ConfigHub's component map: mer-kyverno-base, three class bases (prod, test, uat), and six cluster variants, each marked Live and Synced, some one release behind a rollout in progress](../images/sveltos/sveltos-meridian-tree.png)

- **Synced and Healthy:** Sveltos applied the latest release, and the
  Deployments, StatefulSets and DaemonSets it delivers were available when it
  did. The revision is the digest of that release.
- **OutOfSync and Progressing:** a newer release was created after Sveltos
  last applied, or Sveltos is still deploying.
- **Degraded:** Sveltos reports a failure, and the message says what failed.

**Which release a cluster runs is worked out from times.** Sveltos does not
report which release it fetched. So `status` takes the latest release created
before Sveltos last applied the delivery profile, and reports its digest. Two
releases created close together, or clocks that disagree, can make it name the
wrong one. That matters, because when the revision equals a release's digest,
ConfigHub moves that release's change order on by itself. The reading says
what Sveltos applied and when; it is not proof that the cluster runs that exact
release. Checking the running objects against the release is
[#39](https://github.com/confighub/sveltos-confighub/issues/39)'s drift report.
