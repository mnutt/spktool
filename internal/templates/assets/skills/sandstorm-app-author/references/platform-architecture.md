# Platform Architecture

Use this reference when the user needs the platform model behind Sandstorm constraints.

Core model:

- Sandstorm isolates each grain separately rather than isolating only per app package.
- App packages are signed archives containing a complete runtime environment.
- Package contents are mounted read-only.
- Persistent mutable state belongs under `/var`.

Runtime behavior:

- Most web apps talk HTTP internally and rely on `sandstorm-http-bridge` to translate between HTTP and Sandstorm's Cap'n Proto `WebSession` transport.
- Apps can also speak the Sandstorm APIs directly without the HTTP bridge, but most packaged web apps do not.
- Grain processes are aggressively stopped when no tab is open and restarted on demand.
- Grains are normally suspended when inactive, so background work must tolerate suspension and later restart.
- If the app must stay alive during a bounded operation, use the `stay-awake` `spktool` utility rather than assuming the grain process will simply keep running.

Security model:

- Server-side sandboxing uses Linux isolation primitives and syscall filtering.
- The app should assume minimal ambient authority and explicit capability grants.
- Sharing is naturally safer because each grain is an isolated object.

App-author implications:

- Do not depend on global mutable package files; write state to `/var`.
- Do not assume background daemons remain alive forever.
- Design grains so they can restart cleanly and restore state from disk.
- Persist job state, checkpoints, and work queues under `/var` so incomplete work can resume safely.
- Treat restartability as a product requirement, not a cleanup task after the app appears to work once.
- Favor Sandstorm-mediated sharing, identity, and capability flow over custom cross-grain or cross-user trust models.

## Restartability Checklist

When validating a nontrivial app, check all of these:

- first launch succeeds on a fresh grain
- restart or resume behavior works after the grain is stopped and reopened
- persistent state under `/var` survives restarts
- incomplete background work is resumable or safely recoverable
- package updates and rebuilt packages still include the runtime-critical files the app needs
