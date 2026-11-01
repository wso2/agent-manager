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
	"strings"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

const (
	// predecessorRecheckDelay is how long a promotion waits before looking again
	// at the environment it is promoted from, while that one is not completed.
	// Waiting consumes no attempt.
	predecessorRecheckDelay = 30 * time.Second
	// promotionSourceMissingRecheckDelay is the same wait when the source has no
	// binding at all, for example while the agent is being deleted.
	promotionSourceMissingRecheckDelay = 10 * time.Minute
	// userFixableRetryFloor and userFixableRetryCap bound the backoff of a
	// promotion that waits for a user to change something (a role that references
	// an MCP server not deployed in the target). It doubles from the floor and is
	// never exhausted into a failure by retrying.
	userFixableRetryFloor = 5 * time.Minute
	userFixableRetryCap   = 30 * time.Minute
	// identitySyncLease is how long one replica holds a due re-promotion. A crash
	// is retried once it passes.
	identitySyncLease = 2 * time.Minute
	// identitySyncRetryDelay and identitySyncUserFixableDelay are the waits after
	// a failed re-promotion, for an outage and for a refusal the user has to fix.
	identitySyncRetryDelay        = 2 * time.Minute
	identitySyncUserFixableDelay  = 15 * time.Minute
	identitySyncBatchSize         = 50
	identitySyncEndTimeout        = 10 * time.Second
	identitySyncConcurrency       = 5
	maxPromotionNoteLength        = 300
	promotionTriggerCreation      = "creation"
	promotionTriggerSync          = "sync"
	promotionWaitingNoteFormat    = "Waiting for the agent identity in %s to be set up."
	promotionNotApplicableMessage = "the environment does not receive its identity by promotion"
)

var (
	errPromotionNotApplicable = errors.New(promotionNotApplicableMessage)
	// errPromotionNotReady marks a re-promotion that cannot start yet because the
	// environment it is promoted from has no completed identity.
	errPromotionNotReady = errors.New("the environment the identity is promoted from is not ready")
)

// userFixable reports whether a refusal needs a user to change something, such
// as deploying an MCP server in the target. Such a promotion keeps retrying on a
// long backoff and is never exhausted into a failure. An outage or a refusal of
// the deployment is retried on the normal budget instead.
func (e *IdentityPromotionError) userFixable() bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case IdentityPromotionBlocked, IdentityPromotionConflict, IdentityPromotionMissingValues:
		return true
	case IdentityPromotionUnavailable, IdentityPromotionForbidden:
		return false
	}
	return false
}

// promotionNote renders the refusal for the binding's last error, which the
// Agent ID page shows to the user. It carries the typed message and the number of
// missing values, never the cause and never a value name.
func promotionNote(e *IdentityPromotionError) string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = "the identity promotion was refused"
	}
	runes := []rune(msg)
	msg = strings.ToUpper(string(runes[:1])) + string(runes[1:])
	if len(e.MissingValues) > 0 {
		msg += fmt.Sprintf(" (%d required value(s) are missing)", len(e.MissingValues))
	}
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return capNote(msg)
}

func capNote(note string) string {
	runes := []rune(note)
	if len(runes) <= maxPromotionNoteLength {
		return note
	}
	return string(runes[:maxPromotionNoteLength-3]) + "..."
}

// promotionNoteOf returns the note for a failed promotion, and whether the
// failure is one the user has to fix.
func promotionNoteOf(err error) (note string, userFixable bool) {
	if errors.Is(err, errPromotionNotApplicable) {
		// Retrying cannot help: the placement and the binding disagree, which a
		// deployment change has to settle. It waits on the long backoff.
		return "This environment no longer receives its identity by promotion.", true
	}
	var typed *IdentityPromotionError
	if errors.As(err, &typed) {
		return promotionNote(typed), typed.userFixable()
	}
	return "", false
}

// userFixableRetryDelay doubles the previous wait, from the floor to the cap. The
// previous wait is the gap the last attempt left between its own start and the
// time it set for the next one.
func userFixableRetryDelay(b models.AgentThunderClient) time.Duration {
	delay := userFixableRetryFloor
	if b.NextRetryAt != nil && b.LastAttemptedAt != nil {
		if previous := b.NextRetryAt.Sub(*b.LastAttemptedAt); previous > 0 {
			delay = previous * 2
		}
	}
	return min(max(delay, userFixableRetryFloor), userFixableRetryCap)
}

