---
type: regex
pattern: '(?is)(healthMeasurement.{0,120}unmeasured|unmeasured.{0,120}controller-chain).*(currentChange.{0,120}PASS|PASS.{0,120}rollout).*(not unhealthy|does not mean unhealthy|not evidence of application health)'
flags: im
target: last_message
arm: with-only
---
