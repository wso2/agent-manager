# AMP Lite with Docker Compose

Runs AMP Lite with no Kubernetes, Helm or ingress controller. Every service publishes its own port, and the AI Gateway fronts agent traffic and OTLP trace ingest.

| Service | What it is | Default URL |
|---|---|---|
| `thunder` | ThunderID identity provider | http://thunder.amp.localhost:8080 |
| `amp-api` | Agent Manager API | http://localhost:9000 |
| `amp-api` (internal) | Gateway control plane (WebSocket, self-signed TLS) | https://localhost:9243 |
| `amp-console` | Console | http://localhost:3000 |
| `amp-observer` | Trace query API | http://localhost:9098 |
| `gateway-runtime` | AI Gateway (agent traffic, `/otel/v1/traces`) | http://default-default.gateway.localhost:19080 |
| `opensearch`, `otel-collector` | Trace store and pipeline | internal only |
| `postgres` (optional profile) | PostgreSQL instead of SQLite | internal only |

Startup, in order:

1. `amp-init` (the `amp-api` image, `-init-thunder` / `-init-gateway`) writes ThunderID's configuration and bootstrap resources and the AI Gateway's `config.toml` from the settings in `docker-compose.yml` and `.env`: public URLs, ports, client IDs and secrets, default users. Scope lists and role grants come from Agent Manager's own permission catalog. On the first run it also creates the gateway's encryption key, listener certificate and registration token (unless `DEFAULT_GATEWAY_TOKEN` is set), and keeps them afterwards.
2. `thunder-setup` runs ThunderID's setup when the generated bootstrap differs from the one last applied. A rerun is additive: it updates AMP's clients, roles and settings and keeps everything else in ThunderID's database. It also sets the admin password to `THUNDER_ADMIN_PASSWORD` again.
3. `amp-api` migrates its database, then registers the default gateway (`DEFAULT_GATEWAY_*`) with the token from step 1, creating the `default` organization and environment if needed.
4. `gateway-controller` connects to `amp-api` with that token. On every gateway connection, `amp-api` deploys the built-in trace-ingest API (`/otel`, `OTEL_INGEST_UPSTREAM_URL`) on the gateway if it does not serve the current version. It is a control-plane deployment, so a gateway restart or resync keeps it.
5. `amp-observer` applies the `otel-traces-*` index template (span IDs and attributes as keywords) to OpenSearch before it reports healthy; `otel-collector` waits for that.

## Run

From a release bundle (this folder, unpacked), the images are already pinned in `.env.example`:

```bash
cp .env.example .env
docker compose up -d --wait
```

From a source checkout, `make up` at the repository root builds the Agent Manager images and starts the whole stack; `make dev-up` instead runs the API from source under air (hot reload, via `docker-compose.dev.yml`) and the console with `make console` on :3000. Both create `.env` from `.env.example` on first run. `make help` lists the rest (`down`, `logs`, `reset`, `thunder-reset`, …). Without make, run this from `deployments/standalone`:

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build --wait
```

`docker-compose.build.yml` builds the Agent Manager images from the checkout; release bundles leave it out.

Sign in to the console as `admin` with `THUNDER_ADMIN_PASSWORD` from `.env`.

### PostgreSQL

SQLite is the default database. To use PostgreSQL, uncomment `COMPOSE_PROFILES=postgres` and `AMP_DB_DRIVER=postgres` in `.env` (or run `make up DB=postgres`); `amp-api` migrates it on start. Switching does not copy data between the two.

On Linux hosts OpenSearch needs `sudo sysctl -w vm.max_map_count=262144`; Docker Desktop and Colima already set it.

On macOS with Colima, keep this folder inside your home folder: Colima shares only that with its VM, and the bind mounts come up empty anywhere else.

## Send a trace

```bash
curl -X POST http://localhost:19080/otel/v1/traces \
  -H 'Host: default-default.gateway.localhost' \
  -H "x-amp-api-key: <agent token>" \
  -H 'Content-Type: application/json' -d @spans.json
```

Get the agent token from `POST /api/v1/orgs/{org}/projects/{project}/agents/{agent}/token`.

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
- `otel-collector/config.yaml` — trace pipeline. The index template is built into the observer.
