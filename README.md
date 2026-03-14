# spktool

`spktool` is a Go rearchitecture of the legacy `vagrant-spk` and `lima-spk`
tools. The immediate goal is to keep the old command surface recognizable while
moving the internals to explicit provider plugins, provider-agnostic services,
typed workflows, and a stable project state file.

## Current Shape

- One binary, with provider selection via `--provider` or argv compatibility
  (`vagrant-spk` and `lima-spk` can become symlinks later).
- A `Provider` boundary for VM lifecycle and guest execution.
- Provider-agnostic project services for `setupvm`, `upgradevm`, `init`, and VM
  lifecycle calls.
- A workflow engine with rollback hooks and typed error wrapping.
- A structured command runner with timeout/retry/redaction/trace ID support.
- Embedded legacy assets under `internal/templates/assets/`.
- Stable state persisted to `.sandstorm/project-state.json`.

## Layout

- `cmd/spktool/`: main entrypoint
- `internal/cli/`: command dispatch and compatibility surface
- `internal/services/`: provider-agnostic business logic
- `internal/providers/`: provider interfaces and vagrant/lima plugins
- `internal/workflow/`: explicit step execution and rollback
- `internal/runner/`: structured external command execution
- `internal/state/`: durable project state
- `internal/templates/`: embedded stacks, helpers, and box assets
- `internal/keys/`: swappable keyring abstraction

## Next Migration Steps

1. Port the remaining business commands (`dev`, `pack`, `publish`, `verify`,
   key operations, `enter-grain`) onto the service layer.
2. Replace provider bootstrap stubs with typed template rendering for Lima and
   Vagrant configs.
3. Add provider contract tests and smoke tests against real tools.
4. Add migration code for legacy `.sandstorm` layouts with no state file.
