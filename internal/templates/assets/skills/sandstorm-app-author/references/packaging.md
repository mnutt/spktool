# Packaging

Use these docs first for packaging and project setup work:

- `spktool --help`: current command surface
- `spktool config render`: preview generated provider artifacts before reprovisioning

Interpretation guidance:

- Prefer `spktool` over `vagrant-spk` when turning old Sandstorm packaging workflows into real commands.
- For bootstrapped projects, treat `.sandstorm/box.toml` as the checked-in project source of truth.
- Use `spktool vm create` for first boot and provisioning. Use `spktool vm up` only for an existing VM.
- Use `spktool vm halt`, `spktool vm destroy`, and `spktool vm status` for VM lifecycle work outside create/up/provision flows.
- Run `spktool init` from the project root to initialize packaging metadata.
- `spktool dev` and packaging workflows only include files exercised during development and testing.
- Key management uses `spktool keygen`, `spktool listkeys`, and `spktool getkey`.
- Grain debugging can use `spktool enter-grain`.
- Agent skill installation uses `spktool install-skills --codex` and/or `spktool install-skills --claude`.
- Utility discovery and installation use `spktool list-utils`, `spktool describe-util <name>`, and `spktool add <name>`. Example utilities include `get-public-id` and `stay-awake`.

Key project files in `spktool` projects:

- `.sandstorm/box.toml`
- `.sandstorm/box.local.toml`
- `.sandstorm/setup.sh`
- `.sandstorm/build.sh`
- `.sandstorm/launcher.sh`
- `.sandstorm/global-setup.sh`
- `.sandstorm/utils.toml`
- `.sandstorm/utils/*`
- `.sandstorm/.generated/*`

## Managed Vs Editable Files

- Treat `.sandstorm/.generated/*` as output, not source of truth.
- Edit packaging metadata and runtime scripts, not generated artifacts.
- After changing `box.toml`, `box.local.toml`, `setup.sh`, `build.sh`, or `launcher.sh`, regenerate with `spktool upgradevm`.
- If the regeneration changes guest dependencies or services, reprovision with `spktool vm provision`.

## Packaging Validation Checkpoint

After `spktool dev`, validate package inclusion explicitly:

- Inspect the generated package file list or other package metadata that controls inclusion.
- Verify that runtime-critical source files, built assets, dependencies, helper binaries, and configuration are actually captured.
- Do not assume development tracing found everything.
- If required runtime artifacts are missing, add explicit inclusion rules in package metadata before building the final package.

## Triage Model

Before changing app code, separate the failure domain:

- guest provisioning: setup scripts, installed packages, services, permissions inside the VM
- package inclusion: missing files or metadata errors in the built package
- grain startup or runtime: process launch, runtime config, app crashes, bad assumptions about `/var`
- host or provider access: VM health, port forwarding, DNS, provider tooling, host networking

Ask which layer is failing first, then change the narrowest control surface that explains the failure.

## External Dependencies And Environment

Some `spktool` operations depend on environment health rather than app logic:

- network access for downloads, publishing, or utility catalog lookups
- VM creation and provisioning time
- provider health for Lima or Vagrant
- host DNS, ports, and forwarding behavior

When a step fails, check whether the failure is environmental before treating it as an app-authoring problem.
