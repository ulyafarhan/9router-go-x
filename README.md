<div align="center">

![9router-go](docs/logo.svg)

# 9router-go-x

Personal fork of [luqman-v1/9router-go](https://github.com/luqman-v1/9router-go), a Go and Svelte port of [decolua/9router](https://github.com/decolua/9router).

One Go binary with an embedded Svelte dashboard for routing Claude Code, Codex, Cursor, Cline, and other CLI tools across OpenAI, Anthropic, Gemini, and Ollama-compatible providers, with RTK prompt compression and multi-account fallback.

[![License](https://img.shields.io/github/license/ulyafarhan/9router-go-x)](LICENSE)
[![Upstream Go](https://img.shields.io/badge/upstream-luqman--v1%2F9router--go-blue)](https://github.com/luqman-v1/9router-go)
[![Original JS](https://img.shields.io/badge/original-decolua%2F9router-black)](https://github.com/decolua/9router)

[What changed](#what-changed-in-this-fork) • [Quick start](#quick-start) • [Key features](#key-features) • [Share DB](#sharing-a-database-with-9router) • [Setup guide](#setup-guide) • [Credits](#credits)

</div>

## What changed in this fork

Branched off `luqman-v1/9router-go` (`v1.9.17-exp.2`), keeping compatibility with `~/.9router/db/data.sqlite`:

- Fixed tight padding, uneven margins, and cramped layouts across the Svelte dashboard (`Usage`, `Console Log`, `Providers`, `Endpoint`, `Combos`, and `Quota`).
- Fixed the `Console Log` live stream, `Usage` chart and table interactions, and several OAuth and provider routing bugs.
- Added `Task Persona` (`internal/persona`, `PersonaView.svelte`), `context_rescue`, `loopdetect`, and `weblanes`.
- Pointed `LICENSE`, repository links, and self-updater config to `ulyafarhan/9router-go-x`.

I have not published pre-built release binaries for `9router-go-x` yet, so build from source below if you want to run these changes.

## Why 9router-go

Same architecture as [9Router](https://github.com/decolua/9router) without the Node.js runtime: one Go binary serves the proxy APIs and the embedded Svelte dashboard.

If you code with multiple subscriptions, API keys, and free tiers, quota expires unused on some accounts while rate limits or long tool outputs (`git diff`, `grep`, `ls`) hit others. This router sits at `http://localhost:20130/v1` and handles:

- RTK Token Saver, which compresses `tool_result` blocks to trim token spend.
- Auto fallback and combo routing across subscription, cheap, and free tiers using fallback, round-robin, sticky, capacity, or fusion strategies.
- Multi-account round-robin per provider with OAuth refresh and reactive 401 retry.
- Per-connection outbound proxy pools, including bulk Apply Proxy and custom headers for compatible providers.
- Live `Console Log` and usage SSE streams with custom windows (`14d`, `12h`), currency display (USD, IDR, EUR, and ten others), and per-request payload inspection.

## How it works

```text
┌─────────────┐
│  Your CLI   │  (Claude Code, Codex, OpenClaw, Cursor, Cline...)
│   Tool      │
└──────┬──────┘
       │ http://localhost:20130/v1
       ↓
┌─────────────────────────────────────────────┐
│         9router-go (Smart Router)           │
│  • RTK Token Saver (cut tool_result tokens) │
│  • Format translation (OpenAI ↔ Claude)     │
│  • Quota tracking                           │
│  • Auto token refresh                       │
└──────┬──────────────────────────────────────┘
       │
       ├─→ [Tier 1: SUBSCRIPTION] Claude Code, Codex, GitHub Copilot
       │   ↓ quota exhausted
       ├─→ [Tier 2: CHEAP] GLM ($0.6/1M), MiniMax ($0.2/1M)
       │   ↓ budget limit
       └─→ [Tier 3: FREE] Kiro, OpenCode Free, Vertex ($300 credits)
```

## Quick start

Build `9router-go-x` from source (requires Go 1.27 and Bun 1.x):

```bash
git clone -b custom-edition https://github.com/ulyafarhan/9router-go-x.git
cd 9router-go-x
make web-build   # bun install --frozen-lockfile && bun run build
make build       # embeds the built dashboard into the Go binary
```

If you prefer upstream pre-built binaries from [`luqman-v1/9router-go`](https://github.com/luqman-v1/9router-go) instead of this fork:

```bash
# macOS / Linux (upstream release)
curl -fsSL https://raw.githubusercontent.com/luqman-v1/9router-go/main/install.sh | bash
```

```powershell
# Windows PowerShell (upstream release, writes to %LOCALAPPDATA% and user PATH)
irm https://raw.githubusercontent.com/luqman-v1/9router-go/main/install.ps1 | iex
```

On Linux x86-64 CPUs without AVX (Celeron/Pentium N-series, older Atom, some VMs), a regular build dies with `Illegal instruction` on startup. Use upstream's `install-noavx.sh` (`CGO_ENABLED=0`, `GOAMD64=v1`) or pass `VARIANT=noavx` to `install.sh`:

```bash
curl -fsSL https://raw.githubusercontent.com/luqman-v1/9router-go/main/install-noavx.sh | bash
```

Start the gateway (defaults to port `20130` and data dir `~/.9router`):

```bash
9router-go
# Dashboard: http://localhost:20130 (Default password on localhost: 123456)
```

If you already run upstream [9Router](https://github.com/decolua/9router), point `9router-go` at the same data directory and it opens the existing `DATA_DIR/db/data.sqlite` without an import or migration step. See [Sharing a database with 9Router](#sharing-a-database-with-9router) for details.

Run in the background:

```bash
9router-go start          # same as: 9router-go --background (or -d)
9router-go status         # pid, dashboard URL, log path
9router-go restart        # replace the running daemon
9router-go logs -n 100    # tail the background log
9router-go stop           # stop it
```

The detached process writes `DATA_DIR/run/gateway.pid` and `DATA_DIR/run/gateway.log`. Running `start` while a daemon is active exits instead of fighting over the port, and a stale pid file is cleaned on the next command. On Windows, `stop` calls `TerminateProcess` instead of `SIGTERM`, so it skips graceful stream drain; use the dashboard Shutdown button or `9router-go restart` when in-flight streams matter.

Register autostart on boot:

```bash
9router-go service install     # systemd user unit, launchd agent, OpenRC/runit service, cron @reboot, or Windows logon task
9router-go service status
9router-go service start
9router-go service restart
9router-go service stop
9router-go service uninstall
```

`install` picks the host's native backend (override with `--backend systemd|launchd|openrc|runit|cron`). `systemd`, `launchd`, `cron`, and `runit` (`~/service`) run in user scope without root. OpenRC requires `sudo` (or `--backend cron`), and Windows registers a `schtasks` logon task for the current user from an Administrator console.

Then connect a free provider in the dashboard under Providers (for example Kiro AI or OpenCode Free) and point your CLI tool at `http://localhost:20130/v1` with an API key from Settings -> API Keys.

Upstream also publishes Docker images (`luqmenul/9router-go:latest`, without fork changes):

```bash
docker run -d --name 9router-go --restart unless-stopped \
  -p 20130:20130 -v "$HOME/.9router:/data" \
  -e PORT=20130 -e DATA_DIR=/data \
  -e INITIAL_PASSWORD=your-secure-password \
  luqmenul/9router-go:latest
```

## Key features

- Native Svelte 5 dashboard for providers, OAuth, combos, proxy pools, API keys, usage, quota, plans, Task Persona, and sidebar customization.
- OpenAI Chat, Claude Messages, Gemini, Ollama-compatible, Responses API, embeddings, media, search, and web tools.
- Multi-currency cost display (USD, IDR, EUR, and ten more) with editable rates and public rate refresh.
- SQLite WAL persistence, outbound proxy pools, stall and loop detection, MITM commands, self-update, Docker, and cross-compilation.

For sequential combo routing, you can set `Model response timeout` in seconds (decimals like `5.3` work) in the combo's Edit dialog or on the combo card; leave it empty or `0` to disable it. The timer covers connection selection and waiting for a usable response on every model candidate except the last, which keeps the gateway's normal deadline so a combo never fails sooner than it would without the setting (#744). Once a stream starts, the timer does not cut it off; for non-streaming requests, it waits for the complete response. Fusion panel timing is separate. A model that times out is paused from that combo for a grace period (5 minutes by default, configurable under the timeout, or `0` to retry on the next request), held in memory until restart. Stored as `firstTokenGraceMs` and `comboStrategies[comboName].firstTokenTimeoutMs`.

## Sharing a database with 9Router

If you already run [9Router](https://github.com/decolua/9router) (the Next.js version), `9router-go` reads the same SQLite database without import, export, or migration:

| | Path |
|---|---|
| macOS / Linux | `~/.9router/db/data.sqlite` |
| Windows | `%APPDATA%\9router\db\data.sqlite` |
| Docker | `/data/db/data.sqlite` inside the container |

Stop 9Router, start `9router-go`, and log in with your existing password. Your provider connections, proxy pools, combos, API keys, model aliases, and usage history stay intact. You can also run both side by side against the same `DATA_DIR` on different ports:

```bash
# 9Router on :20128 (Next.js) and 9router-go on :20130 (Go, same DB)
PORT=20130 DATA_DIR=/home/you/.9router ./9router-go
```

SQLite serialises writes, so keep one active writer at a time and avoid editing the same combo or setting in both dashboards simultaneously. Running `9router-go` first and switching back to 9Router later also works: `9router-go` adds `lastUsedAt` and `consecutiveUseCount` on `providerConnections` plus the `upstream_leases` table, and upstream's additive schema sync ignores extra columns and tables.

What `9router-go` does not do:

- It will not import legacy JSON exports; start upstream once to convert old JSON files first.
- It never drops or retypes existing tables or columns, only adds missing ones on startup.
- It does not interpret `_meta.schemaVersion`; check compatibility before pointing an older build at a database written by a much newer 9Router release.

Back up your database directory first if it holds data you cannot rebuild:

```bash
cp -r ~/.9router/db ~/9router-db-backup   # stop the daemon first (see DATABASE.md)
```

Full schema and multi-process rules live in [`DATABASE.md`](DATABASE.md), and routing internals live in [`ARCHITECTURE.md`](ARCHITECTURE.md).

## Screenshots

![9router-go dashboard — Providers view](docs/screenshots/providers.png)

![9router-go dashboard — Endpoint & API key setup](docs/screenshots/endpoint.png)

## Setup guide

### Build from source (`9router-go-x`)

Prerequisites: Go 1.27 and Bun 1.x. Build the Svelte dashboard first because `cmd/9router-go` embeds `web/dist`:

```bash
git clone -b custom-edition https://github.com/ulyafarhan/9router-go-x.git
cd 9router-go-x
make web-build
make build
```

### Upstream release binaries

Pre-built binaries and Docker `exp` images are published by upstream at [`luqman-v1/9router-go/releases`](https://github.com/luqman-v1/9router-go/releases) with `SHA256SUMS.txt`. Those binaries track upstream `luqman-v1/9router-go`, not this fork.

### Run

Defaults listen on port `20130` with data in `~/.9router`:

```bash
9router-go
curl http://localhost:20130/health
./9router-go version
```

Override settings via environment variables (`PORT`, `DATA_DIR`, `DB_PATH`, `HOST`; there are no `--port` CLI flags):

```bash
PORT=20129 ./9router-go
DATA_DIR=/srv/9router ./9router-go
DB_PATH=/srv/9router/data.sqlite ./9router-go
HOST=127.0.0.1 ./9router-go
```

### Dashboard login and fresh install

- On `localhost` / `127.0.0.1`, first login uses the default compatibility password `123456`. Change it in Settings -> Profile once logged in.
- On remote hosts, VPS, Docker, or LAN, remote access blocks `123456` to prevent public takeover of fresh installs (CVE-2026-56679). Either set `INITIAL_PASSWORD="your-secure-password"` on launch (or in `.env`) or tunnel via `ssh -L 20130:127.0.0.1:20130 user@remote-host`, log in over `localhost`, and change the password first.

### Client example

```bash
curl http://localhost:20130/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-your-api-key' \
  -d '{"model":"ag/gemini-3.8-flash-high","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

For Claude Messages clients, set `ANTHROPIC_BASE_URL=http://localhost:20130/v1`.

<details>
<summary><b>Advanced: environment variables, API surface, auth, database</b></summary>

### Environment

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `20130` | HTTP port |
| `HOST` / `BIND_ADDR` | all interfaces | Listener address |
| `DATA_DIR` | `~/.9router` (`%APPDATA%/9router` on Windows) | Data root |
| `DB_PATH` | `$DATA_DIR/db/data.sqlite` | SQLite file |
| `INITIAL_PASSWORD` | unset (fallback `123456` locally) | First dashboard password |
| `RTK_ENABLED` | `true` | RTK input compression |
| `CAVEMAN_ENABLED` / `PONYTAIL_ENABLED` | `false` | Style savers |
| `AUTO_UPDATE` | `false` | Background self-update |
| `HTTP_PROXY` / `HTTPS_PROXY` | Go defaults | Upstream egress proxy |
| `PPROF_ENABLED` | `false` | Expose `/debug/pprof/*` (keep off on untrusted networks) |
| `TRUST_PROXY` / `TRUST_CLOUDFLARE` | unset | Trust forwarded client-IP headers |
| `TRUSTED_PROXIES` | unset | CIDR list of reverse proxies whose forwarded client-IP headers are trusted |
| `GEMINI_WEB_ROTATION` | `on` | Renew the Gemini Web session cookie in the background |

#### Gemini Web session renewal

The `gemini-web` provider talks to `gemini.google.com` with a session cookie, and Google rotates `__Secure-1PSIDTS` every few minutes. The router renews it in the background so signed-in accounts keep reaching paid models.

The dashboard offers two ways to grab a session, plus manual paste:

1. Throwaway browser (safest): opens a clean browser profile, waits for Google sign-in, reads the cookies, then closes the browser and deletes the profile so nothing else rotates the session.
2. Already signed-in browser: reads the existing browser cookie database and unseals it using the OS secret store (Linux keyring, macOS Keychain, Windows Local State). Close the browser first so it does not race with the router's renewal and sign the account out.
3. Manual cookie paste: copy the `Cookie` header from an Incognito window, close that window right away, and paste it into the connection form.

Set `GEMINI_WEB_ROTATION=off` if the cookie belongs to a browser session you want to keep open.

`.env.example` documents the security-sensitive subset and OAuth overrides.

Forwarded client-IP headers are only trusted when the TCP peer is inside the trusted set, otherwise clients could spoof `X-Forwarded-For` to bypass login rate limits. `TRUST_PROXY=true` trusts loopback and RFC1918/link-local ranges; `TRUSTED_PROXIES` takes a comma-separated list of CIDRs or IPs and replaces those defaults. Behind an untrusted proxy, all login attempts share the proxy's socket bucket and five failures lock everyone out.

### API surface

```text
POST /v1/chat/completions       OpenAI Chat Completions
POST /v1/messages               Claude Messages
POST /v1/responses              Responses API
POST /v1/embeddings             Embeddings
POST /api/chat                  Ollama-compatible chat
GET  /v1/models                 Model catalog
POST /v1/images/generations     Image generation
POST /v1/videos/generations     Video generation
POST /v1/audio/speech           Text to speech
POST /v1/audio/transcriptions   Speech to text
POST /v1/search                 Web search
GET  /api/usage/stream          Live usage SSE
GET  /health                    Liveness
GET  /api/version               Version metadata
```

### Authentication

- Public: `/health`, dashboard HTML/assets, `/login`, OAuth callbacks.
- Proxy routes require an active client API key (`Authorization: Bearer` or `X-API-Key`).
- Dashboard APIs require a session cookie, CLI token, or API key; destructive ops (shutdown, update, DB import) require a session or CLI token.
- The server binds all interfaces by default; bind localhost or protect the port on untrusted networks.

### Database compatibility

Go reads and writes the upstream 9Router table and JSON shapes, bootstrapping the 11 core tables, backfilling missing columns, and seeding an empty settings row on startup. Existing databases only receive additive backfills. Full contract in [`DATABASE.md`](DATABASE.md), routing internals in [`ARCHITECTURE.md`](ARCHITECTURE.md).

</details>

## Contributing

Pick the matching template when opening issues or pull requests: Bug report (include reproduction, `9router-go version`, OS, and log excerpt), Feature request, Upstream parity (link the `decolua/9router` commit or PR), or Question.

PR checks match CI: `go vet ./...`, `go test -count=1 ./...`, `make test-integration`, plus `cd web && bun test` and `make vet-svelte` for dashboard changes. Skim [`AGENTS.md`](AGENTS.md) before editing code, since provider isolation and Go/Svelte conventions are checked during review.

## Docs

Upstream documentation is searchable at the [9router-go Wiki](https://luqman-v1.github.io/9router-go/). Repository references:

- [`ARCHITECTURE.md`](ARCHITECTURE.md): routing, providers, runtime layout
- [`DATABASE.md`](DATABASE.md): SQLite schema and operator contract
- [`ROADMAP.md`](ROADMAP.md): proposals only, not current behavior
- [`CHANGELOG.md`](CHANGELOG.md): release history (`9router-go version` prints the active binary version; unreleased `.changes/` fragments fold in at release)
- [`docs/TELEGRAM_RELEASE_NOTIFICATIONS.md`](docs/TELEGRAM_RELEASE_NOTIFICATIONS.md): Telegram release notice setup

## Credits

- [luqman-v1/9router-go](https://github.com/luqman-v1/9router-go): Go + Svelte implementation this fork builds on
- [decolua/9router](https://github.com/decolua/9router): original Next.js gateway and SQLite schema

## License

MIT License. See [`LICENSE`](LICENSE) (retains copyright notices for `decolua and contributors`, `Luqmanul Hakim`, and `Muhammad Ulya Farhan`).
