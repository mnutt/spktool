# New App Workflow

Use this reference when starting a Sandstorm app from an empty repo or an app that has not yet been bootstrapped with `spktool`.

Bootstrap rule:

- If `.sandstorm/box.toml` does not exist, this is not yet a bootstrapped `spktool` project.
- Start with `spktool setupvm <stack> --provider lima|vagrant` before trying `spktool init`, `spktool dev`, or package builds.

Canonical first-run sequence:

1. Bootstrap the project with `spktool setupvm <stack> --provider lima|vagrant`.
2. Edit the user-owned source of truth: `.sandstorm/box.toml`, `.sandstorm/box.local.toml` if needed, `.sandstorm/setup.sh`, `.sandstorm/build.sh`, and `.sandstorm/launcher.sh`.
3. Re-render after source-of-truth changes with `spktool upgradevm`.
4. Create and provision the guest with `spktool vm create`.
5. Initialize package metadata with `spktool init`.
6. Run a development session with `spktool dev`.
7. Inspect package file inclusion before trusting the package.
8. Build the package with `spktool pack <output.spk>`.

Iteration rule:

- If guest software, service wiring, or generated provider config changes after editing the source-of-truth files, rerun `spktool upgradevm` and then `spktool vm provision`.

Early validation checkpoints:

- Confirm the app boots in dev mode before debugging packaging.
- After `spktool dev`, inspect the generated package file list and verify runtime-critical files are included.
- Do not assume tracing captured all application sources, dependencies, built assets, or helper binaries.
- If required runtime artifacts are missing, add explicit package inclusion rules before building the final package.

Failure isolation:

- Separate bootstrap and provisioning failures from app-runtime failures.
- Separate package-inclusion failures from host or provider failures.
- Ask whether the current problem is in setup, guest provisioning, package contents, grain startup, or host access before changing app code.
