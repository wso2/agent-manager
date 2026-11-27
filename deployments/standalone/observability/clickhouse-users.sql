-- The observability module's four logins, for your own ClickHouse: the same
-- users and grants as clickhouse-users.xml (CI checks the grants match).
-- Replace each <...> with the password you set in .env, then run it as an
-- admin, e.g. clickhouse-client --multiquery < clickhouse-users.sql.

-- setup (AMP_OBSERVABILITY_SETUP_PASSWORD): applies the schema in `amp migrate`.
CREATE USER IF NOT EXISTS amp_migrator IDENTIFIED BY '<setup password>';
GRANT CREATE DATABASE, CREATE TABLE, DROP TABLE, ALTER COLUMN, ALTER INDEX, ALTER TTL, ALTER MATERIALIZE TTL, ALTER SETTINGS ON otel.* TO amp_migrator;
GRANT SELECT, INSERT ON otel.amp_schema_migrations TO amp_migrator;

-- write (AMP_OBSERVABILITY_WRITE_PASSWORD): the collector.
CREATE USER IF NOT EXISTS amp_ingest IDENTIFIED BY '<write password>';
GRANT INSERT ON otel.otel_traces TO amp_ingest;

-- read (AMP_OBSERVABILITY_READ_PASSWORD): the module. Read-only, and a read
-- stops after 25 s (the module's client gives up at 30 s), as in users.xml.
CREATE SETTINGS PROFILE IF NOT EXISTS amp_reader_profile SETTINGS
    readonly = 2,
    max_execution_time = 25 MIN 1 MAX 25,
    cancel_http_readonly_queries_on_client_close = 1;
CREATE USER IF NOT EXISTS amp_reader IDENTIFIED BY '<read password>' SETTINGS PROFILE 'amp_reader_profile';
GRANT SELECT ON otel.otel_traces TO amp_reader;
GRANT SELECT ON otel.amp_schema_migrations TO amp_reader;

-- delete (AMP_OBSERVABILITY_DELETE_PASSWORD): the module, for a deleted agent's spans.
CREATE USER IF NOT EXISTS amp_cleaner IDENTIFIED BY '<delete password>';
GRANT ALTER DELETE, ALTER UPDATE(_row_exists) ON otel.otel_traces TO amp_cleaner;
