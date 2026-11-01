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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// PipelineIdentityEnvironment is one environment of a project's deployment
// pipeline, as far as an agent identity is concerned.
type PipelineIdentityEnvironment struct {
	Name string
	// Source is the environment this one is promoted from. Empty for the first
	// (home) environment, where the identity is created.
	Source string
}

// orderedPipelineIdentityEnvironments lists the pipeline's environments in
// promotion order: the first environment, then each one reachable from it in
// breadth first order. A pipeline that is a DAG is flattened this way, and the
// environment each one was first reached from is recorded as its source. Nothing
// is returned for a pipeline with no first environment.
func orderedPipelineIdentityEnvironments(paths []models.PromotionPath) []PipelineIdentityEnvironment {
	home := findLowestEnvironment(paths)
	if home == "" {
		return nil
	}
	next := map[string][]string{}
	for _, path := range paths {
		for _, target := range path.TargetEnvironmentRefs {
			next[path.SourceEnvironmentRef] = append(next[path.SourceEnvironmentRef], target.Name)
		}
	}
	ordered := []PipelineIdentityEnvironment{{Name: home}}
	seen := map[string]bool{home: true}
	for i := 0; i < len(ordered); i++ {
		for _, target := range next[ordered[i].Name] {
			if seen[target] {
				continue
			}
			seen[target] = true
			ordered = append(ordered, PipelineIdentityEnvironment{Name: target, Source: ordered[i].Name})
		}
	}
	return ordered
}

// AgentIdentityRef identifies one agent for identity provisioning and syncing.
type AgentIdentityRef struct {
	OUID        string
	ProjectName string
	AgentName   string
	Ownership   models.AgentProvisioningType
}

// AgentIdentityPipelineProvisioner is an optional capability of an
// AgentThunderProvisioningService that gives an agent an identity in every
// environment of its project's pipeline: created in the first environment and
// promoted from there, in pipeline order, into the others.
type AgentIdentityPipelineProvisioner interface {
	// ProvisionForAgentInPipeline writes a pending binding for each pipeline
	// environment, then attempts them in order in the background, like
	// ProvisionForAgent. handled is false when no environment of the pipeline
	// receives its identity by promotion (or promotion is not configured): nothing
	// was written and the caller provisions the way it always did.
	ProvisionForAgentInPipeline(ctx context.Context, ref AgentIdentityRef, pipeline []PipelineIdentityEnvironment,
		requestedBy, awaitEnv string) (handled bool)
	// EnsurePipelineBindings writes the pending bindings an agent is missing for
	// its pipeline, for an agent created before the pipeline had them or whose
	// pipeline gained an environment. It never attempts them: the reconciler does.
	// It is safe to repeat. handled is false under the same condition as above.
	EnsurePipelineBindings(ctx context.Context, ref AgentIdentityRef, pipeline []PipelineIdentityEnvironment) (created int, handled bool)
	// IdentityPromotionEnabled reports whether this deployment promotes agent
	// identities at all. It is false in direct (legacy) identity management.
	IdentityPromotionEnabled() bool
}

// AgentIdentityPipelineReader reads the identity pipeline of a project, in
// promotion order.
type AgentIdentityPipelineReader interface {
	ProjectIdentityPipeline(ctx context.Context, ouID, projectName string) ([]PipelineIdentityEnvironment, error)
}

// AgentIdentityPipelineReaderSetter hands the pipeline reader to the reconciler,
// which is built before the OpenChoreo client exists.
type AgentIdentityPipelineReaderSetter interface {
	SetAgentIdentityPipelineReader(AgentIdentityPipelineReader)
}

type openChoreoIdentityPipelineReader struct {
	client client.OpenChoreoClient
}

// NewAgentIdentityPipelineReader reads a project's pipeline from OpenChoreo.
func NewAgentIdentityPipelineReader(ocClient client.OpenChoreoClient) AgentIdentityPipelineReader {
	if ocClient == nil {
		return nil
	}
	return openChoreoIdentityPipelineReader{client: ocClient}
}

