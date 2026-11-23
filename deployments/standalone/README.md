# AMP Lite with Docker Compose

Runs AMP Lite with no Kubernetes, Helm or ingress controller. Every service publishes its own port, and the AI Gateway fronts agent traffic.

| Service | What it is | Default URL |
|---|---|---|
| `thunder` | ThunderID identity provider | http://thunder.amp.localhost:8080 |
| `amp-api` | Agent Manager API and console | http://localhost:9000 |
| `amp-api` (internal) | Gateway control plane (WebSocket, self-signed TLS) | https://localhost:9243 |
| `gateway-runtime` | AI Gateway (agent traffic) | http://default-default.gateway.localhost:19080 |
| `postgres` (optional profile) | PostgreSQL instead of SQLite | internal only |
| `clickhouse`, `otel-collector` (optional `observability` profile) | Trace store and the OpenTelemetry collector | internal only, on a private network |

Startup, in order:

1. `amp-init` (the `amp-api` image, `amp init thunder … ai-gateway … fileauth …`) generates the secrets nothing sets (client secrets, the admin and gateway passwords) into the `amp-secrets` volume, then writes ThunderID's configuration and bootstrap resources and the AI Gateway's `config.toml` from `amp.yaml`: public URLs, client IDs and secrets, default users. Scope lists and role grants come from Agent Manager's own permission catalog. On the first run it also creates the gateway's encryption key, listener certificate and registration token, and keeps all of these afterwards.
2. `thunder-setup` runs ThunderID's setup when the generated bootstrap differs from the one last applied. A rerun is additive: it updates AMP's clients, roles and settings and keeps everything else in ThunderID's database. It also sets the admin password to the one in `amp-secrets` again.
3. `amp-api` (`amp serve --migrate`) migrates the core's and every module's tables, starts the module host, and creates the `default` organization and environment if needed (`DEFAULT_ORG_*`). The `ai-gateway` module, which `amp` installs at first boot, then registers the default gateway (`MODULE_AI_GATEWAY_DEFAULT_GATEWAY_*`) with the token from step 1 ([how the default gateway registers itself](../../docs/amp-lite/ai-gateway.md#how-the-default-gateway-registers-itself)).
4. `gateway-controller` connects to `amp-api` with that token and receives its APIs.

Without the `observability` profile the stack runs no trace store, and the gateway serves no `/otel` trace-ingest API. With it, ClickHouse and the OpenTelemetry collector run for the `observability` module, and agents send traces to the module at `<amp>/m/observability/otel` with their agent key (see [Observability](#observability-traces)).

## Run

From a release bundle (this folder, unpacked), the images are already pinned in `.env.example`:

```bash
cp .env.example .env
docker compose up -d --wait
```

Agent Manager's settings are in [`amp.yaml`](amp.yaml), mounted into `amp-init` and `amp-api`. `.env` is optional: it changes only what Compose itself needs (images, host ports, ThunderID's network name), and every value has a default. The API port and ThunderID's host and port appear in both files, so change them together. Every key `amp.yaml` takes is in the [configuration reference](../../docs/amp-lite/configuration.md); a value can be a `${env:...}` or `${file:...}` reference.

From a source checkout, `make up` at the repository root builds the Agent Manager images and starts the whole stack; `make dev-up` instead runs the API from source under air (hot reload, via `docker-compose.dev.yml`, built without the embedded console) and the console with `make console` on :3000 (Vite, hot reload). `make help` lists the rest (`down`, `logs`, `reset`, `thunder-reset`, …). Without make, run this from `deployments/standalone`:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build --wait
```

`docker-compose.build.yml` builds the Agent Manager images from the checkout; release bundles leave it out.

Sign in to the console (http://localhost:9000, served by `amp-api`) as `admin`, with the password generated on first start:

```bash
docker compose exec amp-api amp secret show admin-password
```

`amp secret list` names the other generated secrets; they live in the `amp-secrets` volume and are kept across restarts. To choose a secret instead, set it in `amp.yaml` (for example `amp: {admin_password: ...}`, or a `${file:...}` reference); a set value always wins.

The console's settings are `console:` keys in `amp.yaml` (`CONSOLE_<NAME>` fills the `$NAME` of the same name in the console's `config.js`); which of its pages show comes from the modules the API runs (`GET /api/v1/ui-manifest`), plus any listed in `console.ui_modules_enable`. Behind a reverse proxy, set `server.public_url` to the public origin; ThunderID always accepts sign-in from the console at that URL, and the stack adds the development console (`CONSOLE_PORT`) to it.

Environment variables still win over `amp.yaml`, so a setting given on a service's `environment:` overrides the file.

`auth.mode` decides how sign-in works and ships as `file`: the API checks the password against `/amp/fileauth/users.yaml` (`FILE_AUTH_USERS_PATH`) and issues its own tokens, with no identity provider involved. `amp-init` regenerates that file on every start from the admin password and `amp.default_users`, so edit `amp.yaml` and restart rather than the file itself. Sessions last `FILE_AUTH_TOKEN_TTL` (8 hours) and are not renewed.

File mode suits local and single-operator installs. For anything shared, set `auth.mode: idp` to hand sign-in to ThunderID; the key set the API validates tokens with follows the mode. ThunderID runs either way, because agent identity uses it; `amctl` and the MCP endpoints need `idp`.

### amp-core (no AI Gateway)

`make up-core` builds the core binary (`DISTRIBUTION=amp-core`: organizations, projects, agents, sign-in, tokens, no modules) and starts it without `gateway-controller` and `gateway-runtime`; `amp-init` writes no gateway configuration. It adds `docker-compose.core.yml` and runs as its own Compose project, `amp-lite-core`, so its data stays apart from the full stack's. Both use the same ports, so run one at a time. `make down-core` and `make reset-core` stop it; `DB=postgres` works as for `make up`. The console shows only the pages the core serves. Without make, from this folder:

```bash
docker compose -p amp-lite-core -f docker-compose.yml -f docker-compose.build.yml -f docker-compose.core.yml up -d --build --wait
```

### Smoke test

`make smoke` (or `make smoke-core`) runs `scripts/smoke.sh amp|amp-core` against the running stack: sign-in, the core API, which routes the distribution serves, the console and the UI manifest. It reads ports from the environment or `.env`, the admin password from `AMP_ADMIN_PASSWORD` or the running `amp-api`, and needs `auth.mode: file`. `make smoke-observability` runs `scripts/smoke-observability.sh` against a stack from `make up-observability`.

### PostgreSQL

SQLite is the default database. To use PostgreSQL, run `make up DB=postgres`, or set `COMPOSE_PROFILES=postgres` in `.env` and add `-f docker-compose.postgres.yml` (or set the `db:` block in `amp.yaml`); its password is generated with the other secrets, and `amp-api` migrates it on start. Switching does not copy data between the two.

### Observability (traces)

The `observability` profile adds ClickHouse and the OpenTelemetry collector on an internal network that only `amp-api` joins, with no published ports. A one-shot `observability-migrate` step runs `amp migrate` with ClickHouse's migrator login, once `amp-api` is up, so the trace schema is in place before the collector starts and before the module is installed; ClickHouse creates no tables itself, and `amp-api` holds only the reading and cleaning logins.

```bash
make up-observability       # also installs the observability module
make smoke-observability
```

Without make, set `COMPOSE_PROFILES=observability`, `AMP_OBSERVABILITY_CLICKHOUSE_URL` and `AMP_OBSERVABILITY_COLLECTOR_URL` in `.env` (the commented lines in `.env.example`), set the four `AMP_CLICKHOUSE_*_PASSWORD` values (they have no default), start the stack, then run `docker compose exec amp-api amp module install observability`. To use your own ClickHouse instead, or add observability to an existing install, see [docs/amp-lite/observability.md](../../docs/amp-lite/observability.md).

On macOS with Colima, keep this folder inside your home folder: Colima shares only that with its VM, and the bind mounts come up empty anywhere else.

## Host names

Browsers resolve `*.localhost` to 127.0.0.1, so the public URLs above work from the host. Inside Docker, curl and some other clients also pin `*.localhost` to loopback, so containers call each other by service name (`thunder`, `amp-api`, `gateway-runtime`). ThunderID's public URL is still the token issuer everywhere; the API dials it through `THUNDER_RESOLVE_TO_HOST`.

## Stop and reset

```bash
docker compose down        # keep data
docker compose --profile postgres --profile observability down -v   # delete all data, including ThunderID users, the database and traces
```

## Files

- `amp.yaml` — Agent Manager's settings, read by `amp-init` and `amp-api`.
- `docker-compose.postgres.yml` — points Agent Manager at the bundled PostgreSQL.
- `thunder/entrypoint.sh` — installs the configuration `amp-init` generated into the ThunderID image and runs its setup or server. The templates live in `agent-manager-service/ampinit`.
- `gateway/model_prices.json` — LLM pricing data for the gateway's cost policies.
- `observability/clickhouse-users.xml` — ClickHouse's logins for the observability profile, a copy of `agent-manager-service/modules/observability/store/clickhouse/users.xml` (CI checks the two match).
- `observability/otel-collector.yaml` — the collector's configuration: identity from the module's headers, spans to ClickHouse.
