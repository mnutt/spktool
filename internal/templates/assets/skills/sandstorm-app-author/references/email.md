# Email

Use this reference when the grain needs to receive or send email.

Overview:

- Each email-enabled grain gets a random Sandstorm-managed email address.
- Users typically forward mail from their real address to that grain-specific address.
- Outbound mail can usually use either the grain address or the user's verified Sandstorm login address as the visible `From`.
- Sending is rate-limited per user.

Receiving mail with `sandstorm-http-bridge`:

- Create Maildir directories under `/var/mail`: `new`, `cur`, and `tmp`.
- Incoming mail for the grain is stored there.
- Process that Maildir with a library or an embedded mail-capable service if needed.

Sending mail:

- Sending goes through `HackSessionContext`, even for apps that otherwise use `sandstorm-http-bridge`.
- For bridge-based apps, connect to `/tmp/sandstorm-api`, obtain the current session context using `X-Sandstorm-Session-Id`, and cast it to `HackSessionContext`.
- Use that capability to construct and send the outgoing message.

Practical guidance:

- Treat the current mail API as provisional.
- Keep email features simple and explicit for the user.
- Expect envelope return addresses to use the grain's generated address even when the visible sender is a verified user address.