func (r openChoreoIdentityPipelineReader) ProjectIdentityPipeline(ctx context.Context, ouID, projectName string,
) ([]PipelineIdentityEnvironment, error) {
	pipeline, err := r.client.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		return nil, translatePipelineError(err)
	}
	return orderedPipelineIdentityEnvironments(pipeline.PromotionPaths), nil
}

var _ AgentIdentityPipelineProvisioner = (*agentThunderProvisioningService)(nil)

// IdentityPromotionEnabled is true once a promotion manager is configured, which
// the application only allows in the control plane identity modes.
func (s *agentThunderProvisioningService) IdentityPromotionEnabled() bool {
	manager, _, _ := s.promotionDependencies()
	return manager != nil
}

// pipelineBindingPlan is what an agent's pipeline needs: one binding per
// environment, each with the environment it is promoted from, or none when the
// environment gets its identity directly.
type pipelineBindingPlan struct {
	steps []PipelineIdentityEnvironment
	// byPromotion marks the environments that receive their identity by promotion.
	byPromotion map[string]bool
}

// planPipelineBindings asks the placement of every pipeline environment. ok is
// false when the pipeline is not a control plane one (no environment is created
// by promotion, or promotion is not configured), and an error when a placement
// cannot be read. Legacy behavior is the caller's fallback in both cases.
func (s *agentThunderProvisioningService) planPipelineBindings(ctx context.Context, ouID string,
	pipeline []PipelineIdentityEnvironment,
) (pipelineBindingPlan, bool, error) {
	plan := pipelineBindingPlan{steps: pipeline, byPromotion: map[string]bool{}}
	if !s.IdentityPromotionEnabled() || len(pipeline) == 0 {
		return plan, false, nil
	}
	promoted := false
	for _, step := range pipeline {
		byPromotion, err := s.IdentityCreatedByPromotion(ctx, ouID, step.Name)
		if err != nil {
			return plan, false, fmt.Errorf("resolve identity placement for %s: %w", step.Name, err)
		}
		if byPromotion {
			plan.byPromotion[step.Name] = true
			promoted = true
		}
	}
	return plan, promoted, nil
}

func (p pipelineBindingPlan) binding(ref AgentIdentityRef, step PipelineIdentityEnvironment, requestedBy string) models.AgentThunderClient {
	b := models.AgentThunderClient{
		OUID: ref.OUID, ProjectName: ref.ProjectName, AgentName: ref.AgentName, EnvironmentName: step.Name,
		ProvisioningType: ref.Ownership, Status: models.AgentThunderStatusPending, RequestedBy: requestedBy,
	}
	if p.byPromotion[step.Name] {
		b.PromotionSource = step.Source
	}
	return b
}

func (s *agentThunderProvisioningService) ProvisionForAgentInPipeline(ctx context.Context, ref AgentIdentityRef,
	pipeline []PipelineIdentityEnvironment, requestedBy, awaitEnv string,
) bool {
	plan, cpPipeline, err := s.planPipelineBindings(ctx, ref.OUID, pipeline)
	if err != nil {
		s.logger.Error("Failed to resolve agent identity placement for the pipeline; provisioning the direct way, "+
			"which cannot create identity in a promoted environment",
			"ouID", ref.OUID, "projectName", ref.ProjectName, "agentName", ref.AgentName, "error", err)
		return false
	}
	if !cpPipeline {
		return false
	}
	bindings := make([]models.AgentThunderClient, 0, len(pipeline))
	for _, step := range pipeline {
		b := plan.binding(ref, step, requestedBy)
		// The same write-ahead as ProvisionForAgent: this row is all the reconciler
		// can later heal from.
		if err := s.upsertWriteAhead(ctx, &b); err != nil {
			s.logger.Error("Failed to write-ahead agent thunder binding after retries",
				"agentName", ref.AgentName, "env", step.Name, "error", err)
			continue
		}
		bindings = append(bindings, b)
	}

	detached := context.WithoutCancel(ctx)
	if awaitEnv != "" {
		background := make([]models.AgentThunderClient, 0, len(bindings))
		for _, b := range bindings {
			if b.EnvironmentName == awaitEnv {
				s.awaitAttempt(detached, b)
				continue
			}
			background = append(background, b)
		}
		bindings = background
	}
	go s.attemptAll(detached, bindings)
	return true
}

