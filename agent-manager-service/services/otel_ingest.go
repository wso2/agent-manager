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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// otelIngestHandle names the per-organization trace-ingest artifact. The amp- prefix keeps it clear of
	// the names users give their own artifacts.
	otelIngestHandle  = "amp-otel-ingest"
	otelIngestVersion = "v1.0"
	otelIngestContext = "/otel"
)

// otelIngestClaimHeaders maps agent-token claims to the headers the collector turns into span resource
// attributes, which the observer scopes trace queries by.
var otelIngestClaimHeaders = map[string]string{
	"sub":             "x-user-id",
	"component_uid":   "x-user-component",
	"environment_uid": "x-user-environment",
	"project_uid":     "x-user-project",
	"organization":    "x-user-organization-name",
	"project":         "x-user-project-name",
	"agent":           "x-user-agent-name",
}

// OtelIngestKind is the built-in OTLP trace-ingest API: POST /otel/v1/traces, authenticated by an agent
// token in x-amp-api-key and forwarded to the trace pipeline. Gateways store it as a RestApi.
type OtelIngestKind struct {
	upstreamURL string
	issuer      string
}

// NewOtelIngestKind builds the kind. tokenIssuer is the iss of the agent tokens the API accepts.
func NewOtelIngestKind(upstreamURL, tokenIssuer string) *OtelIngestKind {
	return &OtelIngestKind{upstreamURL: strings.TrimSpace(upstreamURL), issuer: tokenIssuer}
}

func (k *OtelIngestKind) Name() string            { return models.KindRestAPI }
func (k *OtelIngestKind) ArtifactKinds() []string { return []string{models.KindOtelIngest} }
func (k *OtelIngestKind) RendersInline() bool     { return true }

func (k *OtelIngestKind) GatewayArtifactID(artifact models.ArtifactRef) string {
	return artifact.ArtifactUUID.String()
}

// Render is the RestApi; it depends on configuration only.
func (k *OtelIngestKind) Render(_ context.Context, _ models.ArtifactRef) (reconcile.Rendered, error) {
	if k.upstreamURL == "" {
		return reconcile.Rendered{}, errors.New("the trace-ingest upstream URL is not configured")
	}
	content, err := yaml.Marshal(&RestAPIDeploymentYAML{
		ApiVersion: apiVersionA2AAgent,
		Kind:       models.KindRestAPI,
		Metadata:   DeploymentMetadata{Name: otelIngestHandle + "-" + otelIngestVersion},
		Spec: RestAPIDeploymentSpec{
			DisplayName: otelIngestHandle,
			Version:     otelIngestVersion,
			Context:     otelIngestContext,
			Upstream:    RestAPIUpstream{Main: RestAPIUpstreamTarget{URL: k.upstreamURL}},
			Policies: []map[string]interface{}{{
				"name":    "jwt-auth",
				"version": "v1",
				"params": map[string]interface{}{
					"headerName":       "x-amp-api-key",
					"authHeaderPrefix": "",
					"issuers":          []string{k.issuer},
					"claimMappings":    otelIngestClaimHeaders,
				},
			}},
			Operations: []Operation{{Method: "POST", Path: "/v1/traces"}},
		},
	})
	if err != nil {
		return reconcile.Rendered{}, fmt.Errorf("failed to marshal the trace-ingest API: %w", err)
	}
	return renderedDeployment(otelIngestHandle, content), nil
}

func (k *OtelIngestKind) DeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.APIDeploymentEvent{
		APIID: ref.ArtifactUUID.String(), DeploymentID: ref.DeploymentID, PerformedAt: ref.PerformedAt,
	}
	return eventhub.NewGatewayEvent("api.deployed", "CREATE", payload.APIID, payload)
}

func (k *OtelIngestKind) UndeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.APIUndeploymentEvent{APIID: ref.ArtifactUUID.String(), PerformedAt: ref.PerformedAt}
	return eventhub.NewGatewayEvent("api.undeployed", "DELETE", payload.APIID, payload)
}

