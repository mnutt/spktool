# Auth

Use this reference for user identity, anonymous access, permissions, and roles.

HTTP headers added by `sandstorm-http-bridge` commonly include:

- `X-Sandstorm-User-Id`: stable per-user identifier for logged-in users. Use this as the primary key for identity when needed.
- `X-Sandstorm-Username`: display name. Treat it as presentation data, not a stable identifier.
- `X-Sandstorm-Permissions`: comma-separated permissions granted for the current grain.
- `X-Sandstorm-Preferred-Handle`: mutable, non-unique handle hint. Do not use for security decisions.
- `X-Sandstorm-User-Picture`: avatar URL.
- `X-Sandstorm-User-Pronouns`: preferred pronouns.
- `X-Sandstorm-Tab-Id`: per-tab identifier useful for correlating requests.

Behavioral rules:

- Sandstorm handles authentication and authorization decisions outside the app.
- The app should enforce the permissions it receives but should not implement its own login flow.
- Anonymous visitors may arrive through sharing links. They should get the permission level granted by the link even without a logged-in user ID.
- If the app keeps an internal user model for legacy reasons, map Sandstorm users into it automatically.

Permissions and roles:

- Permissions are app-defined capability bits exposed through the package definition.
- Roles are human-friendly bundles of permissions chosen in the Sandstorm sharing UI.
- The grain owner effectively has every permission.
- Later app versions may add permissions or roles, but removing old entries is risky for compatibility. Prefer marking obsolete items obsolete rather than deleting them.

Implementation guidance:

- Check `X-Sandstorm-Permissions` on every request that mutates or reveals protected state.
- Use `X-Sandstorm-User-Id` as the durable identity key whenever a stable user key is required.
- Treat display names, handles, pictures, and pronouns as user-profile hints that can change over time.
