---
name: sandstorm-app-author
description: Guidance for app authors packaging, integrating, debugging, and publishing Sandstorm apps. Use when creating or updating a Sandstorm app package, editing sandstorm-pkgdef.capnp, wiring Sandstorm auth or permissions, using powerbox or HTTP APIs, debugging dev-mode grains, or preparing an app for the Sandstorm app market.
allowed-tools: Bash(spktool:list-utils) Bash(spktool:describe-util:*) Bash(spktool:add:*) Bash(spktool:install-skills) Bash(spktool:config:render) Bash(spktool:vm:status)
---

# Sandstorm App Author

Use this skill when the user is building, packaging, debugging, integrating, or publishing a Sandstorm app.

Prefer `spktool` over legacy `vagrant-spk` or `lima-spk`.

## Default assumptions

- Most projects should use `spktool` from the project root.
- `.sandstorm/box.toml` is the checked-in source of truth for stack, networking, and provider defaults.
- `.sandstorm/box.local.toml` is for machine-local overrides such as provider choice.
- Unless the user explicitly asks otherwise, do not install system-wide packages on the host and do not run the app on the host outside the VM.
- Most apps use `sandstorm-http-bridge`.
- App data should live under `/var`.
- Sandstorm handles authentication. The app usually should not implement its own login flow.

## Workflow

1. Identify the task:
   - Bootstrapping a new app from an empty repo
   - Packaging a new or existing app
   - Editing `sandstorm-pkgdef.capnp`
   - Integrating auth, permissions, HTTP APIs, web publishing, or powerbox
   - Debugging a dev-mode grain
   - Preparing app-market metadata or publication

2. Route to the right reference:
   - New app from an empty repo: `references/new-app-workflow.md`
   - Packaging or project setup: `references/packaging.md`
   - Product and UX fit for Sandstorm: `references/app-design.md`
   - Identity, permissions, and roles: `references/auth.md`
   - Grain URLs, titles, sharing, and base paths: `references/path.md`
   - HTTP API export and tokens: `references/http-apis.md`
   - Static web publishing: `references/web-publishing.md`
   - Powerbox requests and capability flow: `references/powerbox.md`
   - Inbound and outbound email: `references/email.md`
   - Platform model and sandbox behavior: `references/platform-architecture.md`
   - App market and release metadata: `references/publishing.md`

## spktool baseline

For `spktool`-managed projects, prefer these commands:

- Initial setup: `spktool setupvm <stack> --provider lima|vagrant`
- Refresh generated project files: `spktool upgradevm`
- Preview generated provider/runtime files: `spktool config render`
- Create and provision the VM: `spktool vm create`
- Start an existing VM: `spktool vm up`
- Re-run guest provisioning: `spktool vm provision`
- Open a guest shell: `spktool vm ssh`
- Initialize packaging metadata: `spktool init`
- Run a dev session: `spktool dev`
- Build a package: `spktool pack <output.spk>`
- Verify a package: `spktool verify <input.spk>`
- Publish a package: `spktool publish <input.spk>`

If `.sandstorm/box.toml` does not exist yet, bootstrap the project first with `spktool setupvm <stack> --provider lima|vagrant` before attempting `spktool init`, `spktool dev`, or packaging work.

For a brand-new app, the usual order is: `setupvm`, edit `box.toml` and runtime scripts, `vm create`, `init`, `dev`, inspect package inclusion, then `pack`.

Do not hand-edit managed files under `.sandstorm/.generated/`. Edit `box.toml`, `box.local.toml`, `setup.sh`, `build.sh`, and `launcher.sh`, then run `spktool upgradevm`. If guest software or services changed, follow with `spktool vm provision`.
