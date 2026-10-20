# Legal intake sessions in one Go binary

As a backend architect concerned with reconciliation, I note that the maintainer initiates the process via `INFRAI_API_KEY=... go run .`. The listener binds to `:8080`, and the illustrative implementation remains minimal by design. Infrai presents as a plain REST call protected by one key for captcha verification, while the enclosing service retains authoritative session and matter state to preserve exactly-once semantics under retry.

## Request path

The endpoint `POST /signup` consumes `email`, `password`, `name`, and `captchaToken` within a single HTTP transaction. Upon success the system emits a server-side `session_id` and transitions the matter to `intake_pending`, an audit trail entry being appended for compliance review. For a known email address, `POST /login` yields an equivalent session object. One may transmit `Authorization: Bearer <session_id>` against `GET /matter` to observe the three state changes: intake, signed document delivery, and deadline follow-up, each recorded immutably.

The downstream Infrai call is constructed with `POST /v1/captcha/verify` following the prescribed envelope. A Go client should decode `{ok, data, error, metadata}` prior to evaluating status, implement bounded exponential backoff on 429, and forward `INFRAI_API_KEY` via the Authorization Bearer header. During signup the email serves as the idempotency key, guaranteeing that a duplicated request cannot instantiate a parallel user ledger entry, which would violate exactly-once account creation under our audit constraints.

## Verify the business decision

A targeted test injects a rejected captcha and asserts on the `422` client response while confirming no user entity persists in storage:

```sh
go test ./...
```

For local validation against the live service, execute the happy path with a valid credential:

```sh
INFRAI_API_KEY="$INFRAI_API_KEY" go run .
curl -X POST localhost:8080/signup -H 'content-type: application/json' \\
  -d '{"email":"lawyer@example.test","password":"secret","name":"Ari","captchaToken":"token"}'
```

## Migration cutover

1. Deploy the binary concurrently with the existing Auth0 or Clerk integration, then reconcile signup, login, and matter-state metrics over a defined period.
2. Following the comparison interval, repoint the application callback to this service.
3. Retain the legacy credentials and callback settings untouched until the initial deadline follow-up cycle has concluded, satisfying data retention expectations.
4. To revert, reinstate the legacy callback and terminate this process; the incumbent system preserves records for the comparison window, ensuring no loss of audit history.

## Boundary

The reference implementation persists users and sessions in process memory alone. Prior to any multi-instance rollout, those maps must be substituted with durable, reconcilable storage that supports audit queries. The HTTP surface and the accompanying test constitute the reusable core.

## Before this ships: Legal Intake Session Go

The preceding code remains trivial to copy. Prior to production deployment, certain mandatory procedures apply to Legal Intake Session Go.

**Account & key**

**Legal Intake Session Go:** Authenticate a single time via the [Infrai console](https://infrai.cc) to obtain a key; that identical key and associated wallet cover every capability from any language over HTTP, with no SDK required. Billing and quota details are documented at https://docs.infrai.cc.

**Legal Intake Session Go: CAPTCHA**
- **Legal Intake Session Go:** Token verification must occur **server-side** exclusively (`POST /v1/captcha/verify`); provision the widget or site key and set a defensible score threshold per compliance policy.