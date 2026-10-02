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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	alertDispatchTickInterval = 15 * time.Second
	alertDispatchBatch        = 50
	// alertDispatchClaimLease must outlast sending the whole claimed batch
	// one after another at the worst-case send time, or another worker could
	// claim the unsent rest mid-batch and deliver them twice. A replica that
	// dies mid-batch leaves its unsent claims to be retried once it expires.
	alertDispatchClaimLease = alertDispatchBatch*alertSendTimeout + time.Minute
)

// alertRetryBackoff is the delay before each retry. Its length plus one is the
// attempt budget: after the last delay, a further failure is final.
var alertRetryBackoff = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// AlertDispatcherService drains the alert delivery outbox.
type AlertDispatcherService interface {
	Start(ctx context.Context) error
	Stop() error
	RunOnce(ctx context.Context)
}

type alertDispatcherService struct {
	alertRepo     repositories.AlertRepository
	sender        AlertSender
	encryptionKey []byte
	logger        *slog.Logger
	now           func() time.Time
	stopCh        chan struct{}
	stopOnce      sync.Once
}

// NewAlertDispatcherService creates the alert delivery worker.
func NewAlertDispatcherService(
	alertRepo repositories.AlertRepository,
	sender AlertSender,
	encryptionKey []byte,
	logger *slog.Logger,
) AlertDispatcherService {
	return &alertDispatcherService{
		alertRepo:     alertRepo,
		sender:        sender,
		encryptionKey: encryptionKey,
		logger:        logger,
		now:           time.Now,
		stopCh:        make(chan struct{}),
		stopOnce:      sync.Once{},
	}
}

func (d *alertDispatcherService) Start(ctx context.Context) error {
	go d.runLoop(ctx)
	d.logger.Info("Alert dispatcher started")
	return nil
}

func (d *alertDispatcherService) Stop() error {
	d.stopOnce.Do(func() {
		close(d.stopCh)
		d.logger.Info("Alert dispatcher stopped")
	})
	return nil
}

func (d *alertDispatcherService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(alertDispatchTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.RunOnce(ctx)
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *alertDispatcherService) RunOnce(ctx context.Context) {
	deliveries, err := d.alertRepo.ClaimDueDeliveries(ctx, d.now(), alertDispatchClaimLease, alertDispatchBatch)
	if err != nil {
		d.logger.Error("Failed to claim alert deliveries", "error", err)
		return
	}
	if len(deliveries) == 0 {
		return
	}

	// Resolve each org's endpoint once per cycle.
	targets := make(map[string]*AlertTarget)
	for i := range deliveries {
		if ctx.Err() != nil {
			return
		}
		d.deliver(ctx, &deliveries[i], targets)
	}
}

func (d *alertDispatcherService) deliver(ctx context.Context, delivery *models.AlertDelivery, targets map[string]*AlertTarget) {
	log := d.logger.With("ouID", delivery.OUID, "deliveryID", delivery.ID, "eventID", delivery.EventID)

	target, cached := targets[delivery.OUID]
	if !cached {
		resolved, err := d.resolveTarget(ctx, delivery.OUID)
		if err != nil {
			// Leave the claim lease to expire; the delivery is retried later.
			log.Error("Failed to resolve alert endpoint", "error", err)
			return
		}
		target = resolved
		targets[delivery.OUID] = target
	}

	attempts := delivery.Attempts + 1
	if target == nil {
		if err := d.alertRepo.MarkDeliveryDead(ctx, delivery.ID, delivery.Attempts, nil,
			"alert endpoint was removed or disabled"); err != nil {
			log.Error("Failed to record alert delivery outcome", "error", err)
		}
		return
	}

	code, sendErr := d.sender.Send(ctx, *target, delivery.EventID, delivery.EventType, []byte(delivery.Payload))
	now := d.now()
	if sendErr == nil {
		if err := d.alertRepo.MarkDeliverySent(ctx, delivery.ID, attempts, code, now); err != nil {
			log.Error("Failed to record alert delivery outcome", "error", err)
		}
		d.recordEndpointOutcome(ctx, delivery.OUID, true, now)
		log.Info("Alert delivered", "eventType", delivery.EventType, "statusCode", code, "attempts", attempts)
		return
	}

	var codePtr *int
	if code != 0 {
		codePtr = &code
	}
	if attempts > len(alertRetryBackoff) || !retryableAlertStatus(code) {
		if err := d.alertRepo.MarkDeliveryDead(ctx, delivery.ID, attempts, codePtr, sendErr.Error()); err != nil {
			log.Error("Failed to record alert delivery outcome", "error", err)
		}
		d.recordEndpointOutcome(ctx, delivery.OUID, false, now)
		log.Warn("Alert delivery failed permanently", "statusCode", code, "attempts", attempts, "error", sendErr)
		return
	}

	next := now.Add(alertRetryBackoff[attempts-1])
	if err := d.alertRepo.MarkDeliveryRetry(ctx, delivery.ID, attempts, codePtr, sendErr.Error(), next); err != nil {
		log.Error("Failed to record alert delivery outcome", "error", err)
	}
	log.Warn("Alert delivery failed, will retry", "statusCode", code, "attempts", attempts, "nextAttemptAt", next, "error", sendErr)
}

// resolveTarget returns the org's decrypted target, or nil when the org has
// no enabled endpoint.
func (d *alertDispatcherService) resolveTarget(ctx context.Context, ouID string) (*AlertTarget, error) {
	endpoint, err := d.alertRepo.GetEndpoint(ctx, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil //nolint:nilnil // nil target means "no endpoint", which callers handle.
		}
		return nil, fmt.Errorf("failed to get alert endpoint: %w", err)
	}
	if !endpoint.Enabled {
		return nil, nil //nolint:nilnil // disabled endpoint is treated as absent.
	}
	target, err := resolveAlertTarget(endpoint, d.encryptionKey)
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (d *alertDispatcherService) recordEndpointOutcome(ctx context.Context, ouID string, success bool, at time.Time) {
	if err := d.alertRepo.RecordEndpointOutcome(ctx, ouID, success, at); err != nil {
		d.logger.Error("Failed to record alert endpoint health", "ouID", ouID, "error", err)
	}
}

// retryableAlertStatus reports whether a failed attempt is worth retrying.
// Transport errors (code 0), 5xx, 408 and 429 are transient; any other 4xx
// means the receiver rejected the request and a retry will not change that.
func retryableAlertStatus(code int) bool {
	if code == 0 || code >= 500 {
		return true
	}
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
}
