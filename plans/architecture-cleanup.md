# Architecture Cleanup Plan

## Goal

Reduce the main sources of structural risk in `spktool` without changing the
user-facing command surface:

- break up the oversized `ProjectService`
- narrow the CLI application boundary
- split the provider contract into clearer capabilities
- either strengthen or simplify the workflow layer so the implementation
  matches the architectural claims

This plan is intentionally incremental. The priority is improving internal
clarity and testability while keeping command behavior stable.

## Current Concerns

### 1. Service concentration

`internal/services/services.go` currently owns too many responsibilities:

- project bootstrapping
- config rendering and generated file writing
- VM lifecycle orchestration
- package commands like `init`, `dev`, `pack`, `verify`, `publish`
- key operations
- grain entry and attach flows
- legacy migration

This makes it easy to add features quickly, but it raises the cost of safe
refactoring and creates hidden coupling between unrelated commands.

### 2. Wide CLI boundary

`internal/cli` depends on a large `App` interface with a method per command.
That keeps dispatch explicit, but it means every new command shape expands the
core interface and the test surface in lockstep.

### 3. Provider interface is too broad

The provider abstraction currently mixes:

- required VM lifecycle operations
- guest command execution
- provisioning
- status inspection
- file transfer
- grain-specific behavior

That is convenient for the current two backends, but it does not express which
capabilities are actually fundamental and which are optional.

### 4. Workflow layer is underspecified

The workflow package is useful, but right now it is closer to a structured step
runner than a full workflow engine. The implementation should either move
further toward a first-class orchestration layer or be documented and used as a
small helper instead of a major architectural pillar.

## Principles

- Preserve existing commands and flags unless there is a strong reason to
  change them.
- Avoid large all-at-once rewrites.
- Keep providers testable through shared contract tests.
- Prefer moving logic into smaller units before inventing new abstractions.
- Do not split interfaces or packages unless the resulting ownership is clear.

## Recommended Execution Order

1. Baseline and safety rails
2. Split `ProjectService` into focused services
3. Narrow the CLI boundary
4. Refactor provider capabilities
5. Re-scope the workflow layer
6. Update docs to match the resulting design

## Phase 1: Baseline And Safety Rails

### Objective

Create enough test and observability coverage that the internal refactor can be
done confidently.

### Work

- inventory command coverage in `internal/cli`, `internal/services`, and
  provider tests
- add focused tests for the most coupled flows before moving code:
  `setupvm`, `upgradevm`, `vm create`, `init`, `pack`, `enter-grain`
- add shared fixtures/helpers where tests are currently repeating setup
- verify current behavior with:
  `GOCACHE=/tmp/go-build go test ./...`

### Deliverables

- a clear list of protected command flows
- regression tests around existing command semantics
- agreement on which behavior is intentionally preserved even if internals move

### Exit Criteria

- the refactor can proceed with stable unit coverage for the key orchestration
  flows

## Phase 2: Split `ProjectService`

### Objective

Decompose `internal/services/services.go` into smaller units with clear
ownership.

### Proposed service split

- `ProjectBootstrapService`
  Owns `setupvm`, legacy migration support, generated file writing, and config
  render helpers.
- `VMLifecycleService`
  Owns `vm create`, `vm up`, `vm halt`, `vm destroy`, `vm status`,
  `vm provision`, `vm ssh`.
- `PackageService`
  Owns `init`, `dev`, `pack`, `verify`, `publish`.
- `GrainService`
  Owns `enter-grain` and grain attach/list behavior.
- `KeyService`
  Owns keyring-facing commands if they continue to exist as first-class
  operations.

### Notes

- It is acceptable to keep a thin façade in `internal/services` during the
  transition if that reduces churn.
- Shared helpers such as `loadProject()`, `projectContext()`, and generated-file
  assembly can remain internal helpers initially, but they should move toward a
  small number of coherent support packages rather than a new catch-all file.

### Files Likely To Change

- `/Users/mnutt/p/personal/spktool/internal/services/services.go`
- new files under `/Users/mnutt/p/personal/spktool/internal/services/`
- `/Users/mnutt/p/personal/spktool/internal/app/app.go`
- related tests in `/Users/mnutt/p/personal/spktool/internal/services/`

### Risks

- helper functions may remain shared in a way that recreates the same coupling
- moving code by command instead of by responsibility could produce shallow
  package splits with no real architectural gain

### Exit Criteria

- no single service file remains the dominant owner of unrelated behavior
- each command flow clearly maps to one primary service

## Phase 3: Narrow The CLI Boundary

### Objective

Reduce the size and churn of the `internal/cli` application interface.

### Options

#### Option A: Smaller capability interfaces

Keep the current dispatcher style, but replace one large `App` interface with
smaller interfaces grouped by concern:

