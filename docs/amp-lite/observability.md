# Observability: traces

The `observability` module serves an organization's traces from ClickHouse. Spans reach ClickHouse through the AMP OpenTelemetry collector:

```text
agent ──OTLP──▶ amp (observability module: checks the agent key, sets identity headers)
                 │
                 ▼  observability network (internal, no published ports)
          otel-collector ──insert──▶ ClickHouse ◀──read── amp
```

The module's OTLP ingest route, which forwards to the collector, is still to come. This page covers the install component: ClickHouse, the collector, and the schema step.

Without any of this, the module keeps traces in memory, and Lite runs no ClickHouse or collector.

## What is installed

| Piece | What it does |
|---|---|
| `observability-secrets` | Writes the four passwords, one file per job, before anything else in the profile starts. |
| `clickhouse` | Bundled profile only. Stores spans in `otel.otel_traces`. Creates no tables itself; its four logins come from [`users.xml`](../../agent-manager-service/modules/observability/store/clickhouse/users.xml). |
| `observability-migrate` | Runs `amp migrate` as the migrator login once ClickHouse and `amp-api` are healthy (`amp-api`'s first start creates the encryption key, which needs an empty database), so the schema is applied before the module is installed (and, with the bundled ClickHouse, before the collector starts). |
| `otel-collector` | Receives OTLP/HTTP, sets each span's identity from the request headers, and writes to ClickHouse as the insert-only login. |
| `observability` network | Internal: holds `amp-api`, ClickHouse and the collector (and the migrate step while it runs), with no route out and no published ports. With your own ClickHouse, the collector also joins a network of its own to reach it. |

Each login does one job, and its password is a file named for that job:

| Job | Login | Rights | Used by |
|---|---|---|---|
| setup | `amp_migrator` | DDL on `otel`, the schema history | `amp migrate` only |
| write | `amp_ingest` | `INSERT` on the span table | the collector |
| read | `amp_reader` | `SELECT` on the span table and the schema history; a read-only profile that stops a read after 25 s | the module |
| delete | `amp_cleaner` | `DELETE` on the span table | the module, for a deleted agent's spans |

### Identity

The collector trusts only the headers the module sets from the verified agent key ([`otel-collector.yaml`](../../deployments/standalone/observability/otel-collector.yaml)):

1. It deletes every `agent-manager/*` resource attribute the agent sent.
2. It sets them from the headers. The span table's `org_id`, `project_id` and `agent_id` columns are materialized from the first three.

   | Header | Resource attribute |
   |---|---|
   | `x-org-id` | `agent-manager/org-id` |
   | `x-user-project` | `agent-manager/project-uid` |
   | `x-user-component` | `agent-manager/agent-uid` |
   | `x-user-environment` | `agent-manager/environment-uid` |
   | `x-user-organization-name`, `x-user-project-name`, `x-user-agent-name` | `agent-manager/organization`, `agent-manager/project`, `agent-manager/agent` |

3. It drops a span with no `x-org-id`; it never stores one with an empty org.

Only containers on the observability network can reach the collector, so only `amp` sets those headers.

## Quick setup: bundled ClickHouse

Put these two lines in `.env` next to the compose files (copy `.env.example` if there is none):

```bash
COMPOSE_PROFILES=observability          # postgres,observability with PostgreSQL
AMP_OBSERVABILITY_STORE_URL=http://clickhouse:8123
```

Every `docker compose` command, `make up` and `make dev-up` read them. With either observability profile on and no `AMP_OBSERVABILITY_STORE_URL`, the start stops at its first step (`service "observability-secrets" didn't complete successfully`); `docker compose logs observability-secrets` (or `observability-secrets-external`) names the missing line. `make up-observability` sets them for its own run only, so without them in `.env` a later restart, `make up`, or the rotation steps below bring `amp-api` back without the store address, serving traces from memory, and the profile's services are left out.

From a source checkout:

```bash
make up-observability       # the full stack, ClickHouse, the collector; installs the module
make smoke-observability    # ports, network, schema, and a span stored under the right org
```

`make dev-up-observability` does the same on the dev stack, with the API hot-reloading under air and the console on :3000; `make smoke-observability` checks it too.

From a release bundle, or without make:

```bash
docker compose up -d --wait
docker compose exec amp-api /go/bin/amp module install observability
```

The four passwords are generated on first start. `AMP_OBSERVABILITY_TTL_DAYS` sets how long spans are kept (30 days when empty).

## Your own ClickHouse, with Compose

1. **Create the logins.** Fill in a password for each job in [`clickhouse-users.sql`](../../deployments/standalone/observability/clickhouse-users.sql) and run it on your ClickHouse as an admin. It grants the same rights as `users.xml`.
2. **Set `.env`.** Compose refuses to start while a password is unset.

   ```bash
   COMPOSE_PROFILES=observability-external
   AMP_OBSERVABILITY_STORE_URL=http://clickhouse.example.internal:8123
   AMP_OBSERVABILITY_READ_PASSWORD=...
   AMP_OBSERVABILITY_WRITE_PASSWORD=...
   AMP_OBSERVABILITY_DELETE_PASSWORD=...
   AMP_OBSERVABILITY_SETUP_PASSWORD=...
   ```

3. **Start and install:** `docker compose up -d --wait`, then `docker compose exec amp-api /go/bin/amp module install observability`.

The collector and the schema step run here, against your ClickHouse; no ClickHouse runs in the stack.

## Passwords

Each password is a file in the `observability-secrets` volume, named for its job (`read-password`, `write-password`, `delete-password`, `setup-password`). The one-shot `observability-secrets` step writes them before the rest of the profile starts: with the bundled ClickHouse it generates any that are missing and keeps them across restarts; with your own, it writes the ones from `.env` every start. Each container mounts only the files it uses:

| Container | Passwords |
|---|---|
| `clickhouse` | all four; its entrypoint exports them into the variables `users.xml` reads, in its own process only |
| `amp-api` | read, delete (a copy in `amp-observability-secrets`) |
| `observability-migrate` | setup |
| the collector | write |

No password is in a container's configuration or `docker inspect`. ClickHouse holds all four in its own process environment, since `users.xml` reads them from there.

To read one, for example to sign in with `clickhouse-client`:

```bash
docker compose exec clickhouse sh -c 'clickhouse-client -u amp_reader --password "$(cat /run/secrets/observability/read-password)"'
```

To rotate one:

- **Bundled:** with the two lines from [Quick setup](#quick-setup-bundled-clickhouse) in `.env`, delete its file and recreate what reads it; a new one is generated. Run these from `deployments/standalone`; on the dev stack (`make dev-up-observability`), add `-f docker-compose.yml -f docker-compose.dev.yml` after `docker compose`.

  ```bash
  docker compose run --rm --no-deps --entrypoint rm observability-secrets /out/all/read-password
  docker compose up -d --wait --no-deps --force-recreate observability-secrets clickhouse amp-api otel-collector
  ```

- **Your own:** change it on ClickHouse and in `.env`, then `docker compose up -d --wait --force-recreate`.

## Your own ClickHouse, without Compose

1. **Create the logins** with [`clickhouse-users.sql`](../../deployments/standalone/observability/clickhouse-users.sql), or mount `users.xml` into the server's `users.d/` with the four `AMP_CLICKHOUSE_*_PASSWORD` variables in its environment.
2. **Set the addresses on `amp`**, in its environment or `amp.yaml`. Give each password as a file: `<SETTING>_FILE` in the environment, or `${file:/path}` in `amp.yaml`. Every key is listed in [Module: observability](configuration.md#module-observability).

   | Setting | Value |
   |---|---|
   | `MODULE_OBSERVABILITY_CLICKHOUSE_URL` | ClickHouse's HTTP address, e.g. `http://clickhouse.internal:8123` |
   | `MODULE_OBSERVABILITY_CLICKHOUSE_USER`, `_PASSWORD` | `amp_reader` and the read password |
   | `MODULE_OBSERVABILITY_CLICKHOUSE_CLEANER_USER`, `_CLEANER_PASSWORD` | `amp_cleaner` and the delete password; unset, a deleted agent's spans stay until they expire |

3. **Run `amp migrate`** with the setup login, set for this run only:

   ```bash
   MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_USER=amp_migrator \
   MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_PASSWORD_FILE=/path/to/setup-password \
   MODULE_OBSERVABILITY_CLICKHOUSE_TTL_DAYS=30 \
   amp migrate
   ```

   Running it again applies nothing.
4. **Restart `amp`** so the servers read the new settings.
5. **Enable the module:** `amp module install observability` (or `amp module enable observability` if it is installed). Installing checks that the schema is applied and ClickHouse is reachable, and is refused otherwise.
6. **Run a collector** with [`otel-collector.yaml`](../../deployments/standalone/observability/otel-collector.yaml), with `AMP_CLICKHOUSE_ENDPOINT` (e.g. `tcp://clickhouse.internal:9000`) in its environment and the write password in `/run/secrets/observability/write-password`. Make it reachable from `amp` only: it trusts the identity headers of anyone who can reach it.

## Adding observability after install

`amp serve --migrate` does not apply the ClickHouse schema: the servers hold only the reader login, so it logs that the migrator login is not configured and carries on. Adding observability to a running install, or upgrading to a release with a new schema file, means running `amp migrate` again with the migrator login. With the profile, `docker compose up` runs the migrate step each time (`docker compose run --rm observability-migrate` runs it alone); with your own ClickHouse without Compose, run step 3 above.

## Turning it off

`amp module disable observability` stops the module's routes. To stop ClickHouse and the collector too, remove the profile lines from `.env`, run `docker compose --profile observability --profile observability-external rm -sf clickhouse otel-collector otel-collector-external`, then `docker compose up -d --wait`. The spans stay in the `clickhouse-data` volume.

## Kubernetes

There is no Helm chart for Lite yet, so there is no Helm value for this either.
