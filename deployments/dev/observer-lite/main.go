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

// Command observer-lite stands in for the OpenChoreo Observer in lite dev mode.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wso2/agent-manager/deployments/dev/observer-lite/api"
	"github.com/wso2/agent-manager/deployments/dev/observer-lite/store"
)

const (
	apiAddr         = ":8080"
	ingestAddr      = ":4318"
	shutdownTimeout = 10 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := start(ctx); err != nil {
		slog.Error("observer-lite failed", "error", err)
		os.Exit(1)
	}
}

func start(ctx context.Context) error {
	client, ingest, closeStore, err := store.New()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	apiLn, err := net.Listen("tcp", apiAddr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen on %s: %w", apiAddr, err), closeStore())
	}
	ingestLn, err := net.Listen("tcp", ingestAddr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen on %s: %w", ingestAddr, err), apiLn.Close(), closeStore())
	}
	slog.Info("observer-lite listening", "api", apiLn.Addr().String(), "ingest", ingestLn.Addr().String())
	return run(ctx, apiLn, ingestLn, api.NewHandler(client), ingest, closeStore)
}

// run serves both handlers until ctx is cancelled or either server fails,
// then shuts both down and closes the store.
func run(ctx context.Context, apiLn, ingestLn net.Listener, apiHandler, ingestHandler http.Handler, closeStore func() error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	servers := []*http.Server{
		{Handler: apiHandler, ReadHeaderTimeout: shutdownTimeout},
		{Handler: ingestHandler, ReadHeaderTimeout: shutdownTimeout},
	}
	listeners := []net.Listener{apiLn, ingestLn}
	serveErrs := make(chan error, len(servers))
	for i, srv := range servers {
		go func() {
			err := srv.Serve(listeners[i])
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			serveErrs <- err
		}()
	}

	var errs []error
	select {
	case <-ctx.Done():
	case err := <-serveErrs:
		errs = append(errs, err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancelShutdown()
	for _, srv := range servers {
		errs = append(errs, srv.Shutdown(shutdownCtx))
	}
	for range servers {
		select {
		case err := <-serveErrs:
			errs = append(errs, err)
		default:
		}
	}
	errs = append(errs, closeStore())
	return errors.Join(errs...)
}
