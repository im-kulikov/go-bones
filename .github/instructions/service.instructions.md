---
applyTo: "service/**"
---

# Launcher lifecycle: known non-issues

## Do not flag

- "`context.WithoutCancel` drops the deadline, so shutdown hooks can block
  forever." By design: hooks run after the callback returned, when Start's
  context is usually already done, so they get a detached context. The shutdown
  deadline belongs to `Stop`'s context, and `Stop` never waits beyond it. Hooks
  must bound their own blocking work (documented on `WithLauncherShutdownHooks`).
