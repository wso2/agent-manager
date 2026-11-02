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
	"encoding/json"
	"fmt"
	"log/slog"

	"time"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	monitorAlertFanoutConsumer   = events.MonitorAlertFanoutConsumer
	monitorAlertFanoutRetryDelay = 5 * time.Second
)

// MonitorAlertProcessor evaluates runs and queues durable destination deliveries.
type MonitorAlertProcessor interface {
	Start(ctx context.Context) error
	Stop() error
}

type monitorAlertProcessor struct {
	bus  events.Bus
	repo repositories.MonitorAlertRepository

	logger *slog.Logger

	sub events.Subscription

	cancel context.CancelFunc
}

// NewMonitorAlertProcessor creates the monitor alert processor. bus may be
// nil (events off), in which case Start does nothing.
func NewMonitorAlertProcessor(
	bus events.Bus,
	repo repositories.MonitorAlertRepository,

	logger *slog.Logger,
) MonitorAlertProcessor {
	return &monitorAlertProcessor{
		bus:  bus,
		repo: repo,

		logger: logger.With("component", "monitor-alerting"),
	}
}

func (n *monitorAlertProcessor) Start(ctx context.Context) error {
	if n.bus == nil {
		n.logger.Info("Monitor alerting not started: events are off")
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	sub, err := n.bus.SubscribeEvents(runCtx, monitorAlertFanoutConsumer, func(msg events.Message) {
		n.handleEvent(runCtx, msg)
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to consume events: %w", err)
	}
	n.sub = sub

	n.logger.Info("Monitor alerting started")
	return nil
}

func (n *monitorAlertProcessor) Stop() error {
	if n.cancel != nil {
		n.cancel()
	}
	if n.sub != nil {
		n.sub.Stop()
	}

	return nil
}

// handleEvent evaluates the shared alerts for a monitor run.
func (n *monitorAlertProcessor) handleEvent(ctx context.Context, msg events.Message) {
	var e events.Event
	if err := json.Unmarshal(msg.Data(), &e); err != nil || e.ID == "" {
		n.logger.Error("Dropping malformed event", "error", err)
		_ = msg.Drop()
		return
	}
	run, ok := parseMonitorRunEvent(e, msg.Data())
	if !ok {
		_ = msg.Ack() // not about a monitor run
		return
	}
	log := n.logger.With("eventId", e.ID, "monitorId", run.MonitorID)

	watches, err := n.repo.ListAlerts(ctx, run.MonitorID)
	if err != nil {
		log.Error("Failed to load monitor watches; will retry", "error", err)
		_ = msg.Retry(monitorAlertFanoutRetryDelay)
		return
	}
	for i := range watches {
		w := &watches[i]
		if w.OUID != run.OrgID {
			continue
		}
		err := n.repo.WithAlert(ctx, w.ID, func(current *models.MonitorAlert, txRepo repositories.MonitorAlertRepository) error {
			if current.OUID != run.OrgID {
				return nil
			}
			outcome := evaluateAlert(current, run)
			if err := txRepo.QueueAlerts(ctx, current, outcome.Notifications, e); err != nil {
				return err
			}
			if outcome.Evaluated {
				return txRepo.UpdateAlertBreached(ctx, current.ID, outcome.Breached, run.Time)
			}
			return nil
		})
		if err != nil {
			log.Error("Failed to evaluate monitor watch; will retry", "watchId", w.ID, "error", err)
			_ = msg.Retry(monitorAlertFanoutRetryDelay)
			return
		}
	}
	deliveries, err := n.repo.AlertDeliveries(ctx, e.ID)
	if err != nil {
		_ = msg.Retry(monitorAlertFanoutRetryDelay)
		return
	}
	for _, delivery := range deliveries {
		body, err := json.Marshal(eventDeliveryJob{DestinationID: delivery.DestinationID.String(), Event: delivery.Payload})
		if err != nil {
			_ = msg.Retry(monitorAlertFanoutRetryDelay)
			return
		}
		if err := n.bus.Enqueue(ctx, events.QueueEventDeliveries, delivery.EventID+":"+delivery.DestinationID.String(), body); err != nil {
			_ = msg.Retry(monitorAlertFanoutRetryDelay)
			return
		}
	}
	_ = msg.Ack()
}
