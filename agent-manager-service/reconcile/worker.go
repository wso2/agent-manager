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

package reconcile

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	renderBatch        = 20
	placeBatch         = 100
	driftBatch         = 100
	driftUnlockTimeout = 5 * time.Second
	// placeQueue is how many requested placements wait for the worker; past it the slow placement pass covers them.
	placeQueue = 64
	// placedReason is what a user is shown while a row the placement pass placed is rendered.
	placedReason = "placed on the environment's gateway"
)

// Worker runs the core's three loops: rendering due rows, placing environment-addressed records that have no row,
// and checking deployed rows for drift.
type Worker struct {
	core   *Core
	logger *slog.Logger

	nudgeCh  chan struct{}
	placeCh  chan placeRequest
	stopCh   chan struct{}
	stopOnce sync.Once
	done     sync.WaitGroup
}

// placeRequest is an environment a request just mapped a gateway to.
type placeRequest struct {
	ouID string
	env  uuid.UUID
}

// NewWorker makes the core's nudges reach the worker.
func NewWorker(core *Core, logger *slog.Logger) *Worker {
	w := &Worker{
		core: core, logger: logger,
		nudgeCh: make(chan struct{}, 1), placeCh: make(chan placeRequest, placeQueue), stopCh: make(chan struct{}),
	}
	core.nudge = w.Nudge
	return w
}

// Start runs the render loop on its short interval and on nudges, the fleet-wide placement pass at once and then on
// its slow interval, requested placements as they come, and the drift check on its own slow interval.
func (w *Worker) Start(ctx context.Context) error {
	w.done.Add(4)
	go w.loop(ctx, w.core.cfg.RenderInterval, w.nudgeCh, w.RunOnce)
	go func() {
		w.placeAll(ctx)
		w.loop(ctx, w.core.cfg.PlaceInterval, nil, w.placeAll)
	}()
	go w.placeRequested(ctx)
	go w.driftLoop(ctx)
	w.logger.Info("Reconcile worker started", "renderInterval", w.core.cfg.RenderInterval,
		"placeInterval", w.core.cfg.PlaceInterval, "driftInterval", w.core.cfg.DriftInterval)
	return nil
}

// Stop ends the loops and waits for the work in progress.
func (w *Worker) Stop() error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.done.Wait()
		w.logger.Info("Reconcile worker stopped")
	})
	return nil
}

// Nudge runs the render loop now, for a render a request just made owed.
func (w *Worker) Nudge() {
	select {
	case w.nudgeCh <- struct{}{}:
	default:
	}
}

