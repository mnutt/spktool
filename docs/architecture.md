# Architecture

## Goals

The rearchitecture separates three concerns that are coupled in the legacy
Python scripts:

1. Provider lifecycle and guest execution
2. Sandstorm packaging business logic
3. CLI compatibility and operator ergonomics

The Go rewrite keeps the legacy command vocabulary visible while moving the
implementation to typed boundaries that are easier to test and evolve.

## Layering

- `internal/cli`
  Parses global flags, preserves legacy verbs like `setupvm` and `vm up`, and
  supports machine-readable output plus config inspection commands such as
  `config render`.
- `internal/services`
  Owns provider-agnostic business workflows such as project setup, config
  rendering, legacy migration, initialization, and VM lifecycle orchestration.
- `internal/providers`
  Defines the `Provider` contract:
  `Up/Halt/Destroy/SSH/Exec/Provision/Status`
- `internal/workflow`
  Runs explicit steps with rollback hooks and returns typed workflow errors.
- `internal/runner`
  Executes external commands with trace IDs, retries, timeouts, captured
  stdout/stderr, and redaction hooks.
- `internal/templates`
  Embeds stacks, box assets, and helpers directly into the binary.
- `internal/keys`
  Abstracts signing key storage behind a replaceable interface.

## Provider Model

Each provider plugin has two responsibilities:

1. Implement the runtime contract for VM lifecycle and command execution.
2. Contribute provider-specific bootstrap files during `setupvm` and
   `upgradevm`, and expose those same rendered artifacts for inspection via
   `config render`.

That keeps service logic provider-agnostic while still letting each backend own
its instance naming and host integration details.

## Workflow Model

Business operations are modeled as workflows rather than shell chains. A
workflow is an ordered list of named steps, and each step can declare a rollback
hook.

This structure is intended to replace:

- ad-hoc `&&` sequencing
- partially-applied setup logic
- opaque subprocess failures with no operation context

The current scaffold uses the workflow engine for `setupvm`, `dev`, `pack`,
`verify`, and `publish`. `upgradevm` still uses a simpler linear flow today and
can move onto the same pattern later if it needs explicit rollback structure.

## Stable State

The current config-driven project model centers on:

- `.sandstorm/box.toml`
- `.sandstorm/box.local.toml`
- `.sandstorm/.generated/*`

Ownership rules:

- `box.toml` is the checked-in source of truth
- `box.local.toml` is local-only override state
- `.generated/*` is disposable derived output
- provider artifacts like `lima.yaml` and `Vagrantfile` are regenerated, not
  edited in place

Legacy metadata may still exist in older projects, such as `.sandstorm/stack`
and legacy top-level provider files like `.sandstorm/lima.yaml` and
`.sandstorm/Vagrantfile`.

These are used only for explicit migration via `upgradevm`, not routine command
execution.

## Compatibility Strategy

Compatibility is handled at the CLI edge instead of inside the services:

- `argv[0] == vagrant-spk` defaults the provider to `vagrant`
- `argv[0] == lima-spk` defaults the provider to `lima`
- legacy verbs like `setupvm`, `upgradevm`, and `vm up` remain first-class,
  with `vm create` added for first-boot provisioning

This allows internals to change without forcing users to relearn the command
surface immediately.

## Testing Pyramid

The intended test mix is:

1. Unit tests for `services`, `workflow`, and template rendering
2. Provider contract tests shared by all provider implementations
3. Small smoke tests against real provider CLIs

The current scaffold includes unit coverage for workflow rollback behavior and
config-driven service behavior. Recent real-provider verification also proved
out several assumptions:

- `config render` exposes the same generated files written by `setupvm` and
  `upgradevm`
- Lima QEMU needs an explicit mount type for the Debian 12 image
- Lima provisioning must target the existing instance rather than rerunning
  `limactl start`
- shared guest setup logic cannot assume a `vagrant` user exists
- Vagrant runs correctly from `.sandstorm/.generated/`
- `runtime.env` is visible inside both Lima and Vagrant guests via the project
  mount
- build-tagged acceptance coverage now exists for both Lima and Vagrant
