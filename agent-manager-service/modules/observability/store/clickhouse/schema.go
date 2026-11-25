// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package clickhouse

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the versioned schema files, NNN_description.sql, one
// statement each. Add a new file; never edit one that has been released. A
// file may be applied again if recording it failed, so it must be safe to
// repeat (CREATE ... IF NOT EXISTS).
func Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

// HistoryTable records the schema files applied to the database, next to the
// data they describe.
const HistoryTable = "amp_schema_migrations"

const createHistory = `CREATE TABLE IF NOT EXISTS ` + HistoryTable + ` (
    version String,
    checksum String,
    applied_at DateTime64(3) DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(applied_at) ORDER BY version`

// SchemaConfig locates the database and the login that applies or reads its
// schema.
type SchemaConfig struct {
	URL      string
	Database string // DefaultDatabase when empty
	User     string
	Password string
	// TTLDays is the span table's retention; zero leaves it as it is.
	TTLDays int
}

// Schema applies the schema files to ClickHouse and records them in
// HistoryTable. Applying needs the migrator login; reading what was applied
// needs only the reader's.
type Schema struct {
	cfg    SchemaConfig
	c      *client // in the database
	server *client // in no database, to create it
	files  fs.FS
}

// NewSchema returns the Schema of files for cfg. It does not connect.
func NewSchema(cfg SchemaConfig, files fs.FS) (*Schema, error) {
	if cfg.Database == "" {
		cfg.Database = DefaultDatabase
	}
	if !identifier.MatchString(cfg.Database) {
		return nil, fmt.Errorf("clickhouse: invalid database name %q", cfg.Database)
	}
	if cfg.TTLDays < 0 {
		return nil, fmt.Errorf("clickhouse: ttl-days must not be negative")
	}
	c, err := newClient(cfg.URL, cfg.Database, cfg.User, cfg.Password)
	if err != nil {
		return nil, err
	}
	server, err := newClient(cfg.URL, "", cfg.User, cfg.Password)
	if err != nil {
		return nil, err
	}
	return &Schema{cfg: cfg, c: c, server: server, files: files}, nil
}

// Store names the store.
func (s *Schema) Store() string { return "clickhouse" }

// Files are the schema files.
func (s *Schema) Files() fs.FS { return s.files }

// exists reports whether the database holds table, as far as the login sees.
func (s *Schema) exists(ctx context.Context, table string) (bool, error) {
	n := 0
	err := s.server.query(ctx, "SELECT count() AS n FROM system.tables WHERE database = {db:String} AND name = {t:String}",
		map[string]string{"db": s.cfg.Database, "t": table}, func(line []byte) error {
			var r struct {
				N int `json:"n"`
			}
			err := json.Unmarshal(line, &r)
			n = r.N
			return err
		})
	return n > 0, err
}

// Applied returns the version and checksum of every recorded file.
func (s *Schema) Applied(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	ok, err := s.exists(ctx, HistoryTable)
	if err != nil || !ok {
		return out, err
	}
	err = s.c.query(ctx, "SELECT version, argMax(checksum, applied_at) AS checksum FROM "+HistoryTable+" GROUP BY version", nil,
		func(line []byte) error {
			var r struct {
				Version  string `json:"version"`
				Checksum string `json:"checksum"`
			}
			if err := json.Unmarshal(line, &r); err != nil {
				return err
			}
			out[r.Version] = r.Checksum
			return nil
		})
	return out, err
}

// Apply runs one schema file and records it.
func (s *Schema) Apply(ctx context.Context, version, checksum string, body []byte) error {
	if err := s.server.exec(ctx, "CREATE DATABASE IF NOT EXISTS "+s.cfg.Database, nil); err != nil {
		return err
	}
	if err := s.c.exec(ctx, createHistory, nil); err != nil {
		return err
	}
	if err := s.c.exec(ctx, string(body), nil); err != nil {
		return err
	}
	return s.c.exec(ctx, "INSERT INTO "+HistoryTable+" (version, checksum) SELECT {version:String}, {checksum:String}",
		map[string]string{"version": version, "checksum": checksum})
}

// Settle sets the span table's TTL from config when it differs. Existing
// parts are not rewritten: a changed TTL applies to parts written or merged
// after it.
func (s *Schema) Settle(ctx context.Context) error {
	if s.cfg.TTLDays == 0 {
		return nil
	}
	ttl := "toDateTime(Timestamp) + toIntervalDay(" + strconv.Itoa(s.cfg.TTLDays) + ")"
	var engine string
	err := s.server.query(ctx, "SELECT engine_full FROM system.tables WHERE database = {db:String} AND name = {t:String}",
		map[string]string{"db": s.cfg.Database, "t": Table}, func(line []byte) error {
			var r struct {
				Engine string `json:"engine_full"`
			}
			err := json.Unmarshal(line, &r)
			engine = r.Engine
			return err
		})
	if err != nil {
		return err
	}
	if engine == "" {
		return fmt.Errorf("clickhouse: table %s.%s is missing", s.cfg.Database, Table)
	}
	if strings.Contains(engine, "TTL "+ttl+" ") || strings.HasSuffix(engine, "TTL "+ttl) {
		return nil
	}
	return s.c.exec(ctx, "ALTER TABLE "+Table+" MODIFY TTL "+ttl+" SETTINGS materialize_ttl_after_modify = 0", nil)
}
