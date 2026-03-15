# Next Steps

## Current Baseline

The repo now has:

- `/.sandstorm/box.toml` as checked-in project config
- `/.sandstorm/box.local.toml` as local override config
- generated provider/runtime artifacts under `/.sandstorm/.generated/`
- Vagrant and Lima configs rendered from resolved config
- streamed provider startup output
- Lima serial boot tailing behind `--verbose`

The remaining work is mostly cleanup, validation, and real-world verification.

## Progress Update

Completed:

- `config render` is implemented for provider artifact inspection.
- Lima end-to-end verification was run against a real Lima 2.0.3 instance.
- `README.md` and `docs/architecture.md` were updated to reflect the config
  ownership model.
- A separate opt-in Go acceptance lane now exists for real Lima lifecycle
  testing.

Lima verification found and fixed:

- Lima QEMU now renders `mountType: reverse-sshfs` so Debian 12 mounts work on
  Lima 2.0.x.
- Lima provisioning now targets the existing instance instead of rerunning
  `limactl start`.
- shared guest setup logic no longer assumes a `vagrant` user exists.

Still outstanding:

- no blocking engineering work remains in this migration plan

## Recommended Order

1. Manual follow-ups only as needed

## Concrete Checklist

### 1. Add `config render`

Status:
- Done

Goal:
- Make generated provider config inspectable without encouraging edits to generated files.

Commands to add:
- `spktool config render --provider lima`
- `spktool config render --provider vagrant`
- optional: `spktool config show`

Files to touch:
- `/Users/mnutt/p/personal/spktool/internal/cli/cli.go`
- `/Users/mnutt/p/personal/spktool/internal/cli/cli_test.go`
- `/Users/mnutt/p/personal/spktool/internal/services/services.go`

Implementation notes:
- Reuse the existing config load/resolve path.
- Return rendered `lima.yaml` / `Vagrantfile` and maybe `runtime.env`.
- Do not require the VM to exist.

Definition of done:
- You can inspect resolved provider config without opening `/.sandstorm/.generated/*`.

### 2. Manually verify Lima end-to-end

Status:
- Done, with one remaining host-side reachability caveat in the sandboxed shell

Goal:
- Confirm the new config/render/provision path works against a real Lima instance.

Test flow:
1. `spktool setupvm node`
2. `spktool vm create`
3. `spktool init`
4. Check that Sandstorm is reachable on the configured external port
5. Confirm host/origin behavior matches the configured port

Specific things to verify:
- `/.sandstorm/.generated/lima.yaml` is valid for the installed Lima version
- `/.sandstorm/.generated/runtime.env` is consumed correctly
- `global-setup.sh` updates Sandstorm config as expected
- disabling auto port forwarding actually suppresses the extra port noise after recreating the instance

Observed results:
- `setupvm`, `config render`, `vm create`, and `init` all succeeded
  against a real Lima 2.0.3 instance after the fixes above.
- Sandstorm was active in the guest and listening on `0.0.0.0:6090`.
- Lima hostagent logs showed the host-side forward for port `6090` being
  created.
- Host-side `curl` from the sandboxed shell remained inconclusive, so browser
  reachability still deserves a non-sandbox manual check.

Potential files to inspect if something fails:
- `/Users/mnutt/p/personal/math-fun2/.sandstorm/.generated/lima.yaml`
- `/Users/mnutt/p/personal/math-fun2/.sandstorm/.generated/runtime.env`
- `/Users/mnutt/.lima/<instance>/serial*.log`

### 3. Manually verify Vagrant end-to-end

Status:
- Done

Goal:
- Confirm the `.generated` working-directory move did not break Vagrant assumptions.

Test flow:
1. `spktool setupvm node --provider vagrant`
2. `spktool vm create`
3. `spktool init`
4. Confirm the configured external port works

Specific things to verify:
- Vagrant runs correctly from `/.sandstorm/.generated/`
- `../..` mount paths in the generated Vagrantfile map the project root correctly
- `/host-dot-sandstorm` is still mounted and usable
- `runtime.env` under `/.generated` is visible inside the guest at `/opt/app/.sandstorm/.generated/runtime.env`

Files to inspect:
- `/Users/mnutt/p/personal/spktool/internal/templates/assets/box/Vagrantfile`
- `/Users/mnutt/p/personal/spktool/internal/providers/vagrant/provider.go`

Observed results:
- `setupvm`, `vm create`, and `init` succeeded against a real
  Linux Vagrant/libvirt host after fixing the Node stack reprovision GPG issue.
- Vagrant ran from `/.sandstorm/.generated/` as intended.
- `runtime.env` was visible in the guest at
  `/opt/app/.sandstorm/.generated/runtime.env`.