- `ProjectBootstrapApp`
- `VMApp`
- `PackageApp`
- `KeyApp`
- `GrainApp`

This is the lowest-risk path.

#### Option B: Command handlers

Introduce command objects/handlers with a shape like:

- `Name() string`
- `Run(context.Context, Invocation) error`

This reduces central branching and makes new command additions more local, but
it is a larger change.

### Recommendation

Start with Option A. It gives most of the interface benefit without forcing a
full CLI framework redesign. Revisit command handlers only after the service
split lands cleanly.

### Files Likely To Change

- `/Users/mnutt/p/personal/spktool/internal/cli/cli.go`
- `/Users/mnutt/p/personal/spktool/internal/cli/cli_test.go`
- `/Users/mnutt/p/personal/spktool/internal/app/app.go`

### Exit Criteria

- CLI tests no longer require one monolithic mock surface
- adding a new VM command does not expand the package/publish/key contract

## Phase 4: Refactor Provider Capabilities

### Objective

Turn the provider abstraction into a smaller required core plus optional
capabilities.

### Proposed capability model

- `ProviderCore`
  `Name`, `Up`, `Halt`, `Destroy`, `Provision`, `Status`
- `CommandExecutor`
  `Exec`, `ExecInteractive`, `SSH`
- `BootstrapRenderer`
  `BootstrapFiles`, `WriteFile`, `DetectInstanceName`
- `GrainManager`
  `ListGrains`, `AttachGrain`

The exact names can change, but the split should reflect real usage patterns.

### Why this helps

- providers can be tested against the capabilities they actually implement
- grain-specific behavior stops shaping the entire provider abstraction
- future backends have a clearer minimal contract

### Work

- map current service usage to capability groups
- identify which interfaces are truly required for every backend
- add shared provider contract tests for the core VM lifecycle behavior
- keep a compatibility adapter during the transition if needed

### Files Likely To Change

- `/Users/mnutt/p/personal/spktool/internal/providers/provider.go`
- `/Users/mnutt/p/personal/spktool/internal/providers/lima/provider.go`
- `/Users/mnutt/p/personal/spktool/internal/providers/vagrant/provider.go`
- services and tests that currently depend on the monolithic provider interface

### Exit Criteria

- the required provider contract is materially smaller than today
- grain support is optional from the perspective of the core VM abstraction

## Phase 5: Re-Scope The Workflow Layer

### Objective

Decide whether `internal/workflow` should become a stronger orchestration tool
or stay a minimal structured helper.

### Decision path

#### If keeping it minimal

- document it as a step runner with rollback
- stop describing it as a broad workflow engine
- use it only where rollback semantics are meaningful

#### If making it stronger

- add explicit support for shared step state or typed execution context
- define how rollback errors are surfaced to callers and logs
- standardize workflow usage across `setupvm`, VM creation/provisioning, and any
  other multi-step operations that need the same guarantees

### Recommendation

Default to the minimal path unless a concrete upcoming workflow needs richer
state handling. The current implementation is small and understandable; it does
not need to become more abstract unless the code genuinely demands it.

### Files Likely To Change

- `/Users/mnutt/p/personal/spktool/internal/workflow/workflow.go`
- docs in `/Users/mnutt/p/personal/spktool/docs/architecture.md`
- any services currently using workflow for simple linear sequencing

### Exit Criteria

- the docs and implementation describe the same thing
- rollback behavior and rollback failures are visible enough for operators and
  tests

## Phase 6: Documentation Cleanup

### Objective

Bring the repository narrative back in sync with the code after the refactor.

### Work

- update `/Users/mnutt/p/personal/spktool/README.md`
- update `/Users/mnutt/p/personal/spktool/docs/architecture.md`
- add a short section explaining the new service/provider boundaries
- note any intentional compatibility choices that still exist only for the CLI
  edge

### Exit Criteria

- the docs describe the actual layering, not the previous target state

## Suggested Milestones

### Milestone 1

- baseline tests tightened
- `ProjectService` split into 2-3 smaller units

### Milestone 2

- CLI boundary narrowed
- provider capabilities split with compatibility adapters removed or minimized

### Milestone 3

- workflow stance decided
- docs updated
- full test suite green

## Validation Checklist

- `GOCACHE=/tmp/go-build go test ./...`
- targeted acceptance checks for Lima and Vagrant still pass when intentionally
  enabled
- `setupvm`, `config render`, `vm create`, `init`, and `enter-grain` behavior
  remain stable from the operator’s perspective

## Non-Goals

- redesigning the user-facing CLI
- adding new providers during the refactor
- introducing a generic plugin system beyond the current in-process provider
  model
- replacing embedded templates or the checked-in config model

## Recommendation Summary

The highest-value move is to split the service layer first. The CLI and provider
problems are real, but both are easier to address once responsibilities are no
longer concentrated in one service file. The workflow decision should come
after that, not before.
