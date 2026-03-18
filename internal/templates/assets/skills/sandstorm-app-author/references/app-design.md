# App Design

Use this reference when deciding whether an app fits Sandstorm well and how to adapt its UX.

Key product assumptions:

- A Sandstorm app usually provides a personal network service to the person who creates a grain.
- A new grain should be usable immediately. The first screen should let the user start the core task without setup friction.
- A grain should represent a unit of sharing: one document, one blog, one album, one repo, or another independently shareable object.
- The app should not invent its own login or ACL model. Sandstorm handles identity and sharing.
- Persistent app data belongs under `/var`.

Good adaptation patterns:

- Collapse multi-tenant concepts into per-grain state where possible.
- Bundle dependencies the app needs instead of assuming external shared services.
- If the original app has a users table, auto-provision local user records from Sandstorm identity headers rather than prompting for signup.
- Choose the smallest practical grain size so sharing can be delegated to Sandstorm instead of app-defined authorization logic.

Warnings:

- Do not treat a grain like a whole hosted service with many unrelated workspaces inside it unless that is truly the smallest shareable unit.
- Do not require initial account setup before the user can do useful work.
- Do not rely on long-running app processes staying alive after the browser tab closes. (if you absolutely must do this, use the stay-awake util)
