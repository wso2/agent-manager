package app

import (
	"fmt"
	"log/slog"

	"github.com/wso2/agent-manager/agent-manager-observer/clickhouse"
	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearchreader"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// newTraceReader creates a TraceReader based on the configuration.
func newTraceReader(cfg *config.Config, openChoreo observer.Client) (observer.TraceReader, error) {
	switch cfg.Traces.Backend {
	case "", config.TraceBackendOpenChoreo:
		slog.Info("trace backend selected", "backend", config.TraceBackendOpenChoreo)
		return openChoreo, nil
	case config.TraceBackendClickHouse:
		db, err := clickhouse.Open(cfg.Traces)
		if err != nil {
			return nil, fmt.Errorf("failed to open clickhouse: %w", err)
		}

		resolver, err := scope.NewStaticResolverFromJSON(cfg.Traces.ScopeUIDs)

		if err != nil {
			_ = db.Close()
			return nil, err
		}

		reader, err := clickhouse.NewTraceReader(db, clickhouse.Options{
			Table: cfg.Traces.Table, Layout: cfg.Traces.Layout,
			DefaultNamespace: cfg.Observer.DefaultNamespace, Resolver: resolver,
		})

		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to create clickhouse trace reader: %w", err)
		}
		slog.Info("trace backend selected", "backend", config.TraceBackendClickHouse)
		return reader, nil
	case config.TraceBackendOpenSearch:
		resolver, err := scope.NewStaticResolverFromJSON(cfg.Traces.ScopeUIDs)
		if err != nil {
			return nil, err
		}
		reader, err := opensearchreader.NewTraceReader(opensearchreader.Options{
			Addr: cfg.Traces.OSAddr, Username: cfg.Traces.OSUser, Password: cfg.Traces.OSPassword,
			IndexPattern: cfg.Traces.OSIndex, InsecureTLS: cfg.Traces.OSInsecureTLS,
			DefaultNamespace: cfg.Observer.DefaultNamespace, Resolver: resolver,
		})
		if err != nil {
			return nil, fmt.Errorf("create opensearch trace reader: %w", err)
		}
		slog.Info("trace backend selected", "backend", config.TraceBackendOpenSearch)
		return reader, nil
	default:
		return nil, fmt.Errorf("unknown TRACE_BACKEND %q", cfg.Traces.Backend)
	}
}