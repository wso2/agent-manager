// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db/connpool"
	"github.com/wso2/agent-manager/agent-manager-service/db/sqlitedialect"
)

var (
	db     *gorm.DB
	dbOnce sync.Once
)

// instance lazily establishes the database connection on first use.
//
// The connection is intentionally NOT opened in an init() function: doing so
// forced every package that transitively imports db to reach a live database
// at import time, which made package init (and therefore pure-logic unit tests)
// fail without Postgres. Connecting on first use keeps production behavior
// unchanged — startup paths (app bootstrap, migrations) call DB()/GetDB()
// immediately after config load, so the process still connects at startup and
// still fails fast (os.Exit) if the database is unreachable — while letting
// callers that never touch the DB run without one.
func instance() *gorm.DB {
	dbOnce.Do(func() {
		cfg := config.GetConfig()
		db = initDbConn(cfg.Database, cfg.POSTGRESQL)
	})
	return db
}

// slogWriter implements the GORM logger Writer interface using slog
type slogWriter struct{}

func (w slogWriter) Printf(format string, v ...interface{}) {
	slog.Warn(fmt.Sprintf(format, v...), "log_type", "gorm")
}

func initDbConn(dbCfg config.DatabaseConfig, cfg config.POSTGRESQL) *gorm.DB {
	gormDB, err := Open(dbCfg, cfg)
	if err != nil {
		slog.Error("initDbConn: failed to open the database", "driver", dbCfg.Driver, "error", err)
		os.Exit(1)
	}
	slog.Info("database connected", "driver", dbCfg.Driver)
	return gormDB
}

// Open connects to the configured database. Pool, logger and GORM settings are the same for every driver.
func Open(dbCfg config.DatabaseConfig, cfg config.POSTGRESQL) (*gorm.DB, error) {
	var (
		driverName string
		dsn        string
		dialector  func(dsn string, conn gorm.ConnPool) gorm.Dialector
	)
	switch dbCfg.Driver {
	case config.DBDriverPostgres:
		driverName, dsn = "pgx", makeConnString(cfg)
		dialector = func(dsn string, conn gorm.ConnPool) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, Conn: conn})
		}
	case config.DBDriverSQLite:
		if err := os.MkdirAll(filepath.Dir(dbCfg.SQLitePath), 0o750); err != nil {
			return nil, fmt.Errorf("creating the SQLite directory: %w", err)
		}
		driverName, dsn = sqlitedialect.DriverName, makeSQLiteConnString(dbCfg.SQLitePath)
		dialector = func(dsn string, conn gorm.ConnPool) gorm.Dialector {
			return &sqlitedialect.Dialector{DSN: dsn, Conn: conn}
		}
	default:
		return nil, fmt.Errorf("unsupported database driver %q", dbCfg.Driver)
	}

	sqlConnPool, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	setConfigsOnDB(sqlConnPool, cfg.DbConfigs)
	if err := sqlConnPool.Ping(); err != nil {
		_ = sqlConnPool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	connPool := connpool.New(sqlConnPool, connpool.RetryParams{
		MaxRetries: 3,
		BackoffFunc: func(failedCount int) time.Duration {
			return time.Duration(failedCount) * time.Second * 5
		},
	})

	gormLogger := logger.New(
		slogWriter{},
		logger.Config{
			SlowThreshold:             time.Duration(cfg.SlowThresholdMilliseconds) * time.Millisecond,
			IgnoreRecordNotFoundError: true,
			LogLevel:                  logger.Warn,
		},
	)

	// The dialector is given the wrapped pool: left to itself it opens a pool of its own from the DSN, bypassing
	// the retries, the pool settings and the UTC time arguments.
	gormDB, err := gorm.Open(dialector(dsn, connPool), &gorm.Config{
		Logger:                 gormLogger,
		SkipDefaultTransaction: cfg.SkipDefaultTransaction,
		PrepareStmt:            false,
		FullSaveAssociations:   false,
		ConnPool:               connPool,
		// SQLite stores times as text, which sorts correctly only when every value is in the same zone.
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		_ = sqlConnPool.Close()
		return nil, fmt.Errorf("gorm.Open: %w", err)
	}
	return gormDB, nil
}

// makeSQLiteConnString enables foreign keys and WAL on every connection, and starts every transaction with the write
// lock (BEGIN IMMEDIATE) so a transaction that reads and then writes never fails to upgrade its lock. _time_format=sqlite
// writes times as "2006-01-02 15:04:05.999999999-07:00": with UTC the suffix is "+00:00", and "+" sorts before ".", so
// text order is time order even though trailing fractional zeros are dropped. A "Z" suffix would break that, and so
// would the driver's default, time.Time.String, which appends the zone name.
func makeSQLiteConnString(path string) string {
	params := url.Values{}
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "journal_mode(WAL)")
	params.Add("_pragma", "busy_timeout(5000)")
	params.Set("_txlock", "immediate")
	params.Set("_time_format", "sqlite")
	return "file:" + path + "?" + params.Encode()
}

func setConfigsOnDB(db *sql.DB, cfg config.DbConfigs) {
	if cfg.MaxIdleTimeSeconds != nil {
		db.SetConnMaxIdleTime(time.Duration(*cfg.MaxIdleTimeSeconds) * time.Second)
	}
	if cfg.MaxLifetimeSeconds != nil {
		db.SetConnMaxLifetime(time.Duration(*cfg.MaxLifetimeSeconds) * time.Second)
	}
	if cfg.MaxOpenCount != nil {
		db.SetMaxOpenConns(int(*cfg.MaxOpenCount))
	}
	if cfg.MaxIdleCount != nil {
		db.SetMaxIdleConns(int(*cfg.MaxIdleCount))
	}
}

// GetDB returns the raw database instance without any context attached.
// This should be used when injecting the DB into repositories that will
// add context per-operation using db.DB(ctx).
func GetDB() *gorm.DB {
	return instance()
}

func makeConnString(p config.POSTGRESQL) string {
	params := url.Values{}
	// Only set the TLS parameters when configured. Omitting them keeps the DSN
	// byte-identical to earlier builds, so pgx's libpq-compatible "prefer"
	// default (and any PGSSLMODE/PGSSLROOTCERT in the environment) still apply.
	if p.SSLMode != "" {
		params.Set("sslmode", p.SSLMode)
	}
	if p.SSLRootCert != "" {
		params.Set("sslrootcert", p.SSLRootCert)
	}
	conn := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     fmt.Sprintf("%s:%d", p.Host, p.Port),
		Path:     "/" + p.DBName,
		RawQuery: params.Encode(),
	}
	return conn.String()
}
