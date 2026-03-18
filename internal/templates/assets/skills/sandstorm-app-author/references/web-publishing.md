# Web Publishing

Use this reference when the app publishes static content to the public web.

What this feature does:

- Sandstorm can serve static files on behalf of the app.
- Published content can appear on a Sandstorm-managed hostname or on a user-controlled custom domain.
- This feature is for static content, not arbitrary dynamic site hosting.

Filesystem layout:

- Put published files under `/var/www`.
- Each directory intended to be browsable should contain an `index.html`.

Publishing flow:

- The grain obtains a `publicId`.
- Sandstorm serves the static files at a hostname derived from that `publicId`.
- The user can point a custom domain at the Sandstorm server with DNS.
- The user also adds a TXT record that maps the custom domain to the `publicId`.

App-author guidance:

- Show the user the generated preview URL.
- Explain the required DNS records clearly: a CNAME to the Sandstorm host and a TXT record carrying the `publicId`.
- Treat this feature as provisional and a little awkward; keep the user flow explicit.

Constraints:

- Static publishing avoids waking the app for every public request.
- Dynamic custom-domain hosting is not the intended use here.
- If you need public interactivity, combine published static assets with API calls rather than assuming a fully dynamic public app endpoint.