func (s *agentThunderProvisioningService) EnsurePipelineBindings(ctx context.Context, ref AgentIdentityRef,
	pipeline []PipelineIdentityEnvironment,
) (int, bool) {
	if !s.IdentityPromotionEnabled() {
		return 0, false
	}
	existing, err := s.repo.FindByAgent(ctx, ref.OUID, ref.ProjectName, ref.AgentName)
	if err != nil {
		s.logger.Warn("Failed to read agent thunder bindings", "agentName", ref.AgentName, "error", err)
		return 0, true
	}
	have := make(map[string]bool, len(existing))
	requestedBy := ""
	for _, row := range existing {
		have[row.EnvironmentName] = true
		if requestedBy == "" {
			requestedBy = row.RequestedBy
		}
	}
	// The first environment's binding is created with the agent. A missing one is
	// not recreated here: nothing could be promoted from it.
	missing := make([]PipelineIdentityEnvironment, 0, len(pipeline))
	for _, step := range pipeline {
		if !have[step.Name] && step.Source != "" {
			missing = append(missing, step)
		}
	}
	if len(missing) == 0 {
		return 0, true
	}
	// The placement is only asked for when something is missing, so an agent that
	// is complete costs one read.
	plan, cpPipeline, err := s.planPipelineBindings(ctx, ref.OUID, pipeline)
	if err != nil {
		s.logger.Warn("Failed to resolve agent identity placement for the pipeline", "agentName", ref.AgentName, "error", err)
		return 0, false
	}
	if !cpPipeline {
		return 0, false
	}
	created := 0
	for _, step := range missing {
		b := plan.binding(ref, step, requestedBy)
		if err := s.upsertWriteAhead(ctx, &b); err != nil {
			s.logger.Error("Failed to write agent thunder binding for the pipeline", "agentName", ref.AgentName, "env", step.Name, "error", err)
			continue
		}
		created++
	}
	return created, true
}

// upsertWriteAhead writes a new pending binding, retrying a momentary DB blip
// like ProvisionForAgent does.
func (s *agentThunderProvisioningService) upsertWriteAhead(ctx context.Context, b *models.AgentThunderClient) error {
	var err error
	for attempt := 0; attempt < writeAheadUpsertAttempts; attempt++ {
		if err = s.repo.Upsert(ctx, b); err == nil {
			return nil
		}
		time.Sleep(writeAheadUpsertRetryDelay)
	}
	return err
}

// promotionChain is where one promoted binding stands: the rows of its agent,
// the first (home) environment its roles and groups are read from, and the row of
// the environment it is promoted from.
type promotionChain struct {
	home   string
	source *models.AgentThunderClient
}

var errPromotionSourceMissing = errors.New("the environment the identity is promoted from has no binding")

// resolvePromotionChain follows the recorded sources from b up to the first
// environment. It fails with errPromotionSourceMissing when a link is absent, and
// the source row it returns may not be completed yet: the caller decides to wait.
func (s *agentThunderProvisioningService) resolvePromotionChain(ctx context.Context, b models.AgentThunderClient) (promotionChain, error) {
	rows, err := s.repo.FindByAgent(ctx, b.OUID, b.ProjectName, b.AgentName)
	if err != nil {
		return promotionChain{}, fmt.Errorf("read agent identity bindings: %w", err)
	}
	byEnv := make(map[string]*models.AgentThunderClient, len(rows))
	for i := range rows {
		byEnv[rows[i].EnvironmentName] = &rows[i]
	}
	source, ok := byEnv[b.PromotionSource]
	if !ok {
		return promotionChain{}, errPromotionSourceMissing
	}
	home := b.PromotionSource
	// Bounded by the number of rows, so a cycle in the recorded sources ends.
	for hops := 0; hops <= len(rows); hops++ {
		row, ok := byEnv[home]
		if !ok {
			return promotionChain{}, errPromotionSourceMissing
		}
		if row.PromotionSource == "" {
			return promotionChain{home: home, source: source}, nil
		}
		home = row.PromotionSource
	}
	return promotionChain{}, errPromotionSourceMissing
}