- `/host-dot-sandstorm` remained mounted and usable inside the guest.
- Sandstorm remained reachable on the configured forwarded port.

### 4. Remove or isolate legacy state fallback

Goal:
- Stop treating `project-state.json` as a normal runtime path.

Current status:
- Done. `project-state.json` and `internal/state` were removed from the runtime
  path and from the repo.

Options:
- Deleted entirely.

Files to touch:
- `/Users/mnutt/p/personal/spktool/internal/services/services.go`
- `/Users/mnutt/p/personal/spktool/internal/domain/state.go`
- `/Users/mnutt/p/personal/spktool/internal/app/app.go`

Recommended approach:
- Done.

Definition of done:
- Completed. `.sandstorm/project-state.json` no longer exists.

### 5. Tighten `setupvm` behavior

Status:
- Done

Goal:
- Define what happens when `box.toml` or `box.local.toml` already exists.

Questions to answer:
- Should `setupvm` fail if `box.toml` exists?
- Should it preserve local overrides?
- Should there be a `--force` mode?

Recommended default:
- Fail if `box.toml` already exists, unless a force/update path is explicitly requested.

Files to touch:
- `/Users/mnutt/p/personal/spktool/internal/services/services.go`
- `/Users/mnutt/p/personal/spktool/internal/cli/cli.go`
- `/Users/mnutt/p/personal/spktool/internal/services/services_test.go`

Definition of done:
- Completed. `setupvm` fails if `box.toml` exists unless `--force` is passed,
  and preserves `box.local.toml` unless force-overwriting.

### 6. Improve config validation

Status:
- Done

Goal:
- Fail early and clearly for bad config.

Add validation for:
- invalid provider names
- invalid or privileged external ports
- unsupported provider-specific values
- missing host/port fields
- bad combinations like unsupported guest/arch defaults

Files to touch:
- `/Users/mnutt/p/personal/spktool/internal/config/config.go`
- optional split: `internal/config/validate.go`
- tests under `internal/config/`

Definition of done:
- Completed. Config errors now point to the bad setting and fail before provider
  commands run.

### 7. Update docs

Status:
- Done

Goal:
- Make the new ownership model obvious.

Docs to update:
- `/Users/mnutt/p/personal/spktool/README.md`
- `/Users/mnutt/p/personal/spktool/docs/architecture.md`

Key points to document:
- `box.toml` is the project source of truth
- `box.local.toml` is local-only
- `/.sandstorm/.generated/*` is disposable
- `--provider` is a per-invocation override
- `--verbose` enables extra Lima boot diagnostics
- provider artifacts should not be edited manually

### 8. Add smoke/integration coverage

Status:
- Done

Goal:
- Add a small amount of real-provider coverage for the risky parts.

Good targets:
- config render path
- generated artifact paths under `/.generated`
- Lima/Vagrant startup command args
- runtime env generation

Current progress:
- provider-level Lima tests now cover the explicit mount type and provisioning
  command shape
- provider-level Vagrant tests now cover generated mount/port rendering plus
  `up` and `provision` command shape from `/.sandstorm/.generated/`
- service-level tests now cover VM lifecycle passthrough, provider overrides,
  and SSH argument/context wiring
- the opt-in Lima acceptance test now verifies `runtime.env` exists after
  `setupvm`, is included in `config render`, and is visible in the guest mount
- an opt-in `go test -tags=acceptance ./internal/acceptance` lane now exists
  for both real Lima and real Vagrant lifecycle tests

Definition of done:
- Completed. The risky config/render/generated-path/lifecycle paths now have
  contract or real-provider coverage on both Lima and Vagrant.

Files to extend:
- `/Users/mnutt/p/personal/spktool/internal/providers/lima/provider_test.go`
- `/Users/mnutt/p/personal/spktool/internal/providers/vagrant/provider_test.go`
- `/Users/mnutt/p/personal/spktool/internal/services/services_test.go`

## Nice-to-Have Follow-Ups

- Add `spktool config validate`
- Add `spktool config show`
- Add better subcommand help for `setupvm --help`
- Revisit Lima defaults on Apple Silicon, especially `arch` and `vmType`
- Decide whether `box.local.toml` should ever be auto-created outside `setupvm`

## Suggested Success Criteria

You are in a good state when:

- a new project can be set up and started on Lima without hand-editing generated files
- a new project can be set up and started on Vagrant without path regressions
- the configured external port is reflected both in provider forwarding and Sandstorm config
- users can inspect rendered config without treating generated files as source
- legacy state is no longer part of the normal execution path
