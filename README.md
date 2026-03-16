# spktool

`spktool` is a Go rearchitecture of the legacy `vagrant-spk` and `lima-spk`
tools. The immediate goal is to keep the old command surface recognizable while
moving the internals to explicit provider plugins, provider-agnostic services,
typed workflows, and a stable project state file.

## Current Shape

- One binary, with provider selection via `--provider` or argv compatibility
  (`vagrant-spk` and `lima-spk` can become symlinks later).
- Provider capability interfaces for VM lifecycle, guest execution, bootstrap
  rendering, guest file writes, and grain attachment.
- Provider-agnostic project services split by concern: bootstrap, packages, VM
  lifecycle, grains, and keys.
- A small workflow step runner with rollback hooks and typed error wrapping.
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
spktool vm create --provider lima
spktool init --provider lima
```

`spktool config render` prints the resolved generated artifacts without
requiring the VM to exist.

`spktool vm create` starts a VM and runs provisioning. `spktool vm up` starts an
existing VM without reprovisioning, and `spktool vm provision` reruns guest
setup explicitly.

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

SPKTOOL_ACCEPTANCE_VAGRANT=1 \
GOCACHE=/tmp/go-build \
go test -tags=acceptance ./internal/acceptance -run TestVagrantLifecycleAcceptance -count=1
```

The acceptance tests:

- use a real `limactl` or `vagrant`
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

## Architecture Status

- Service logic is split by concern into bootstrap, package, VM lifecycle,
  grain, and key services.
- The CLI depends on grouped application capabilities rather than one
  monolithic app interface.
- Providers expose smaller capability contracts, with shared provider-core
  contract tests covering the common VM lifecycle surface.
- `internal/workflow` is intentionally a minimal step runner used only where
  rollback semantics add value.
