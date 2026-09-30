**The release a cluster runs is worked out, not reported.**
- **What happens:** Sveltos does not report which release it fetched, so `cub sveltos status` takes the latest release created before Sveltos last applied the profile.
- **What to do:** read `Synced` as "Sveltos applied the latest release", not as proof of the exact artifact.