// attemptPromotion makes one attempt to give a promoted binding its identity: it
// waits, without using an attempt, until the environment it is promoted from is
// completed, then carries the agent application, roles, groups and resource
// servers into this environment, from the first environment. It runs from
// AttemptProvision, for a binding with a promotion source, on the reconciler's
// schedule and right after an agent is created.
func (s *agentThunderProvisioningService) attemptPromotion(ctx context.Context, binding models.AgentThunderClient) {
	// A background attempt has no request behind it: carry the binding's own
	// organization, as the injection reconcile does.
	ctx = agentThunderBindingOrgContext(ctx, binding)
	completed := false
	// A panic would otherwise crash the process: this runs on a detached goroutine
	// or the reconciler's own. It is recorded as one failed attempt, unless the
	// binding was already completed: a completed binding is never moved back.
	defer func() {
		if r := recover(); r != nil {
			if completed {
				s.logger.Error("Recovered from panic after an agent identity promotion completed",
					"bindingID", binding.ID, "agentName", binding.AgentName, "envName", binding.EnvironmentName, "panic", r)
				return
			}
			s.logger.Error("Recovered from panic during agent identity promotion",
				"bindingID", binding.ID, "agentName", binding.AgentName, "envName", binding.EnvironmentName, "panic", r)
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("panic during promotion attempt: %v", r))
		}
	}()

	chain, err := s.resolvePromotionChain(ctx, binding)
	switch {
	case errors.Is(err, errPromotionSourceMissing):
		s.logger.Warn("The environment an agent identity is promoted from has no binding; waiting",
			"ouID", binding.OUID, "projectName", binding.ProjectName, "agentName", binding.AgentName, "envName", binding.EnvironmentName)
		s.deferPromotionWithNote(ctx, binding, promotionSourceMissingRecheckDelay,
			fmt.Sprintf("The agent identity in %s is missing, so %s cannot be set up yet.", binding.PromotionSource, binding.EnvironmentName))
		return
	case err != nil:
		// Nothing was claimed or consumed: the row is due again on the next sweep.
		s.logger.Warn("Cannot read the agent identity bindings for a promotion", "bindingID", binding.ID, "error", err)
		return
	case chain.source.Status == models.AgentThunderStatusFailed:
		s.deferPromotionWithNote(ctx, binding, predecessorRecheckDelay,
			fmt.Sprintf("The agent identity in %s failed to set up; retry it there first.", binding.PromotionSource))
		return
	case chain.source.Status != models.AgentThunderStatusCompleted:
		s.deferPromotion(ctx, binding, predecessorRecheckDelay)
		return
	}

	// The claim makes the inline attempt after creation and the reconciler's sweep
	// exclusive, like the claim of a direct attempt.
	claimed, err := s.repo.ClaimForAttempt(ctx, binding.ID)
	if err != nil {
		s.logger.Error("Failed to claim agent thunder binding for promotion", "bindingID", binding.ID, "error", err)
		return
	}
	if !claimed {
		s.logger.Debug("Agent thunder binding already claimed by another attempt, skipping", "bindingID", binding.ID)
		return
	}

	result, err := s.promoteBinding(ctx, binding, chain, promotionTriggerCreation)
	if err != nil {
		s.recordPromotionFailure(ctx, binding, err)
		return
	}
	completed = true
	// The binding is completed by the promotion itself. A role held back for another
	// agent is noted, and the agents it waits for are told this one is ready.
	if note := heldRolesNote(result.HeldBackRoles, binding.EnvironmentName); note != "" {
		if err := s.repo.SetLastError(ctx, binding.ID, note); err != nil {
			s.logger.Warn("Failed to record the roles held back for other agents", "bindingID", binding.ID, "error", err)
		}
	}
	s.markCoAssignees(ctx, binding, chain.home, chain.source.ThunderAgentID)
	reconcileWorkloadInjection(ctx, s.getWorkloadInjector(), binding, s.logger)
}

