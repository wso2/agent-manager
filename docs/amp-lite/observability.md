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
| `amp-init` | Generates the four passwords, or writes the ones `amp.yaml` sets, when the store URL is set: one file per job, into a folder per container that reads it. It does nothing new without a store URL. |
| `clickhouse` | Bundled profile only. Stores spans in `otel.otel_traces`. Creates no tables itself; its four logins come from [`users.xml`](../../agent-manager-service/modules/observability/store/clickhouse/users.xml). |
| `observability-migrate` | Runs `amp migrate` as the migrator login once ClickHouse and `amp-api` are healthy (`amp-api`'s first start creates the encryption key, which needs an empty database), so the schema is applied before the collector starts and before the module is installed. |
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

Every `docker compose` command, `make up` and `make dev-up` read them. With either observability profile on and no `AMP_OBSERVABILITY_STORE_URL`, amp-init writes no observability passwords, so ClickHouse and `observability-migrate` stop with a message naming that line (`docker compose logs clickhouse observability-migrate`) and the collector does not start; the rest of the stack runs, with traces in memory. `make up-observability` sets them for its own run only, so without them in `.env` a later restart, `make up`, or the rotation steps below bring `amp-api` back without the store address, serving traces from memory, and the profile's services are left out.

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

amp-init generates the four passwords on first start. `AMP_OBSERVABILITY_TTL_DAYS` sets how long spans are kept (30 days when empty).

## Your own ClickHouse, with Compose

The collector and the schema step run here, against your ClickHouse; no ClickHouse runs in the stack. The four passwords are set like any other Agent Manager secret: amp-init generates each one you don't set in `amp.yaml`, and none goes in `.env`. Run the commands below from `deployments/standalone`.

1. **Set `.env`:**

   ```bash
   COMPOSE_PROFILES=observability-external
   AMP_OBSERVABILITY_STORE_URL=http://clickhouse.example.internal:8123
   ```

2. **Generate or set the passwords.**
   - To have them generated, run `docker compose run --rm amp-init`, then read each one:

     ```bash
     docker compose run --rm amp-init secret list
     docker compose run --rm amp-init secret show setup-password    # and read-, write-, delete-password
     ```

   - To choose them, keep them in a folder only root can read (`chmod 700` the folder, `chmod 600` the files), mount it into `amp-init` alone with a `docker-compose.override.yml` beside the compose files, and name the files in `amp.yaml`:

     ```yaml
     # docker-compose.override.yml
     services:
       amp-init:
         volumes:
           - /etc/amp/observability:/etc/amp/observability:ro
     ```

     ```yaml
     # amp.yaml
     module:
       observability:
         clickhouse:
           password: "${file:/etc/amp/observability/read-password:-}"
           cleaner-password: "${file:/etc/amp/observability/delete-password:-}"
           migrator-password: "${file:/etc/amp/observability/setup-password:-}"
           ingest-password: "${file:/etc/amp/observability/write-password:-}"
     ```

     amp-init writes each one where its containers read it, on every start, so the folder always wins. The `:-` lets `amp-api` and `observability-migrate`, which also read `amp.yaml` but never mount this folder, start without it; they read amp-init's copies instead. If amp-init can't read a file either, it generates that password, and `observability-migrate` then stops with the message below.

3. **Create the logins.** Fill in each `<... password>` in [`clickhouse-users.sql`](../../deployments/standalone/observability/clickhouse-users.sql) and run it on your ClickHouse as an admin. It grants the same hosts and rights as `users.xml`.
4. **Start and install:** `docker compose up -d --wait`, then `docker compose exec amp-api /go/bin/amp module install observability`.

If a login is missing or its password differs, `observability-migrate` stops with "ClickHouse refused the amp_migrator login … create the observability logins first", the collector does not start, and installing the module is refused; `amp-api` keeps running. Create or fix the login, then run `docker compose up -d --wait` again.

## Passwords

The four are generated secrets like `admin-password`: amp-init writes each one when the store URL is set, keeps a generated one across restarts, and writes a value `amp.yaml` (or the environment) sets instead, on every start. An empty value counts as unset. The files are named for the job (`read-password`, `write-password`, `delete-password`, `setup-password`) and none is in `amp-secrets`, the folder `amp-api` reads its own secrets from. Each container mounts only the folder with its own:

| Volume | Passwords | Mounted by |
|---|---|---|
| `observability-secrets` | all four | `clickhouse`, whose entrypoint exports them into the variables `users.xml` reads, in its own process only |
| `amp-observability-secrets` | read, delete | `amp-api` |
| `observability-migrate-secrets` | setup | `observability-migrate` |
| `observability-collector-secrets` | write | the collector |

amp-init mounts all four volumes to write them. No password is in a container's configuration or `docker inspect`. ClickHouse holds all four in its own process environment, since `users.xml` reads them from there.

To list or read one, run the amp CLI in amp-init, the only container that holds all four (`amp secret` in `amp-api` lists only its own):

```bash
docker compose run --rm amp-init secret list
docker compose run --rm amp-init secret show read-password
```

To rotate one, from `deployments/standalone` with the profile and store URL in `.env` (on the dev stack, add `-f docker-compose.yml -f docker-compose.dev.yml` after `docker compose`):

1. Generated: delete its file, and amp-init generates a new one. Chosen: change the file in your folder.

   ```bash
   docker compose run --rm --no-deps --entrypoint rm amp-init /out/clickhouse/read-password
   ```

2. With your own ClickHouse, set the new password on it: `ALTER USER amp_reader IDENTIFIED BY '...'`, with the value from `amp secret show`.
3. Rerun amp-init and recreate what reads it (`clickhouse` only with the bundled one; `otel-collector-external` with your own):

   ```bash
   docker compose up -d --wait --no-deps --force-recreate amp-init clickhouse amp-api otel-collector
   ```

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
