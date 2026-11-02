package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

func Open(cfg config.TraceReaderConfig) (*sql.DB, error) {
	db := ch.OpenDB(&ch.Options{
		Addr: []string{cfg.Addr},
		Auth: ch.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		DialTimeout: cfg.DialTimeout,
	})

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to clickhouse at %s: %w", cfg.Addr, err)
	}
	slog.Info("clickhouse connected", "addr", cfg.Addr, "database", cfg.Database)
	return db, nil
}