// promoteBinding carries the agent's identity into binding's environment from
// the environment it records as its source.
func (s *agentThunderProvisioningService) promoteBinding(ctx context.Context, binding models.AgentThunderClient,
	chain promotionChain, trigger string,
) (AgentIdentityPromotionResult, error) {
	result, err := s.PromoteIdentityResources(ctx, AgentIdentityPromotionRequest{
		OUID: binding.OUID, ProjectName: binding.ProjectName, AgentName: binding.AgentName,
		DefaultEnvironment: chain.home, SourceEnvironment: binding.PromotionSource, TargetEnvironment: binding.EnvironmentName,
		RequestedBy: binding.RequestedBy, Ownership: binding.ProvisioningType, Background: true, Trigger: trigger,
	})
	if err != nil {
		return AgentIdentityPromotionResult{}, err
	}
	if result.NotApplicable {
		return AgentIdentityPromotionResult{}, errPromotionNotApplicable
	}
	return result, nil
}

// deferPromotion leaves a pending binding for a later sweep without using an
// attempt, with a note saying what it waits for.
func (s *agentThunderProvisioningService) deferPromotion(ctx context.Context, binding models.AgentThunderClient, delay time.Duration) {
	s.deferPromotionWithNote(ctx, binding, delay, fmt.Sprintf(promotionWaitingNoteFormat, binding.PromotionSource))
}

func (s *agentThunderProvisioningService) deferPromotionWithNote(ctx context.Context, binding models.AgentThunderClient,
	delay time.Duration, note string,
) {
	if err := s.repo.Defer(ctx, binding.ID, time.Now().Add(delay), note); err != nil {
		s.logger.Warn("Failed to defer an agent identity promotion", "bindingID", binding.ID, "error", err)
	}
}

// recordPromotionFailure records a failed promotion attempt. A refusal the user
// has to fix keeps the binding pending on a growing backoff, with the refusal as
// its last error, and never exhausts it. Anything else uses the normal attempt
// budget.
func (s *agentThunderProvisioningService) recordPromotionFailure(ctx context.Context, binding models.AgentThunderClient, cause error) {
	note, userFixable := promotionNoteOf(cause)
	if !userFixable {
		s.recordFailure(ctx, binding, "", "", cause)
		return
	}
	delay := userFixableRetryDelay(binding)
	s.logger.Warn("Agent identity promotion needs a change before it can complete; it will be retried",
		"ouID", binding.OUID, "projectName", binding.ProjectName, "bindingID", binding.ID,
		"agentName", binding.AgentName, "envName", binding.EnvironmentName, "retryIn", delay, "error", cause)
	if err := s.repo.ReleaseClaim(ctx, binding.ID, time.Now().Add(delay), note); err != nil {
		s.logger.Error("Failed to record the refusal of an agent identity promotion", "bindingID", binding.ID, "error", err)
	}
}

// IdentitySyncRunner is an optional capability of an AgentThunderProvisioningService:
// it re-promotes the bindings whose carried identity changed.
type IdentitySyncRunner interface {
	// SyncDuePromotions re-promotes the bindings whose re-promotion time has
	// passed and returns how many it took.
	SyncDuePromotions(ctx context.Context) int
}

var _ IdentitySyncRunner = (*agentThunderProvisioningService)(nil)

// SyncDuePromotions re-promotes the completed promoted bindings whose sync time
// has passed, with the agent's roles, groups and resource servers as they are now.
// Each is claimed first, so replicas never take the same one.
func (s *agentThunderProvisioningService) SyncDuePromotions(ctx context.Context) int {
	if !s.IdentityPromotionEnabled() {
		return 0
	}
	due, err := s.repo.FindSyncDue(ctx, time.Now(), identitySyncBatchSize)
	if err != nil {
		s.logger.Error("Failed to query agent identities due for a sync", "error", err)
		return 0
	}
	sem := make(chan struct{}, identitySyncConcurrency)
	var wg sync.WaitGroup
	for _, binding := range due {
		sem <- struct{}{}
		wg.Add(1)
		go func(b models.AgentThunderClient) {
			defer wg.Done()
			defer func() { <-sem }()
			s.syncPromotedBinding(ctx, b)
		}(binding)
	}
	wg.Wait()
	return len(due)
}

