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

package services

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// secretJobInterval is how often the worker looks for due secret vault jobs.
const secretJobInterval = 10 * time.Second

// SecretJobWorker runs the external-vault work the secret store queues once the transaction that queued it commits:
// promoting new values to the external vault and removing the values they replaced. Jobs survive a restart in the
// database, so the worker picks up whatever an earlier process left.
type SecretJobWorker interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce runs the jobs due now.
	RunOnce(ctx context.Context)
}

type secretJobWorker struct {
	runner   *repositories.SecretJobRunner
	logger   *slog.Logger
	interval time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once
	done     sync.WaitGroup
}

// NewSecretJobWorker runs runner's jobs every few seconds.
func NewSecretJobWorker(runner *repositories.SecretJobRunner, logger *slog.Logger) SecretJobWorker {
	return &secretJobWorker{runner: runner, logger: logger, interval: secretJobInterval, stopCh: make(chan struct{})}
}

func (w *secretJobWorker) Start(ctx context.Context) error {
	w.done.Add(1)
	go w.runLoop(ctx)
	w.logger.Info("Secret vault job worker started", "interval", w.interval)
	return nil
}

// Stop ends the loop and waits for a pass in progress.
func (w *secretJobWorker) Stop() error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.done.Wait()
		w.logger.Info("Secret vault job worker stopped")
	})
	return nil
}

func (w *secretJobWorker) runLoop(ctx context.Context) {
	defer w.done.Done()
	// Pick up what an earlier process left at once.
	w.RunOnce(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.RunOnce(ctx)
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *secretJobWorker) RunOnce(ctx context.Context) {
	ran, failed, err := w.runner.RunOnce(ctx, time.Now().UTC())
	if err != nil {
		w.logger.Error("Failed to claim secret vault jobs", "error", err)
		return
	}
	if ran > 0 {
		w.logger.Debug("Ran secret vault jobs", "ran", ran, "failed", failed)
	}
}
