# Security

This page covers how Pulse handles authentication, sessions, two-factor authentication, password recovery, and API keys.

## Password hashing

Admin passwords are hashed with Argon2id. The plaintext password is never stored.

## Sessions

Login uses a username and password. On success, Pulse sets an HttpOnly session cookie. The HttpOnly flag prevents client-side scripts from reading the cookie.

## Two-factor authentication (TOTP)

Pulse supports optional time-based one-time password (TOTP) two-factor authentication with an authenticator app.

- **Enable.** In the admin settings, start TOTP enrollment, scan the code with an authenticator app, and confirm with a generated code. After confirmation, login requires the password and a current code.
- **Disable.** In the admin settings, turn TOTP off. After disabling, login requires only the password.

## Resetting the admin password

If you are locked out, reset the admin password from the command line with the `reset-password` subcommand:

```bash
pulse reset-password
```

Run this against the same data directory and SQLite file the server uses so the change applies to the running configuration.

## API key handling

API keys are named and scoped. See [API](api.md) for the scope list and Bearer usage. Key handling has these properties:

- The full key is shown once, at creation. Save it then.
- Only a hash of the key is stored. Pulse cannot show the full key again.
- Keys are revocable. Revoke a key to invalidate it immediately.
- Each key carries only the scopes you assign, so you can grant least privilege for a given integration.
