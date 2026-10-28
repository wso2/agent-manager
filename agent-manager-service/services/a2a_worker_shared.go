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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// a2aLastErrorMaxRunes keeps a stored last_error readable in the console.
const a2aLastErrorMaxRunes = 1024

// sanitizeA2ALastError makes msg storable in a Postgres TEXT column and bounded in length.
func sanitizeA2ALastError(msg string) string {
	msg = strings.ReplaceAll(strings.ToValidUTF8(msg, "�"), "\x00", "")
	if runes := []rune(msg); len(runes) > a2aLastErrorMaxRunes {
		msg = string(runes[:a2aLastErrorMaxRunes-1]) + "…"
	}
	return msg
}

// a2aPublicError is a failure whose message is safe to show any reader of the agent.
type a2aPublicError struct {
	msg   string
	cause error
}

func (e *a2aPublicError) Error() string { return e.msg }
func (e *a2aPublicError) Unwrap() error { return e.cause }

// newA2APublicError marks msg user-safe; cause stays for the log and errors.Is.
func newA2APublicError(msg string, cause error) error {
	return &a2aPublicError{msg: msg, cause: cause}
}

// prefixA2APublicError keeps a public error public under prefix; anything else stays internal.
func prefixA2APublicError(prefix string, err error) error {
	var public *a2aPublicError
	if errors.As(err, &public) {
		return newA2APublicError(prefix+": "+public.msg, err)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// a2aLogError renders err for logs only, appending the hidden cause of a public error.
func a2aLogError(err error) string {
	if err == nil {
		return ""
	}
	var public *a2aPublicError
	if errors.As(err, &public) && public.cause != nil {
		return err.Error() + " (cause: " + a2aLogError(public.cause) + ")"
	}
	return err.Error()
}

const (
	a2aPublishInternalError = "publishing to the gateway failed because of an internal error; check the service logs"
	a2aCardInternalError    = "the agent card could not be fetched because of an internal error; check the service logs"
)

// userFacingA2AError is cause's public message, or fallback when cause carries internal detail.
func userFacingA2AError(cause error, fallback string) string {
	var public *a2aPublicError
	if errors.As(cause, &public) {
		return sanitizeA2ALastError(public.Error())
	}
	return fallback
}

// warnIfInternalA2AError logs cause as msg when lastErr is the fallback that hides it from the user.
func warnIfInternalA2AError(logger *slog.Logger, lastErr, fallback string, cause error, msg string, args ...any) {
	if lastErr == fallback {
		logger.Warn(msg, append(args, "error", a2aLogError(cause))...)
	}
}

// a2aWorkerConcurrency caps how many rows of a claimed batch run at once.
const a2aWorkerConcurrency = 4

// drainWithinLease runs each row, a2aWorkerConcurrency at a time, and starts none once the lease budget is spent.
func drainWithinLease[T any](
	ctx context.Context, logger *slog.Logger, deadline time.Time, rows []T, label string, each func(context.Context, T),
) {
	batchCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, a2aWorkerConcurrency)
	for i, row := range rows {
		slots <- struct{}{}
		if batchCtx.Err() != nil {
			logger.Info(label+" batch ran out of lease; leaving the rest for a later claim", "remaining", len(rows)-i)
			return
		}
		wg.Go(func() {
			defer func() { <-slots }()
			each(batchCtx, row)
		})
	}
}

// a2aAttemptContexts bounds an attempt by timeout and returns a context to record its outcome on even after that.
func a2aAttemptContexts(ctx context.Context, timeout time.Duration) (attempt, record context.Context, cancel context.CancelFunc) {
	attempt, cancel = context.WithTimeout(ctx, timeout)
	return attempt, context.WithoutCancel(ctx), cancel
}

// findIngressGateway is the environment's INGRESS gateway, where inbound A2A traffic belongs, or nil while none is
// mapped; egress gateways host outbound LLM/MCP artifacts.
func findIngressGateway(gatewayRepo repositories.GatewayRepository, ouID string, envUUID uuid.UUID) (*models.Gateway, error) {
	envID := envUUID.String()
	gateways, err := gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID:      ouID,
		FunctionalityTypeIn: models.IngressGatewayRoles,
		EnvironmentID:       &envID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways for environment %s: %w", envID, err)
	}
	for _, gw := range gateways {
		if gw != nil {
			return gw, nil
		}
	}
	return nil, nil //nolint:nilnil // No ingress gateway is mapped yet; the caller decides whether that is an error.
}

// resolveIngressGateway is findIngressGateway for a worker: the bootstrap job registers the gateway after the
// environment exists, so absence is a timing condition the attempt retries on.
func resolveIngressGateway(gatewayRepo repositories.GatewayRepository, ouID string, envUUID uuid.UUID) (*models.Gateway, error) {
	gateway, err := findIngressGateway(gatewayRepo, ouID, envUUID)
	if err != nil {
		return nil, err
	}
	if gateway == nil {
		return nil, newA2APublicError("no ingress gateway is mapped to this environment yet", nil)
	}
	return gateway, nil
}

// a2aGatewayURL is where gateway serves the agent: its public vhost plus the agent's context path.
func a2aGatewayURL(gateway *models.Gateway, agentName string) string {
	return strings.TrimRight(buildPublicProxyURL(gateway, nil), "/") + a2aAgentContext(agentName)
}
