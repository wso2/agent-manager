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
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db/connpool"
)

var db *gorm.DB

func init() {
	db = initDbConn(config.GetConfig().POSTGRESQL)
}

// slogWriter implements the GORM logger Writer interface using slog
type slogWriter struct{}

func (w slogWriter) Printf(format string, v ...interface{}) {
	slog.Warn(fmt.Sprintf(format, v...), "log_type", "gorm")
}

func initDbConn(cfg config.POSTGRESQL) *gorm.DB {
	dsn := makeConnString(cfg)
	sqlConnPool, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error("initDbConn: sql.Open failed", "error", err)
		os.Exit(1)
	}
	setConfigsOnDB(sqlConnPool, cfg.DbConfigs)
	if err := sqlConnPool.Ping(); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
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

	// Open PostgreSQL connection
	dialector := postgres.Open(dsn)
	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 gormLogger,
		SkipDefaultTransaction: cfg.SkipDefaultTransaction,
		PrepareStmt:            false,
		FullSaveAssociations:   false,
		ConnPool:               connPool,
	})
	if err != nil {
		slog.Error("initDbConn: gorm.Open failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected")
	return gormDB
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

func makeConnString(p config.POSTGRESQL) string {
	params := url.Values{}
	conn := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     fmt.Sprintf("%s:%d", p.Host, p.Port),
		Path:     "/" + p.DBName,
		RawQuery: params.Encode(),
	}
	return conn.String()
}