func (w *Worker) loop(ctx context.Context, interval time.Duration, wake <-chan struct{}, run func(context.Context)) {
	defer w.done.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run(ctx)
		case <-wake:
			run(ctx)
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// RunOnce renders every row due now, a batch at a time, each render of a batch in parallel.
func (w *Worker) RunOnce(ctx context.Context) {
	for {
		claims, err := w.core.repo.ClaimDueRenders(ctx, w.core.now(), w.core.cfg.Lease, renderBatch)
		if err != nil {
			w.logger.Error("Failed to claim due renders", "error", err)
			return
		}
		var wg sync.WaitGroup
		for _, claim := range claims {
			wg.Go(func() {
				if err := ignoreRenderFailed(w.core.renderClaim(ctx, claim)); err != nil {
					w.logger.Error("Failed to write back a render", "artifactUUID", claim.ArtifactUUID,
						"gatewayUUID", claim.GatewayUUID, "error", err)
				}
			})
		}
		wg.Wait()
		if len(claims) < renderBatch {
			return
		}
	}
}

// RunOnceFor renders the artifact's rows due now, as RunOnce does for the fleet. A test renders its own artifact
// with it without claiming rows other tests sharing the database made due.
func (w *Worker) RunOnceFor(ctx context.Context, ouID string, artifactUUID uuid.UUID) error {
	return w.core.renderOwed(ctx, ouID, artifactUUID, nil, ignoreRenderFailed)
}

// RequestPlaceEnv queues a placement pass over the environment, which a gateway was just mapped to, without waiting
// for it. A full queue drops the request: the slow placement pass places the environment.
func (w *Worker) RequestPlaceEnv(ouID string, env uuid.UUID) {
	select {
	case w.placeCh <- placeRequest{ouID: ouID, env: env}:
	default:
		w.logger.Warn("Placement queue is full; the slow placement pass will place the environment",
			"ouID", ouID, "environmentUUID", env)
	}
}

// placeRequested runs requested placements one at a time; Stop waits for the one in progress.
func (w *Worker) placeRequested(ctx context.Context) {
	defer w.done.Done()
	for {
		select {
		case req := <-w.placeCh:
			if err := w.PlaceEnv(ctx, req.ouID, req.env); err != nil {
				w.logger.Error("Failed to place gateway artifacts in a newly mapped environment; the placement pass will retry",
					"ouID", req.ouID, "environmentUUID", req.env, "error", err)
			}
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// PlaceEnv places the environment's unplaced records on their gateways and renders what it placed. It runs after a
// gateway is mapped to the environment.
func (w *Worker) PlaceEnv(ctx context.Context, ouID string, env uuid.UUID) error {
	return w.place(ctx, ouID, &env)
}

// PlaceAll places every unplaced record in the fleet: it covers crashes and gateway role changes made outside a
// mapping.
func (w *Worker) PlaceAll(ctx context.Context) error {
	return w.place(ctx, "", nil)
}

func (w *Worker) placeAll(ctx context.Context) {
	if err := w.PlaceAll(ctx); err != nil {
		w.logger.Error("Placement pass failed", "error", err)
	}
}

func (w *Worker) place(ctx context.Context, ouID string, env *uuid.UUID) error {
	var errs []error
	for _, k := range w.core.kinds {
		kind, ok := k.(EnvironmentKind)
		if !ok {
			continue
		}
		for afterKey := ""; ; {
			records, err := kind.UnplacedRecords(ctx, ouID, env, afterKey, placeBatch)
			if err != nil {
				errs = append(errs, err)
				break
			}
			for _, record := range records {
				errs = append(errs, w.placeRecord(ctx, kind, record))
			}
			if len(records) < placeBatch {
				break
			}
			afterKey = records[len(records)-1].Key
		}
	}
	return errors.Join(errs...)
}

// placeRecord places one record on its gateway and renders it at once. A record whose environment has no
// suitable gateway is left until one is mapped.
func (w *Worker) placeRecord(ctx context.Context, kind EnvironmentKind, record Record) error {
	var placed *models.Placement
	err := w.core.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		gateway, ok, err := w.core.target(ctx, tx, kind, record)
		if err != nil || !ok {
			return err
		}
		p := models.Placement{ArtifactRef: record.ArtifactRef, GatewayUUID: gateway}
		inserted, err := w.core.repo.InsertPending(ctx, tx, p, placedReason)
		if inserted {
			placed = &p
		}
		return err
	})
	if err != nil || placed == nil {
		return err
	}
	return w.core.renderOwed(ctx, placed.OUID, placed.ArtifactUUID, []uuid.UUID{placed.GatewayUUID}, ignoreRenderFailed)
}

// driftLoop checks for drift on interval boundaries, so the replicas' ticks line up and the scan lock leaves
// one scan per interval.
func (w *Worker) driftLoop(ctx context.Context) {
	defer w.done.Done()
	interval := w.core.cfg.DriftInterval
	for {
		untilBoundary := interval - time.Duration(time.Now().UnixNano()%int64(interval))
		select {
		case <-time.After(untilBoundary):
			if _, err := w.CheckDrift(ctx); err != nil {
				w.logger.Error("Drift check failed", "error", err)
			}
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// CheckDrift marks DEPLOYED rows whose inputs moved for a re-render, for every kind with a DriftChecker. One
// replica scans at a time: scanned is false when another holds the scan. The scan holds a session lock on one
// pinned connection, so no transaction stays open across the external drift calls.
func (w *Worker) CheckDrift(ctx context.Context) (scanned bool, err error) {
	err = w.core.db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		locked, err := repositories.TryLockDriftScan(conn)
		if err != nil || !locked {
			return err
		}
		scanned = true
		defer w.unlockDriftScan(ctx, conn)
		return w.scanForDrift(ctx)
	})
	return scanned, err
}

// unlockDriftScan ignores ctx's cancellation: a scan cut short by it must still release the lock. A lock that
// won't release would outlive the scan on its pooled connection, so that connection is discarded.
func (w *Worker) unlockDriftScan(ctx context.Context, conn *gorm.DB) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), driftUnlockTimeout)
	defer cancel()
	err := repositories.UnlockDriftScan(conn.WithContext(ctx))
	if err == nil {
		return
	}
	w.logger.Warn("Failed to release the drift scan lock; discarding its connection", "error", err)
	if err := discardConnection(conn); err != nil {
		w.logger.Error("Failed to discard the connection holding the drift scan lock; the lock stays until it closes", "error", err)
	}
}

// discardConnection makes database/sql close a pinned connection instead of pooling it, which ends its session.
func discardConnection(conn *gorm.DB) error {
	pinned, ok := conn.Statement.ConnPool.(*sql.Conn)
	if !ok {
		return fmt.Errorf("pinned connection is a %T, not a *sql.Conn", conn.Statement.ConnPool)
	}
	return pinned.Raw(func(any) error { return driver.ErrBadConn })
}

func (w *Worker) scanForDrift(ctx context.Context) error {
	drifted := 0
	for _, kind := range w.core.kinds {
		checker, ok := kind.(DriftChecker)
		if !ok {
			continue
		}
		for _, artifactKind := range artifactKindsOrName(kind) {
			n, err := w.checkDrift(ctx, checker, artifactKind)
			drifted += n
			if err != nil {
				return err
			}
		}
	}
	if drifted > 0 {
		w.logger.Info("Re-rendering drifted gateway deployments", "count", drifted)
		w.Nudge()
	}
	return nil
}

func (w *Worker) checkDrift(ctx context.Context, checker DriftChecker, artifactKind string) (int, error) {
	drifted := 0
	for afterKey := ""; ; {
		placements, err := w.core.repo.ListDeployedForDrift(ctx, artifactKind, afterKey, driftBatch)
		if err != nil {
			return drifted, err
		}
		for _, p := range placements {
			reason, isDrifted, err := checker.Drifted(ctx, p)
			if err != nil {
				w.logger.Warn("Couldn't check a deployment for drift", "artifactUUID", p.ArtifactUUID,
					"gatewayUUID", p.GatewayUUID, "error", err)
				continue
			}
			if !isDrifted {
				continue
			}
			if _, err := w.core.repo.QueueRender(ctx, nil, p.OUID, p.ArtifactUUID, []uuid.UUID{p.GatewayUUID}, reason,
				repositories.QueueScopePlaced); err != nil {
				return drifted, err
			}
			drifted++
		}
		if len(placements) < driftBatch {
			return drifted, nil
		}
		afterKey = repositories.DriftKey(placements[len(placements)-1])
	}
}
