# AMP Lite with Docker Compose

Runs AMP Lite with no Kubernetes, Helm or ingress controller. Every service publishes its own port, and the AI Gateway fronts agent traffic.

| Service | What it is | Default URL |
|---|---|---|
| `thunder` | ThunderID identity provider | http://thunder.amp.localhost:8080 |
| `amp-api` | Agent Manager API and console | http://localhost:9000 |
| `amp-api` (internal) | Gateway control plane (WebSocket, self-signed TLS) | https://localhost:9243 |
| `gateway-runtime` | AI Gateway (agent traffic) | http://default-default.gateway.localhost:19080 |
| `postgres` (optional profile) | PostgreSQL instead of SQLite | internal only |

Startup, in order:

1. `amp-init` (the `amp-api` image, `amp init thunder … ai-gateway … fileauth …`) writes ThunderID's configuration and bootstrap resources and the AI Gateway's `config.toml` from the settings in `docker-compose.yml` and `.env`: public URLs, ports, client IDs and secrets, default users. Scope lists and role grants come from Agent Manager's own permission catalog. On the first run it also creates the gateway's encryption key, listener certificate and registration token (unless `DEFAULT_GATEWAY_TOKEN` is set), and keeps them afterwards.
2. `thunder-setup` runs ThunderID's setup when the generated bootstrap differs from the one last applied. A rerun is additive: it updates AMP's clients, roles and settings and keeps everything else in ThunderID's database. It also sets the admin password to `AMP_ADMIN_PASSWORD` again.
3. `amp-api` (`amp serve --migrate`) migrates the core's and every module's tables, starts the module host, and creates the `default` organization and environment if needed (`DEFAULT_ORG_*`). The built-in gateway module then registers the default gateway (`DEFAULT_GATEWAY_*`) with the token from step 1.
4. `gateway-controller` connects to `amp-api` with that token and receives its APIs.

The stack runs no trace store: there is no OpenSearch, OpenTelemetry collector or trace query service, and the gateway serves no `/otel` trace-ingest API. Traces will return as an optional module.

## Run

From a release bundle (this folder, unpacked), the images are already pinned in `.env.example`:

```bash
cp .env.example .env
docker compose up -d --wait
```

From a source checkout, `make up` at the repository root builds the Agent Manager images and starts the whole stack; `make dev-up` instead runs the API from source under air (hot reload, via `docker-compose.dev.yml`, built without the embedded console) and the console with `make console` on :3000 (Vite, hot reload). Both create `.env` from `.env.example` on first run. `make help` lists the rest (`down`, `logs`, `reset`, `thunder-reset`, …). Without make, run this from `deployments/standalone`:

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build --wait
```

`docker-compose.build.yml` builds the Agent Manager images from the checkout; release bundles leave it out.

Sign in to the console (http://localhost:9000, served by `amp-api`) as `admin` with `AMP_ADMIN_PASSWORD` from `.env`. The console's settings are `CONSOLE_*` variables on `amp-api` (each fills the `$NAME` of the same name in the console's `config.js`); which of its pages show comes from the modules the API runs (`GET /api/v1/ui-manifest`), plus any listed in `CONSOLE_UI_MODULES_ENABLE`. Behind a reverse proxy, set `SERVER_PUBLIC_URL` on amp-api to the public origin.

`amp-api` takes its settings from the environment Compose passes it. Anything else can go in an `amp.yaml` the container reads (`--config` or `AMP_CONFIG`), including the `modules:` list it reconciles at boot; environment variables win over the file. See [`amp.example.yaml`](../../agent-manager-service/amp.example.yaml).

`AUTH_MODE` decides how sign-in works and ships as `file`: the API checks the
password against `/etc/amp/users.yaml` and issues its own tokens, with no
identity provider involved. `amp-init` regenerates that file on every start from
`AMP_ADMIN_PASSWORD` and `AMP_DEFAULT_USERS`, so edit `.env` and restart rather
than the file itself. Sessions last `FILE_AUTH_TOKEN_TTL` (8 hours) and are not
renewed.

File mode suits local and single-operator installs. For anything shared, set
`AUTH_MODE=idp` to hand sign-in to ThunderID — along with `KEY_MANAGER_JWKS_URL`,
which defaults to the API's own key set and does not follow `AUTH_MODE`.
`.env.example` lists both. ThunderID runs either way, because
agent identity uses it; `amctl` and the MCP endpoints need `idp`.

### amp-core (no AI Gateway)

`make up-core` builds the core binary (`DISTRIBUTION=amp-core`: organizations, projects, agents, sign-in, tokens, no modules) and starts it without `gateway-controller` and `gateway-runtime`; `amp-init` writes no gateway configuration. It adds `docker-compose.core.yml` and runs as its own Compose project, `amp-lite-core`, so its data stays apart from the full stack's. Both use the same ports, so run one at a time. `make down-core` and `make reset-core` stop it; `DB=postgres` works as for `make up`. The console shows only the pages the core serves. Without make, from this folder:

```bash
docker compose -p amp-lite-core -f docker-compose.yml -f docker-compose.build.yml -f docker-compose.core.yml up -d --build --wait
```

### Smoke test

`make smoke` (or `make smoke-core`) runs `scripts/smoke.sh amp|amp-core` against the running stack: sign-in, the core API, which routes the distribution serves, the console and the UI manifest. It reads ports and `AMP_ADMIN_PASSWORD` from the environment or `.env`, and needs `AUTH_MODE=file`.

### PostgreSQL

SQLite is the default database. To use PostgreSQL, uncomment `COMPOSE_PROFILES=postgres` and `AMP_DB_DRIVER=postgres` in `.env` (or run `make up DB=postgres`); `amp-api` migrates it on start. Switching does not copy data between the two.

On macOS with Colima, keep this folder inside your home folder: Colima shares only that with its VM, and the bind mounts come up empty anywhere else.

## Host names

Browsers resolve `*.localhost` to 127.0.0.1, so the public URLs above work from the host. Inside Docker, curl and some other clients also pin `*.localhost` to loopback, so containers call each other by service name (`thunder`, `amp-api`, `gateway-runtime`). ThunderID's public URL is still the token issuer everywhere; the API dials it through `THUNDER_RESOLVE_TO_HOST`.

## Stop and reset

```bash
docker compose down        # keep data
docker compose --profile postgres down -v   # delete all data, including ThunderID users and the database
```

## Files

- `thunder/entrypoint.sh` — installs the configuration `amp-init` generated into the ThunderID image and runs its setup or server. The templates live in `agent-manager-service/ampinit`.
- `gateway/model_prices.json` — LLM pricing data for the gateway's cost policies.
