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
3. `amp-api` (`amp serve --migrate`) migrates the core's and every module's tables, starts the module host, and, with file sign-in, gives the trial organization (`DEFAULT_ORG_*`) its `default` environment if needed. The `ai-gateway` module, which `amp` installs at first boot, then registers the default gateway (`MODULE_AI_GATEWAY_DEFAULT_GATEWAY_*`) with the token from step 1 ([how the default gateway registers itself](../../docs/amp-lite/ai-gateway.md#how-the-default-gateway-registers-itself)).
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

File mode suits local and single-operator installs. It has no identity provider, so users, groups, roles, profiles and agent identities are off: the console hides them and the API answers their routes with 409 `IDENTITY_PROVIDER_REQUIRED`. For anything shared, set `auth.mode: idp` to hand sign-in to ThunderID, or to [another identity provider](#connecting-another-identity-provider); the key set the API validates tokens with follows the mode, and those features come on. `amctl` and the MCP endpoints need `idp` too. ThunderID still runs in the stack in either mode, but only `idp` uses it.

Agent Manager keeps no table of organizations: a request's organization is the one its token names (`ouId`, `ouHandle`). File sign-in names a trial organization, `default.org_ou_id` (`00000000-0000-0000-0000-000000000001`) with handle `default.org_handle`, and the bundled gateway is registered to it. Switching to an identity provider starts in the provider's own organization, which gets its default environment on its first request and has no gateway until one is registered for it; the trial organization's projects, agents, secrets and gateway stay in the database under the trial id and come back if `auth.mode` returns to `file`. Treat the first run as a trial. An organization's handle should not change once it is in use: traces are found by it. A database created before this layout (it had an `organizations` table) needs `docker compose down -v`.

### Starting with an identity provider

The bundled gateway is registered to file sign-in's trial organization. A stack that signs in through an identity provider from its first start can register it to the provider's organization instead:

1. Find the provider's organization id, the `ouId` its tokens carry. For the bundled ThunderID it is `01900000-0000-7000-8000-000000000001`; for your own ThunderID it is the id of the organization unit your users belong to (its console, `GET /organization-units`, or the `ouId` claim of one of its tokens).
2. Before the first start, set the sign-in mode and the gateway's organization in `amp.yaml`:

   ```yaml
   auth:
     mode: idp
   modules:
     - name: ai-gateway
       config:
         default_gateway_org_ou_id: 01900000-0000-7000-8000-000000000001   # the provider's organization id
   ```

   A `modules:` list is authoritative: a module it leaves out is turned off. `amp` installs only `ai-gateway` by default, so list any other module you have turned on as well. `MODULE_AI_GATEWAY_DEFAULT_GATEWAY_ORG_OU_ID` on `amp-api` does the same.
3. Start the stack (`make up`), sign in through the provider, and check that **Infrastructure → Gateways** lists *Default AI Gateway* as active.

This works only before the bundled gateway has registered elsewhere. After a trial with file sign-in its registration token belongs to the trial organization, and pointing it at another organization leaves the gateway unregistered: the gateway module keeps retrying and logs "its token's prefix belongs to another token". Register a new gateway for the provider's organization from the console instead (**Add gateway**).

### Connecting another identity provider

`auth.mode: idp` uses the bundled ThunderID unless `idp.issuer` names another OpenID Connect provider. The API then trusts that provider's tokens and nothing of ThunderID's: it finds the provider's key set through its discovery document (`<issuer>/.well-known/openid-configuration`; a `KEY_MANAGER_JWKS_URL` you set wins, so remove one that names ThunderID's keys), accepts its issuer and the audiences in `idp.audience`, and advertises it to MCP clients as their authorization server.

```yaml
auth:
  mode: idp
idp:
  issuer: https://idp.example.com/realms/amp   # exactly as its tokens' iss claim names it
  audience: amp-api                            # the aud its tokens name for AMP, comma-separated
platform:
  admin_ou_id: 0190a000-0000-7000-8000-0000000000aa   # your platform organization's ouId
```

The provider's access tokens must be RS256-signed JWTs that carry:

- `ouId`: the organization's id, a UUID that never changes. It becomes the organization's id in AMP; a new `ouId` is a new, empty organization.
- `ouHandle`: the organization's handle. Traces are found by it, so it should not change either.
- `scope`: the `amp:*` permissions, space-separated.

Most providers add such claims with a mapper or a post-login action; one organization can be a fixed value on the client. `platform.admin_ou_id` (`PLATFORM_ADMIN_OU_ID`) has no default with another provider: until it is set, the cross-organization routes refuse everyone. Register the bundled gateway to the same `ouId` as in [Starting with an identity provider](#starting-with-an-identity-provider).

Today this covers the API, `amctl` and MCP clients. The console still signs in with the ThunderID SDK, and users, groups, roles, profiles and agent identities are administered in the bundled ThunderID, so those work only with it for now.

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

The `observability` profile adds ClickHouse and the OpenTelemetry collector on an internal network that only `amp-api` joins, with no published ports. A one-shot `observability-migrate` step runs `amp migrate` with ClickHouse's migrator login, once `amp-api` is up, so the trace schema is in place before the collector starts and before the module is installed; ClickHouse creates no tables itself, and `amp-api` holds only the reading and cleaning logins. `amp-init` generates the four passwords, or writes the ones `amp.yaml` sets, into a folder per container that reads them; `observability-external` uses your own ClickHouse instead, with the same passwords (see [docs/amp-lite/observability.md](../../docs/amp-lite/observability.md)).

```bash
make up-observability       # also installs the observability module
make dev-up-observability   # the same on the dev stack (hot reload)
make smoke-observability    # either one
```

Set `COMPOSE_PROFILES=observability` and `AMP_OBSERVABILITY_STORE_URL` in `.env` (the commented lines in `.env.example`): `make up-observability` sets them for its own run only, and later commands (a restart, `make up`, rotating a password) need them to keep `amp-api` on ClickHouse. Without make, start the stack, then run `docker compose exec amp-api amp module install observability`. To use your own ClickHouse instead, or add observability to an existing install, see [docs/amp-lite/observability.md](../../docs/amp-lite/observability.md).

On macOS with Colima, keep this folder inside your home folder: Colima shares only that with its VM, and the bind mounts come up empty anywhere else.

## Host names

Browsers resolve `*.localhost` to 127.0.0.1, so the public URLs above work from the host. Inside Docker, curl and some other clients also pin `*.localhost` to loopback, so containers call each other by service name (`thunder`, `amp-api`, `gateway-runtime`). ThunderID's public URL is still the token issuer everywhere; the API dials it through `THUNDER_RESOLVE_TO_HOST`.

## Stop and reset

```bash
docker compose down        # keep data
docker compose --profile postgres --profile observability --profile observability-external down -v   # delete all data, including ThunderID users, the database and traces
```

## Files

- `amp.yaml` — Agent Manager's settings, read by `amp-init` and `amp-api`.
- `docker-compose.postgres.yml` — points Agent Manager at the bundled PostgreSQL.
- `thunder/entrypoint.sh` — installs the configuration `amp-init` generated into the ThunderID image and runs its setup or server. The templates live in `agent-manager-service/ampinit`.
- `gateway/model_prices.json` — LLM pricing data for the gateway's cost policies.
- `observability/clickhouse-users.xml` — ClickHouse's logins for the observability profile, a copy of `agent-manager-service/modules/observability/store/clickhouse/users.xml` (CI checks the two match).
- `observability/clickhouse-users.sql` — the same logins as SQL, for your own ClickHouse (CI checks each login's hosts and grants match).
- `observability/otel-collector.yaml` — the collector's configuration: identity from the module's headers, spans to ClickHouse.
