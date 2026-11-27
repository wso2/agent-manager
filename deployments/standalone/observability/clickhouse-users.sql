-- The observability module's four logins, for your own ClickHouse: the same
-- users, hosts and grants as clickhouse-users.xml (CI checks they match).
-- Replace each <...> with its password (docker compose run --rm amp-init
-- secret show <job>-password), then run it as an admin, e.g.
-- clickhouse-client --multiquery < clickhouse-users.sql.

-- setup (setup-password): applies the schema in `amp migrate`.
CREATE USER IF NOT EXISTS amp_migrator IDENTIFIED BY '<setup password>' HOST IP '127.0.0.1', IP '::1', IP '10.0.0.0/8', IP '172.16.0.0/12', IP '192.168.0.0/16', IP 'fc00::/7';
GRANT CREATE DATABASE, CREATE TABLE, DROP TABLE, ALTER COLUMN, ALTER INDEX, ALTER TTL, ALTER MATERIALIZE TTL, ALTER SETTINGS ON otel.* TO amp_migrator;
GRANT SELECT, INSERT ON otel.amp_schema_migrations TO amp_migrator;

-- write (write-password): the collector.
CREATE USER IF NOT EXISTS amp_ingest IDENTIFIED BY '<write password>' HOST IP '127.0.0.1', IP '::1', IP '10.0.0.0/8', IP '172.16.0.0/12', IP '192.168.0.0/16', IP 'fc00::/7';
GRANT INSERT ON otel.otel_traces TO amp_ingest;

-- read (read-password): the module. Read-only, and a read
-- stops after 25 s (the module's client gives up at 30 s), as in users.xml.
CREATE SETTINGS PROFILE IF NOT EXISTS amp_reader_profile SETTINGS
    readonly = 2,
    max_execution_time = 25 MIN 1 MAX 25,
    cancel_http_readonly_queries_on_client_close = 1;
CREATE USER IF NOT EXISTS amp_reader IDENTIFIED BY '<read password>' HOST IP '127.0.0.1', IP '::1', IP '10.0.0.0/8', IP '172.16.0.0/12', IP '192.168.0.0/16', IP 'fc00::/7' SETTINGS PROFILE 'amp_reader_profile';
GRANT SELECT ON otel.otel_traces TO amp_reader;
GRANT SELECT ON otel.amp_schema_migrations TO amp_reader;

-- delete (delete-password): the module, for a deleted agent's spans.
CREATE USER IF NOT EXISTS amp_cleaner IDENTIFIED BY '<delete password>' HOST IP '127.0.0.1', IP '::1', IP '10.0.0.0/8', IP '172.16.0.0/12', IP '192.168.0.0/16', IP 'fc00::/7';
GRANT ALTER DELETE, ALTER UPDATE(_row_exists) ON otel.otel_traces TO amp_cleaner;
