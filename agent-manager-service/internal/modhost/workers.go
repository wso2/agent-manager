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

package modhost

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	// DefaultWorkerDrainTimeout is how long a stopping worker gets to return
	// after its ctx is cancelled before it is abandoned.
	DefaultWorkerDrainTimeout = 10 * time.Second
	// DefaultWorkerRetryBase is the first delay before a failed worker is
	// restarted; it doubles up to maxWorkerRetry.
	DefaultWorkerRetryBase = time.Second
	maxWorkerRetry         = time.Minute
)

type workerEntry struct {
	module string
	name   string
	fn     func(ctx context.Context) error
}

// moduleWorkers are the running workers of one module on this replica.
type moduleWorkers struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// syncWorkers brings this replica's running workers in line with v: workers
// of modules that are no longer active stop (each within the drain timeout),
// and those of newly active modules start. A module active before and after
// keeps the workers it has, so reloading for another module's change never
// runs two copies of a worker. Workers run only on a booted, non-passive host.
func (h *Host) syncWorkers(v *view) {
	h.workMu.Lock()
	defer h.workMu.Unlock()
	var stopping []*moduleWorkers
	for name, mw := range h.running {
		if !slices.Contains(v.active, name) {
			stopping = append(stopping, mw)
			delete(h.running, name)
			h.log.Info("modhost:stopping module workers", "module", name)
		}
	}
	h.drainWorkers(stopping)
	if h.NoWorkers || h.runCtx == nil || h.runCtx.Err() != nil {
		return
	}
	for _, name := range v.active {
		entries := v.workers[name]
		if _, ok := h.running[name]; ok || len(entries) == 0 {
			continue
		}
		ctx, cancel := context.WithCancel(h.runCtx)
		mw := &moduleWorkers{cancel: cancel}
		for _, e := range entries {
			mw.wg.Add(1)
			go func() {
				defer mw.wg.Done()
				h.runWorker(ctx, e)
			}()
		}
		h.running[name] = mw
	}
}

// StopWorkers stops every module worker on this replica, giving them the
// drain timeout to return. The server calls it on shutdown.
func (h *Host) StopWorkers() {
	h.workMu.Lock()
	defer h.workMu.Unlock()
	all := make([]*moduleWorkers, 0, len(h.running))
	for name, mw := range h.running {
		all = append(all, mw)
		delete(h.running, name)
	}
	h.drainWorkers(all)
}

// drainWorkers cancels the workers and waits for them, at most the drain
// timeout in all.
func (h *Host) drainWorkers(sets []*moduleWorkers) {
	if len(sets) == 0 {
		return
	}
	done := make(chan struct{})
	for _, mw := range sets {
		mw.cancel()
	}
	go func() {
		for _, mw := range sets {
			mw.wg.Wait()
		}
		close(done)
	}()
	drain := h.WorkerDrainTimeout
	if drain <= 0 {
		drain = DefaultWorkerDrainTimeout
	}
	select {
	case <-done:
	case <-time.After(drain):
		h.log.Warn("modhost:module workers did not stop within the drain timeout; abandoning them", "timeout", drain)
	}
}

// runWorker runs one worker until its ctx ends, restarting it with backoff
// when it fails or panics. A worker that returns nil is done.
func (h *Host) runWorker(ctx context.Context, e workerEntry) {
	log := h.log.With("module", e.module, "worker", e.name)
	base := h.WorkerRetryBase
	if base <= 0 {
		base = DefaultWorkerRetryBase
	}
	delay := base
	for {
		err := callWorker(ctx, e.fn)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			log.Info("modhost:module worker finished")
			return
		}
		log.Error("modhost:module worker failed; restarting", "error", err, "retryIn", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(2*delay, maxWorkerRetry)
	}
}

func callWorker(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return fn(ctx)
}
