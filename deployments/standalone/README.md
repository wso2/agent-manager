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

One-shot services run before the long-running ones and are safe to rerun: `thunder-setup` bootstraps ThunderID once, `opensearch-init` applies the trace index template, `gateway-init` registers the gateway with Agent Manager and renders its config, and `gateway-apis` loads the trace-ingest API into the gateway.

## Run

From the repository root, `make up` builds the Agent Manager images from this checkout and starts the whole stack; `make dev-up` instead runs the API from source under air (hot reload, via `docker-compose.dev.yml`) and the console with `make console` on :3000. Both create `.env` from `.env.example` on first run. `make help` lists the rest (`down`, `logs`, `reset`, `thunder-reset`, …).

Without make, from this directory:

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build --wait
```

`docker-compose.build.yml` builds the Agent Manager images from this checkout. Release bundles leave it out and pin `.env.example` to the published images, so there it is just `docker compose up -d --wait`.

Sign in to the console as `admin` with `THUNDER_ADMIN_PASSWORD` from `.env`.

On Linux hosts OpenSearch needs `sudo sysctl -w vm.max_map_count=262144`; Docker Desktop and Colima already set it.

The ports clash with the k3d development cluster (8080, 19080). Stop one before starting the other, or change the ports in `.env`.

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
docker compose down -v     # delete all data, including ThunderID users and the database
```

## Files

- `thunder/` — ThunderID configuration and AMP bootstrap resources (clients, scopes, roles, default admin), rendered by `render.sh` from `@@NAME@@` placeholders.
- `gateway/` — gateway config template, `gateway-init.sh`, the trace-ingest API and LLM pricing data.
- `otel-collector/config.yaml`, `opensearch/traces-template.json` — trace pipeline.
