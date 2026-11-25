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
| `clickhouse` | Stores spans in `otel.otel_traces`. Creates no tables itself; its four logins come from [`users.xml`](../../agent-manager-service/modules/observability/store/clickhouse/users.xml). |
| `observability-migrate` | Runs `amp migrate` as the migrator login once ClickHouse and `amp-api` are healthy (`amp-api`'s first start creates the encryption key, which needs an empty database), so the schema is applied before the collector starts and before the module is installed. |
| `otel-collector` | Receives OTLP/HTTP, sets each span's identity from the request headers, and writes to ClickHouse as the insert-only login. |
| `observability` network | Internal: holds `amp-api`, ClickHouse and the collector (and the migrate step while it runs), with no route out and no published ports. |

| Login | Rights | Used by |
|---|---|---|
| `amp_migrator` | DDL on `otel`, the schema history | `amp migrate` only |
| `amp_ingest` | `INSERT` on the span table | the collector |
| `amp_reader` | `SELECT` on the span table and the schema history | the module |
| `amp_cleaner` | `DELETE` on the span table | the module, for a deleted agent's spans |

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

## Quick setup: the Compose profile

From a source checkout:

```bash
make up-observability       # the full stack, ClickHouse, the collector; installs the module
make smoke-observability    # ports, network, schema, and a span stored under the right org
```

From a release bundle, or without make, set these in `.env` and start as usual:

```bash
COMPOSE_PROFILES=observability          # postgres,observability with PostgreSQL
AMP_OBSERVABILITY_CLICKHOUSE_URL=http://clickhouse:8123
AMP_OBSERVABILITY_COLLECTOR_URL=http://otel-collector:4318
```

```bash
docker compose up -d --wait
docker compose exec amp-api /go/bin/amp module install observability
```

Change the four `AMP_CLICKHOUSE_*_PASSWORD` values in `.env` first; ClickHouse refuses to start while any is empty. `AMP_OBSERVABILITY_TTL_DAYS` sets how long spans are kept (30 days when empty).

## Using an existing ClickHouse

1. **Create the logins.** Mount `users.xml` into the server's `users.d/` with the four `AMP_CLICKHOUSE_*_PASSWORD` variables in its environment, or create users with the same grants in SQL. The grants name the `otel` database.
2. **Set the addresses on `amp`**, in its environment or `amp.yaml`:

   | Setting | Value |
   |---|---|
   | `MODULE_OBSERVABILITY_CLICKHOUSE_URL` | ClickHouse's HTTP address, e.g. `http://clickhouse.internal:8123` |
   | `MODULE_OBSERVABILITY_CLICKHOUSE_USER`, `_PASSWORD` | `amp_reader` and its password |
   | `MODULE_OBSERVABILITY_CLICKHOUSE_CLEANER_USER`, `_CLEANER_PASSWORD` | `amp_cleaner` and its password; unset, a deleted agent's spans stay until they expire |
   | `MODULE_OBSERVABILITY_CLICKHOUSE_DATABASE` | `otel` (the default) |
   | `MODULE_OBSERVABILITY_COLLECTOR_URL` | the collector's OTLP/HTTP address |

3. **Run `amp migrate`** with the migrator login, set for this run only:

   ```bash
   MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_USER=amp_migrator \
   MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_PASSWORD=... \
   MODULE_OBSERVABILITY_CLICKHOUSE_TTL_DAYS=30 \
   amp migrate
   ```

   Running it again applies nothing.
4. **Restart `amp`** so the servers read the new settings.
5. **Enable the module:** `amp module install observability` (or `amp module enable observability` if it is installed). Installing checks that the schema is applied and ClickHouse is reachable, and is refused otherwise.
6. **Run a collector** with [`otel-collector.yaml`](../../deployments/standalone/observability/otel-collector.yaml), with `AMP_CLICKHOUSE_ENDPOINT` (e.g. `tcp://clickhouse.internal:9000`) and `AMP_CLICKHOUSE_INGEST_PASSWORD` in its environment. Make it reachable from `amp` only: it trusts the identity headers of anyone who can reach it.

## Adding observability after install

`amp serve --migrate` does not apply the ClickHouse schema: the servers hold only the reader login, so it logs that the migrator login is not configured and carries on. Adding observability to a running install, or upgrading to a release with a new schema file, means running `amp migrate` again with the migrator login. With the profile, `docker compose up` runs the migrate step each time (`docker compose run --rm observability-migrate` runs it alone); with your own ClickHouse, run step 3 above.

## Turning it off

`amp module disable observability` stops the module's routes. To stop ClickHouse and the collector too, remove the profile lines from `.env`, run `docker compose --profile observability rm -sf clickhouse otel-collector`, then `docker compose up -d --wait`. The spans stay in the `clickhouse-data` volume.

## Kubernetes

There is no Helm chart for Lite yet, so there is no Helm value for this either.
