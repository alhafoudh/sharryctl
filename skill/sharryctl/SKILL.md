---
name: sharryctl
description: Drive the Sharry workplace platform (any tenant) through the sharryctl CLI - log in with an emailed code, list/add/change/remove parking license plates, view the profile, list and manage access badges, card formats, devices, verification, and call any API endpoint with correct request signing. Use when the user mentions Sharry, sharryapp.com, their workplace/office app, parking plates (SPZ, ŠPZ, EČV, license plate, registration number), access badges or cards, or asks to change their car's plate for office parking.
---

# sharryctl

`sharryctl` is a CLI for the Sharry workplace API. Each tenant is a separate deployment
(`https://<tenant>.sharryapp.com`). Nothing is tied to one tenant: the tenant always comes
from config, `--tenant` or `$SHARRY_TENANT`.

## Before anything

Run `scripts/check.rb`. It reports whether the binary is installed, which tenants are
configured and which profiles are logged in, as JSON. Act on what it reports:

- binary missing: tell the user to `brew install alhafoudh/sharryctl/sharryctl`.
- no tenant: ask the user for the tenant slug, Application Token and API Secret, then
  have them run `sharryctl config tenants add <slug>` themselves (secrets are prompted
  without echo). Never put secrets in a command line you run, a file or the chat transcript.
- no profile: run the login flow below.

Always pass `--json` when you will parse output.

## Login (two steps, the code arrives by email)

1. Request the code: `scripts/login.rb request <addr>` (runs `sharryctl login --request-only`).
2. Ask the user for the code from their email.
3. Submit: `scripts/login.rb submit <addr> <code>` (runs `sharryctl login --skip-request`,
   passing the code over stdin).

The token endpoint is rate limited (~10 requests per window). Do not retry login in a loop.
Tokens refresh automatically afterwards.

## License plates

```sh
sharryctl --json plates list
sharryctl plates add BA123AB [--default]
sharryctl plates change <uuid|current plate> --to BA456CD      # change the number
sharryctl plates change <uuid|current plate> --default          # make default
sharryctl plates remove <uuid|plate>
```

Plates are normalized (uppercase, spaces and dashes removed). Confirm with the user before
`change` or `remove`, and show `plates list` afterwards.

## Parking

```sh
sharryctl --json parking status                     # read-only
sharryctl --json parking reservations               # read-only
sharryctl --json parking recommend --from "tomorrow 09:00" --to 17:00
sharryctl parking reserve --from "tomorrow 09:00" --to 17:00 [--spot <uuid>]
sharryctl parking cancel <reservation-uuid>
sharryctl parking release <reservation-uuid>        # leaving early: frees the spot from now
```

`reserve` books a real spot and uses the default plate; confirm the window with the user
first. Write calls are rate limited to about 60 per window.

## Other commands

| Task | Command |
|---|---|
| Profile | `sharryctl --json me` |
| Session state | `sharryctl --json auth status` |
| Badges | `sharryctl --json badges list`, `badges grant --data @f.json`, `badges revoke <id>`, `badges state <id> <state>` |
| Card formats | `sharryctl --json card-formats list --site-id <id> [--type <t>]` |
| Device state | `sharryctl devices state <device> --data @f.json` |
| Verification | `sharryctl verification submit --data @f.json` |
| Anything else | `sharryctl --json api <METHOD> <path> [-d json] [-q k=v]` |
| Explore API | `sharryctl swagger serve --open` (user-facing, long running) |

`badges`, `card-formats`, `devices` and `verification` default `--user` to the logged-in
user's id. Request bodies for those are not fully documented; inspect `sharryctl swagger spec`
and ask the user before sending writes.

## Multiple tenants or accounts

Add `--tenant <slug>` and/or `--profile <email>` to any command. `sharryctl config tenants list`
shows what exists.

## Errors

Exit code 2 means the API returned an error (message on stderr). `not logged in` means run the
login flow. HTTP 429 means wait before retrying.
