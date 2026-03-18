# Powerbox

Use this reference when the app needs user-mediated access to another grain or an external capability.

Mental model:

- A grain starts isolated.
- To connect it to anything else, ask through the Powerbox.
- The app requests a type of resource, not a specific instance of one.
- The user chooses which matching resource to grant.

Browser-side request flow:

- Construct one or more Powerbox descriptors representing acceptable APIs.
- Send them to the parent frame with `window.parent.postMessage({powerboxRequest: {...}}, "*")`.
- Include a stable `rpcId`, a `query` array of descriptor strings, and a `saveLabel` describing the resulting connection.
- Listen for the matching response message from the parent frame.

Result handling:

- If the user cancels, no connection is made.
- If accepted, the browser receives a claim token.
- Send that claim token to the app server.

Server-side claim flow with `sandstorm-http-bridge`:

- Redeem the claim token by POSTing to `http://http-bridge/session/<session-id>/claim`.
- Use the current `X-Sandstorm-Session-Id` value for the session.
- Provide `requiredPermissions` so the connection is revoked automatically if the requesting user later loses the needed access.
- The response returns a durable capability token that the app can use through the bridge proxy.

Implementation notes:

- Many HTTP client libraries will honor `HTTP_PROXY` and `http_proxy` set by the bridge.
- If the app needs non-HTTP capabilities, it will have to use the raw Cap'n Proto APIs instead of only the HTTP bridge helpers.
