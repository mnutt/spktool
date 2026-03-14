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
  supports machine-readable output.
- `internal/services`
  Owns provider-agnostic business workflows such as project setup, state
  management, initialization, and VM lifecycle orchestration.
- `internal/providers`
  Defines the `Provider` contract:
  `Up/Halt/Destroy/SSH/Exec/Provision/Status`
- `internal/workflow`
  Runs explicit steps with rollback hooks and returns typed workflow errors.
- `internal/runner`
  Executes external commands with trace IDs, retries, timeouts, captured
  stdout/stderr, and redaction hooks.
- `internal/state`
  Persists stable project state to `.sandstorm/project-state.json`.
- `internal/templates`
  Embeds stacks, box assets, and helpers directly into the binary.
- `internal/keys`
  Abstracts signing key storage behind a replaceable interface.

## Provider Model

Each provider plugin has two responsibilities:

1. Implement the runtime contract for VM lifecycle and command execution.
2. Contribute provider-specific bootstrap files during `setupvm` and
   `upgradevm`.

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

The current scaffold uses the workflow engine for `setupvm` and `upgradevm`.
Future migration should move `dev`, `pack`, `publish`, and provisioning onto the
same pattern.

## Stable State

The new project state file lives at:

- `.sandstorm/project-state.json`

Current fields:

- `schemaVersion`
- `migration`
- `provider`
- `vmInstance`
- `stack`
- `toolVersion`
- `updatedAt`

This file is the anchor for:

- provider detection
- idempotent setup/upgrade decisions
- future migration logic
- scripting and machine-readable inspection

## Compatibility Strategy

Compatibility is handled at the CLI edge instead of inside the services:

- `argv[0] == vagrant-spk` defaults the provider to `vagrant`
- `argv[0] == lima-spk` defaults the provider to `lima`
- legacy verbs like `setupvm`, `upgradevm`, and `vm up` remain first-class

This allows internals to change without forcing users to relearn the command
surface immediately.

## Testing Pyramid

The intended test mix is:

1. Unit tests for `services`, `workflow`, `state`, and template rendering
2. Provider contract tests shared by all provider implementations
3. Small smoke tests against real provider CLIs

The current scaffold includes unit coverage for workflow rollback behavior and
state round-tripping. Provider contract fixtures should be the next addition.
