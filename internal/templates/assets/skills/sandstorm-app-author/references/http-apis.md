# HTTP APIs

Use this reference when exposing a grain's HTTP API to scripts, mobile clients, CLIs, or external services.

Overview:

- Sandstorm can expose an app API on stable API hostnames rather than the grain iframe's ephemeral domain.
- Sandstorm validates the API token before forwarding the request to the app.
- The app receives a normal request with Sandstorm identity and permission headers, not the raw token.

Package setup:

- Configure API exposure through `apiPath` in `sandstorm-pkgdef.capnp`.
- `""` disables API access.
- `"/"` exposes the whole app.
- Any other prefix limits which paths are reachable on the API hostname, but do not treat that prefix as a security boundary.

Security model:

- API tokens are capability grants, effectively similar to sharing tokens.
- Do not rely on `apiPath` to reduce authority. Use role assignment on the token itself to limit permissions.
- Prefer Bearer auth over HTTP Basic auth.

Token creation:

- Prefer offer templates rendered in the client through `postMessage({renderTemplate: ...})`.
- Offer templates let Sandstorm embed `$API_HOST`, `$API_TOKEN`, and `$GRAIN_TITLE_SLUG` into UI controlled by the app without exposing the raw token to app JS.
- Use `roleAssignment` to mint read-only or otherwise limited tokens.
- Tokens created through offer templates expire quickly unless refreshed by the Sandstorm shell.

Client access:

- Use `Authorization: Bearer <token>` for normal HTTP requests.
- For WebSockets, prefix the request path with `/.sandstorm-token/<token>`.
- Token-specific API hostnames are easier to work with than the generic API hostname, especially with Basic auth.

Response behavior:

- Sandstorm adds permissive CORS for API hosts.
- Sandstorm adds a restrictive `Content-Security-Policy` so API hosts stay isolated if opened in a browser.