// syncPromotedBinding re-promotes one completed binding. The marker is cleared
// only if it was not moved while this ran, so a change that arrived meanwhile is
// never lost. A re-promotion never marks anything itself.
func (s *agentThunderProvisioningService) syncPromotedBinding(ctx context.Context, b models.AgentThunderClient) {
	if b.SyncAfter == nil {
		return
	}
	// The work is bounded by the lease it holds, so another replica never runs the
	// same promotion alongside it.
	ctx, cancel := context.WithTimeout(agentThunderBindingOrgContext(ctx, b), identitySyncLease)
	defer cancel()
	lease := time.Now().Add(identitySyncLease).Truncate(time.Microsecond)
	claimed, err := s.repo.ClaimSync(ctx, b.ID, *b.SyncAfter, lease)
	if err != nil {
		s.logger.Error("Failed to claim an agent identity for a sync", "bindingID", b.ID, "error", err)
		return
	}
	if !claimed {
		return
	}
	// Ending the sync must work even when the work ran out its time.
	endCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(ctx), identitySyncEndTimeout)
	}
	finish := func(retryAt *time.Time) {
		fctx, fcancel := endCtx()
		defer fcancel()
		if _, err := s.repo.FinishSync(fctx, b.ID, lease, retryAt); err != nil {
			s.logger.Error("Failed to record the end of an agent identity sync", "bindingID", b.ID, "error", err)
		}
	}
	// A panic is logged, noted on the binding and retried like any other failure.
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Recovered from panic during an agent identity sync",
				"bindingID", b.ID, "agentName", b.AgentName, "envName", b.EnvironmentName, "panic", r)
			retryAt := time.Now().Add(identitySyncRetryDelay)
			finish(&retryAt)
			nctx, ncancel := endCtx()
			defer ncancel()
			if err := s.repo.SetLastError(nctx, b.ID, "Applying the latest role and group changes failed and will be retried."); err != nil {
				s.logger.Warn("Failed to record a failed agent identity sync", "bindingID", b.ID, "error", err)
			}
		}
	}()

	err = s.syncOnce(ctx, b)
	if err == nil {
		finish(nil)
		return
	}
	delay := identitySyncRetryDelay
	note, userFixable := promotionNoteOf(err)
	switch {
	case errors.Is(err, errPromotionNotReady):
		delay = predecessorRecheckDelay
	case userFixable:
		delay = identitySyncUserFixableDelay
	default:
		note = "Applying the latest role and group changes failed and will be retried."
	}
	s.logger.Warn("Agent identity sync failed; it will be retried", "ouID", b.OUID, "projectName", b.ProjectName,
		"bindingID", b.ID, "agentName", b.AgentName, "envName", b.EnvironmentName, "retryIn", delay, "error", err)
	retryAt := time.Now().Add(delay)
	finish(&retryAt)
	if !errors.Is(err, errPromotionNotReady) {
		nctx, ncancel := endCtx()
		defer ncancel()
		if err := s.repo.SetLastError(nctx, b.ID, capNote(note)); err != nil {
			s.logger.Warn("Failed to record a failed agent identity sync", "bindingID", b.ID, "error", err)
		}
	}
}

func (s *agentThunderProvisioningService) syncOnce(ctx context.Context, b models.AgentThunderClient) error {
	chain, err := s.resolvePromotionChain(ctx, b)
	switch {
	case errors.Is(err, errPromotionSourceMissing):
		return fmt.Errorf("%w: %w", errPromotionNotReady, err)
	case err != nil:
		return err
	case chain.source.Status != models.AgentThunderStatusCompleted:
		return errPromotionNotReady
	case chain.source.SyncAfter != nil:
		// The predecessor still has changes to promote, or is promoting them now, so the
		// version it runs is stale. Promoting from it would carry the old content and then
		// clear this marker, and the change would never reach this environment. Wait; the
		// predecessor's own sync finishes first, then this one carries the new content.
		return errPromotionNotReady
	}
	result, err := s.promoteBinding(ctx, b, chain, promotionTriggerSync)
	if err != nil {
		return err
	}
	// The note shows what is held back now, and no longer a failure that is over.
	if err := s.repo.SetLastError(ctx, b.ID, heldRolesNote(result.HeldBackRoles, b.EnvironmentName)); err != nil {
		s.logger.Warn("Failed to record the roles held back for other agents", "bindingID", b.ID, "error", err)
	}
	return nil
}
