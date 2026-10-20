<div align="center">

# sharryctl

**Your office app, from the terminal.**

A fast, scriptable command-line client and Go SDK for the [Sharry](https://sharryapp.com) workplace platform.
Manage parking plates, check parking availability, book spots and inspect access badges without opening the phone app.

[![CI](https://github.com/alhafoudh/sharryctl/actions/workflows/ci.yml/badge.svg)](https://github.com/alhafoudh/sharryctl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/alhafoudh/sharryctl)](https://github.com/alhafoudh/sharryctl/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/alhafoudh/sharryctl/sharry.svg)](https://pkg.go.dev/github.com/alhafoudh/sharryctl/sharry)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

```console
$ sharryctl plates list
PLATE    DEFAULT  UUID
BA123AB  yes      c5cb7400-…

$ sharryctl plates change BA123AB --to BA456CD
Changed BA456CD (default) [c5cb7400-…].

$ sharryctl parking status
SCOPE         TOTAL  FREE  RESERVED  OCCUPIED  ALLOCATION
Acme Corp     10     10    0         0         1 guaranteed, 1 bookable, 9 visitor
(whole site)  648    598   50        0         public: 89 free, 0 reserved
```

> [!NOTE]
> **Unofficial.** sharryctl is an independent open-source project and is not affiliated with,
> endorsed by, or supported by Sharry. It talks to the same API as the official mobile app,
> using credentials you provide for a tenant you already have access to. The API is not
> publicly documented and may change without notice.

## Features

- **Any tenant.** Sharry runs a separate deployment per organisation (`<tenant>.sharryapp.com`). Configure as many as you like and switch with `--tenant`.
- **Email-code login** with automatic, transparent token refresh. Tokens are stored per tenant and account with `0600` permissions.
- **License plates**: list, add, change, remove, and set the default plate.
- **Parking**: live availability, reservations, spot recommendations, booking, cancelling, and releasing a spot when you leave early.
- **Access**: badges, card formats, devices, identity verification.
- **`sharryctl api`**: a signed escape hatch for any endpoint, including ones without a dedicated command yet.
- **Built-in Swagger UI** for the bundled OpenAPI spec. A local proxy signs "Try it out" calls, so secrets never touch the browser.
- **Scriptable**: `--json` on every command, stable exit codes, non-interactive login.
- **Go SDK**: the CLI is a thin layer over [`sharry`](sharry), a small dependency-free client package.
- **Claude skill** included, so an AI assistant can drive it for you ("change my parking plate to …").

## Install

**Homebrew** (macOS, Linux)

```sh
brew install alhafoudh/sharryctl/sharryctl
```

**Go**

```sh
go install github.com/alhafoudh/sharryctl/cmd/sharryctl@latest
```

**Binaries**: download from [Releases](https://github.com/alhafoudh/sharryctl/releases) (macOS, Linux, Windows; amd64 and arm64).

## Quick start

### 1. Add your tenant

You need your tenant's **slug** (the subdomain in `https://<slug>.sharryapp.com`) plus its
**Application Token** and **API Secret**. These are tenant-level credentials; this project
does not ship or publish any.

```sh
sharryctl config tenants add acme          # prompts for both secrets without echo
# or, from a local TOML file you keep outside any repo:
sharryctl config tenants add acme --from-file ~/secrets/acme.toml
```

### 2. Log in

```sh
sharryctl login --email you@example.com    # a code is emailed to you; paste it at the prompt
sharryctl me
```

### 3. Use it

```sh
sharryctl plates add BA123AB --default
sharryctl parking recommend --from "tomorrow 09:00" --to 17:00
sharryctl parking reserve   --from "tomorrow 09:00" --to 17:00
```

## Command reference

| Area | Commands |
|---|---|
| **Auth** | `login`, `login resend`, `logout`, `auth status`, `auth refresh`, `auth token`, `auth import`, `me` |
| **Plates** | `plates list`, `plates add <plate> [--default]`, `plates change <plate\|uuid> [--to <plate>] [--default]`, `plates remove <plate\|uuid>` |
| **Parking** | `parking status`, `parking reservations`, `parking recommend`, `parking reserve`, `parking cancel <id>`, `parking release <id>` |
| **Access** | `badges list\|grant\|revoke\|state`, `card-formats list`, `devices state`, `verification submit` |
| **Raw API** | `api <METHOD> <path> [-d json\|@file\|-] [-q key=value]` |
| **OpenAPI** | `swagger serve [--open]`, `swagger validate [file]`, `swagger spec` |
| **Config** | `config tenants list\|add\|use\|show\|remove`, `config path` |

**Global flags:** `--tenant <slug>`, `--profile <email>`, `--json`, `-v/--verbose` (logs requests with secrets redacted).

**Times** (`--from`, `--to`) accept `09:00`, `tomorrow 09:00`, `2026-10-21 09:00`, `now` or ISO 8601.
A bare time for `--to` falls on the same day as `--from`.

**Plates** are normalised: `ba-123 ab` and `BA123AB` are the same plate.

**Exit codes:** `0` success, `1` usage or local error, `2` API error (message on stderr).

### Scripting

```sh
# Non-interactive login (CI, agents)
sharryctl login --email you@example.com --request-only
sharryctl login --email you@example.com --skip-request --code "$CODE"

# JSON everywhere
sharryctl --json plates list | jq -r '.[] | select(.default) | .license_plate'

# Any endpoint, signed and authenticated
sharryctl api GET users/me
```

Secrets can also come from the environment: `SHARRY_TENANT`, `SHARRY_PROFILE`,
`SHARRY_APPLICATION_TOKEN`, `SHARRY_API_SECRET`.

## Configuration

Everything lives in `~/.config/sharry/` (respects `XDG_CONFIG_HOME`; override with `SHARRY_CONFIG_DIR`):

```text
~/.config/sharry/
├── config.toml                       # tenants, 0600
└── profiles/<tenant>/<email>.json    # tokens + cached profile, 0600
```

See [`config.example.toml`](config.example.toml) for every option, including API version,
language and the app identity headers (`Platform`, `App-Version`, `User-Agent`).

## Go SDK

```go
import "github.com/alhafoudh/sharryctl/sharry"

client, err := sharry.NewClient(sharry.Config{
    Tenant:           "acme",
    ApplicationToken: os.Getenv("SHARRY_APPLICATION_TOKEN"),
    APISecret:        os.Getenv("SHARRY_API_SECRET"),
})

// Restore a stored session; refreshed tokens are handed back to you for persistence.
client.SetSession(sharry.NewSession(storedToken, func(t sharry.Token) { save(t) }))

plates, err := client.ListLicensePlates(ctx)
status, err := client.ParkingStatus(ctx, companyUUID)
```

<details>
<summary>How request signing works</summary>

Every request carries the tenant's `Application-Token` and a `Request-Sign` header:

```text
Request-Sign = lowercase_hex( HMAC-SHA256( key = API Secret,
                                           msg = fullURL + "|" + rawBody ) )
```

`fullURL` is the exact URL including the query string; `rawBody` is empty for requests
without a body. Authenticated calls add `Access-Token: <JWT>`. `sharry.Sign` exposes the
signature if you need it in other tooling.

</details>

## OpenAPI and Swagger UI

[`api/openapi.yaml`](api/openapi.yaml) describes the auth flow and every endpoint sharryctl
knows. It is embedded in the binary:

```sh
sharryctl swagger serve --open    # Swagger UI on http://localhost:8088, calls signed by a local proxy
sharryctl swagger validate
```

The proxy listens only on `127.0.0.1` and rejects non-localhost `Host` headers.

## Claude skill

[`skill/sharryctl`](skill/sharryctl) teaches Claude to use sharryctl: checking setup, the
two-step email login, plates, parking and badges, while keeping secrets out of the chat.

- **Claude Code:** `ln -s "$PWD/skill/sharryctl" ~/.claude/skills/sharryctl`
- **Claude Desktop:** zip the `skill/sharryctl` folder and upload it in Settings, Capabilities, Skills.

## Security

- Tenant credentials and user tokens are only ever stored locally, with `0600` permissions.
- `--verbose` logging redacts tokens; `config tenants show` masks secrets.
- Never commit your `config.toml` or token files. This repository contains no tenant credentials.

Found a security issue? Please report it privately through
[GitHub security advisories](https://github.com/alhafoudh/sharryctl/security/advisories/new)
instead of opening a public issue.

## Development

```sh
mise install          # Go + Ruby toolchains from .mise.toml
mise run test         # go test ./...
mise run build        # bin/sharryctl
```

```text
cmd/sharryctl/      entrypoint
sharry/             Go SDK: transport, signing, auth, models
internal/cli/       cobra commands
internal/config/    config + token store
internal/swagger/   Swagger UI server + signing proxy
api/                OpenAPI spec (embedded)
skill/              Claude skill
```

Releases are cut by pushing a `v*` tag. GoReleaser builds the binaries and updates the
[Homebrew tap](https://github.com/alhafoudh/homebrew-sharryctl).

Contributions are welcome, especially typed models for endpoints that still pass raw JSON
(badges, devices, verification). Please include tests that run against `httptest`, never a
live tenant.

## License

[MIT](LICENSE). Bundles [Swagger UI](https://github.com/swagger-api/swagger-ui) (Apache-2.0,
see [`internal/swagger/ui/LICENSE`](internal/swagger/ui/LICENSE)).