func (k *OtelIngestKind) DeleteEvent(artifact models.ArtifactRef) eventhub.Event {
	payload := &models.APIDeletionEvent{APIID: artifact.ArtifactUUID.String()}
	return eventhub.NewGatewayEvent("api.deleted", "DELETE", payload.APIID, payload)
}

// OtelIngestPublisher keeps the trace-ingest API deployed on every gateway. As a control-plane deployment
// it is part of what a gateway syncs, so a gateway restart or resync cannot drop it.
type OtelIngestPublisher struct {
	db          *gorm.DB
	kind        *OtelIngestKind
	artifacts   repositories.ArtifactRepository
	deployments repositories.DeploymentRepository
	core        *reconcile.Core
	logger      *slog.Logger
}

// NewOtelIngestPublisher builds the publisher.
func NewOtelIngestPublisher(
	db *gorm.DB, kind *OtelIngestKind, artifacts repositories.ArtifactRepository,
	deployments repositories.DeploymentRepository, core *reconcile.Core, logger *slog.Logger,
) *OtelIngestPublisher {
	return &OtelIngestPublisher{db: db, kind: kind, artifacts: artifacts, deployments: deployments, core: core, logger: logger}
}

// Enabled reports whether a trace-ingest upstream is configured.
func (p *OtelIngestPublisher) Enabled() bool { return p.kind.upstreamURL != "" }

// EnsureOnGateway deploys the trace-ingest API on the gateway unless it already serves the current one.
func (p *OtelIngestPublisher) EnsureOnGateway(ctx context.Context, ouID string, gatewayID uuid.UUID) error {
	if !p.Enabled() {
		return nil
	}
	artifact, err := p.artifact(ctx, ouID)
	if err != nil {
		return err
	}
	placement := models.Placement{
		ArtifactRef: models.ArtifactRef{
			OUID: ouID, ArtifactUUID: artifact.UUID, Kind: models.KindOtelIngest, Handle: artifact.Handle,
		},
		GatewayUUID: gatewayID,
	}
	rendered, err := p.kind.Render(ctx, placement.ArtifactRef)
	if err != nil {
		return err
	}
	current, err := p.deployments.GetPullable(ctx, ouID, artifact.UUID, gatewayID.String())
	if err != nil {
		return err
	}
	if current != nil && bytes.Equal(current.Content, rendered.Content) {
		return nil
	}
	if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := p.core.Deploy(ctx, tx, placement)
		return err
	}); err != nil {
		return fmt.Errorf("failed to deploy the trace-ingest API on gateway %s: %w", gatewayID, err)
	}
	p.logger.Info("Deployed the trace-ingest API", "gatewayID", gatewayID, "ouID", ouID)
	return nil
}

// artifact is the organization's trace-ingest artifact, created on first use.
func (p *OtelIngestPublisher) artifact(ctx context.Context, ouID string) (*models.Artifact, error) {
	existing, err := p.artifacts.GetByHandle(otelIngestHandle, ouID)
	if err == nil {
		if existing.Kind != models.KindOtelIngest {
			return nil, fmt.Errorf("artifact handle %q is taken by a %s", otelIngestHandle, existing.Kind)
		}
		return existing, nil
	}
	if !errors.Is(err, utils.ErrArtifactNotFound) {
		return nil, err
	}
	artifact := &models.Artifact{
		UUID: uuid.New(), Handle: otelIngestHandle, Name: otelIngestHandle, Version: otelIngestVersion,
		Kind: models.KindOtelIngest, OUID: ouID,
	}
	if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return p.artifacts.Create(tx, artifact)
	}); err != nil {
		// Another gateway of the organization may have created it first.
		if again, getErr := p.artifacts.GetByHandle(otelIngestHandle, ouID); getErr == nil {
			return again, nil
		}
		return nil, fmt.Errorf("failed to create the trace-ingest artifact: %w", err)
	}
	return artifact, nil
}
