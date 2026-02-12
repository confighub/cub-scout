# Image Assets and Capture Scripts

Human-first index for screenshots, GIFs, and VHS capture scripts.

## Current Status

- Last reviewed: 2026-02-12
- Most assets were captured between 2026-01-17 and 2026-01-23.
- Treat current media as usable but due for refresh when major UI output changes.

## Primary Images in Active Docs

| Asset | Last Captured | Used In | Status |
|-------|---------------|---------|--------|
| [map-dashboard.png](map-dashboard.png) | 2026-01-23 | `README.md` | Current (Primary) |
| [map-workloads.png](map-workloads.png) | 2026-01-23 | `README.md` | Current (Primary) |

## Additional Image Assets

| Asset | Last Captured | Notes |
|-------|---------------|-------|
| [map-dashboard.gif](map-dashboard.gif) | 2026-01-22 | Animated map startup/dashboard |
| [map-deep-dive.gif](map-deep-dive.gif) | 2026-01-22 | Deep-dive tab animation |
| [map-deep-dive.png](map-deep-dive.png) | 2026-01-22 | Static deep-dive frame |
| [map-workloads.gif](map-workloads.gif) | 2026-01-22 | Workloads tab animation |
| [trace-flux.gif](trace-flux.gif) | 2026-01-22 | Trace command animation |
| [trace-flux.png](trace-flux.png) | 2026-01-22 | Static trace frame |
| [trace-ownership.gif](trace-ownership.gif) | 2026-01-22 | Trace ownership output |
| [tree-hierarchy.gif](tree-hierarchy.gif) | 2026-01-22 | Tree runtime output |
| [drift-view.gif](drift-view.gif) | 2026-01-22 | Drift view animation |

## Capture Scripts (VHS)

Run from repo root:

```bash
vhs docs/images/<script>.tape
```

| Script | Declared Output | Last Updated | Notes |
|--------|------------------|--------------|-------|
| [screenshot.tape](screenshot.tape) | `map-dashboard.gif` | 2026-01-22 | Output file is checked in |
| [map-workloads.tape](map-workloads.tape) | `map-workloads.gif` | 2026-01-22 | Output file is checked in |
| [map-deep-dive.tape](map-deep-dive.tape) | `map-deep-dive.gif` | 2026-01-22 | Output file is checked in |
| [trace-flux.tape](trace-flux.tape) | `trace-flux.gif` | 2026-01-22 | Output file is checked in |
| [trace-demo.tape](trace-demo.tape) | `trace-ownership.gif` | 2026-01-22 | Output file is checked in |
| [tree-demo.tape](tree-demo.tape) | `tree-hierarchy.gif` | 2026-01-22 | Output file is checked in |
| [drift-demo.tape](drift-demo.tape) | `drift-view.gif` | 2026-01-22 | Output file is checked in |
| [demo.tape](demo.tape) | `demo.gif` | 2026-01-17 | Output file is not currently checked in |

## Refresh Checklist

1. Re-run relevant `.tape` scripts with current CLI output.
2. Confirm referenced images in `README.md` and `docs/` still match current UI/terminology.
3. Keep PNGs for stable docs thumbnails; use GIFs where motion adds clarity.
4. Update this file's "Last reviewed" date after refresh.
