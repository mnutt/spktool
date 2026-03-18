# Paths And Titles

Use this reference for grain URLs, routing, browser title sync, sharing UI hooks, and base URL handling.

URL model:

- Users see top-level grain URLs like `/grain/<grainId>`.
- The actual app content runs inside an iframe on a random per-session subdomain.
- Sharing links use `/shared/<token>` but otherwise follow the same routing model.
- Pathnames and fragments appended to the grain URL are passed through to the app.

Title and path synchronization:

- Because the app runs in an iframe, browser URL and title do not update automatically as the app navigates.
- To push the current path and hash to Sandstorm, use `window.parent.postMessage({setPath: location.pathname + location.hash}, "*")`.
- To push the current document title, use `window.parent.postMessage({setTitle: document.title}, "*")`.
- If the app needs the current grain title, request it through `postMessage()` and handle the async response from the parent frame.

Sharing UI integration:

- To open Sandstorm's sharing dialog, send `window.parent.postMessage({startSharing: {}}, "*")`.
- To include a path or hash in the shared link, pass `pathname` and `hash` in `startSharing`.
- To show who has access, send `window.parent.postMessage({showConnectionGraph: {}}, "*")`.

Base URL guidance:

- Prefer relative URLs or the empty string as the app base URL whenever the framework permits it.
- If the app needs an absolute base URL on each request, read `X-Sandstorm-Base-Path`.
- `X-Sandstorm-Base-Path` includes scheme and host, has no trailing slash, and can change between requests.
- Do not cache the base path globally.

Other request metadata:

- `Host` and `X-Forwarded-Proto` are available for normal browser requests.
- API requests behave differently and may omit or normalize some of these values.
