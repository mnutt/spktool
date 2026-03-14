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
- Project config under `.sandstorm/box.toml` with local overrides in
  `.sandstorm/box.local.toml`.
- Generated provider/runtime artifacts under `.sandstorm/.generated/`.

## Project Files

- `.sandstorm/box.toml` is the checked-in source of truth for stack, networking,
  and provider defaults.
- `.sandstorm/box.local.toml` is for machine-local overrides such as the default
  provider.
- `.sandstorm/.generated/*` is disposable output produced by `setupvm` and
  `upgradevm`.
- Provider files like `lima.yaml` and `Vagrantfile` should not be edited by
  hand; regenerate them instead.

## Common Commands

```sh
spktool setupvm node --provider lima
spktool config render --provider lima
spktool vm up --provider lima
spktool vm provision --provider lima
spktool init --provider lima
```

`spktool config render` prints the resolved generated artifacts without
requiring the VM to exist.

## Lima Notes

- Lima 2.0.x with the Debian 12 image needs an explicit mount type for QEMU.
  `spktool` now renders `mountType: reverse-sshfs` for `vm_type = "qemu"` and
  `mountType: virtiofs` for `vm_type = "vz"`.
- `--verbose` enables Lima serial boot tailing to help diagnose startup issues.
- The generated `runtime.env` is consumed by `global-setup.sh` during
  provisioning and drives the Sandstorm bind address, external port, base URL,
  and wildcard host.

## Acceptance Tests

Real-provider acceptance tests live behind a separate build tag and environment
gate so they do not run in the default unit-test lane.

```sh
SPKTOOL_ACCEPTANCE_LIMA=1 \
GOCACHE=/tmp/go-build \
go test -tags=acceptance ./internal/acceptance -run TestLimaLifecycleAcceptance -count=1
```

The Lima acceptance test:

- uses a real `limactl`
- runs serially
- builds a fresh `spktool` binary
- creates a disposable project directory under the repo
- always attempts `vm destroy` cleanup

## Layout

- `cmd/spktool/`: main entrypoint
- `internal/cli/`: command dispatch and compatibility surface
- `internal/services/`: provider-agnostic business logic
- `internal/providers/`: provider interfaces and vagrant/lima plugins
- `internal/workflow/`: explicit step execution and rollback
- `internal/runner/`: structured external command execution
- `internal/templates/`: embedded stacks, helpers, and box assets
- `internal/keys/`: swappable keyring abstraction

## Next Migration Steps

1. Add stricter config validation and clearer config-focused subcommands.
2. Expand smoke tests around generated artifacts and real provider lifecycle.
3. Keep migrating the remaining legacy workflows onto explicit service/workflow
   steps.
