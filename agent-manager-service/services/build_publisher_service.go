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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
)

// BuildPublisherService performs the OpenChoreo writes a source build's workload
// step used to make itself: it takes the Workload the step generated, verifies the
// build belongs to the calling organization, and publishes the Workload for it.
//
// The caller is a build holding a build token for its organization (see
// jwtassertion.BuildClientAuthMiddleware and BuildTokenIssuer), so ouID is that
// organization and every OpenChoreo call made here is scoped to it.
type BuildPublisherService interface {
	PublishWorkload(ctx context.Context, ouID, buildName string, req *models.PublishBuildWorkloadRequest) error
}

// BuildPublisherBackgroundContextSetter receives the service's trusted background
// context once at startup. See buildPublisherService.openChoreoContext.
type BuildPublisherBackgroundContextSetter interface {
	SetBackgroundContext(ctx context.Context)
}

type buildPublisherService struct {
	ocClient client.OpenChoreoClient
	cfg      config.BuildClientConfig
	logger   *slog.Logger

	mu      sync.RWMutex
	baseCtx context.Context
}

// NewBuildPublisherService creates the build publisher service.
func NewBuildPublisherService(ocClient client.OpenChoreoClient, cfg config.Config, logger *slog.Logger) BuildPublisherService {
	return &buildPublisherService{
		ocClient: ocClient,
		cfg:      cfg.BuildClient,
		logger:   logger,
		baseCtx:  context.Background(),
	}
}

// SetBackgroundContext installs the context OpenChoreo calls are derived from: the
// application's background context, which the deployment's
// BackgroundContextDecorator marks as allowed to use the service's own
// credential. Call once at startup, before the HTTP server starts.
func (s *buildPublisherService) SetBackgroundContext(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseCtx = ctx
}

// openChoreoContext returns the context for this request's OpenChoreo calls.
//
// The calls must not carry the request's token: it is the build's, which is
// meant for this endpoint only and must not be used to act on OpenChoreo or a
// platform API in front of it. So the
// context is derived from the background context, which carries no request token
// and lets the OpenChoreo client fall back to this service's own credential, and
// is given the caller's organization (sent as X-Impersonate-Org) so the calls are
// made for that organization only. It is cancelled with the request, and in
// any case after buildPublishTimeout.
func (s *buildPublisherService) openChoreoContext(reqCtx context.Context, ouID string) (context.Context, context.CancelFunc) {
	s.mu.RLock()
	base := s.baseCtx
	s.mu.RUnlock()

	ctx, cancel := context.WithCancelCause(base)
	ctx, cancelTimeout := context.WithTimeoutCause(ctx, buildPublishTimeout, errBuildPublishTimedOut)
	stop := context.AfterFunc(reqCtx, func() { cancel(context.Cause(reqCtx)) })
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: ouID, OuHandle: ""})
	ctx = logger.WithLogger(ctx, logger.GetLogger(reqCtx))
	return ctx, func() {
		stop()
		cancelTimeout()
		cancel(nil)
	}
}

// buildPublishTimeout bounds a publish's OpenChoreo calls. The gateway in front
// of this endpoint typically ends a request after 30s (a gateway request
// timeout set by the deployment), and cutting the connection there would stop
// the publish wherever it happened to be. Stopping first, with room to record
// the outcome and answer, gives the build step a 5xx it retries; the retry
// skips what was already written (see client.PublishBuildWorkload).
var buildPublishTimeout = 25 * time.Second

// errBuildPublishTimedOut is the cause of a publish stopped by buildPublishTimeout.
var errBuildPublishTimedOut = errors.New("build publish did not finish in time")

// terminalBuildStatuses are run states after which the workload step can no
// longer be running, so nothing may publish for the run any more.
var terminalBuildStatuses = []string{
	client.WorkflowStatusSucceeded,
	client.WorkflowStatusFailed,
	client.WorkflowStatusCompleted,
}

// PublishWorkload verifies the build and publishes its Workload.
//
// Errors map to the endpoint's responses: ErrBuildPublisherDisabled (a
// utils.ErrNotFound) when the feature is off, utils.ErrNotFound when the build does
// not exist in the caller's organization, utils.ErrBadRequest for a malformed
// request, ErrBuildNotAgent, ErrBuildWorkflowNotEnabled or ErrAgentNotSourceBuilt
// (each a utils.ErrForbidden) when the build is not a source build of one of the
// organization's agents on a build workflow that takes the build token,
// utils.ErrConflict when the build has already finished, and
// workloadpolicy.ErrSuspended when the deployment's workload guard does not admit
// the organization (a suspended subscription).
func (s *buildPublisherService) PublishWorkload(ctx context.Context, ouID, buildName string, req *models.PublishBuildWorkloadRequest) error {
	if !s.cfg.Enabled {
		return ErrBuildPublisherDisabled
	}
	if ouID == "" {
		return fmt.Errorf("%w: no organization on the build token", utils.ErrForbidden)
	}
	if errs := validation.IsDNS1123Subdomain(buildName); len(errs) > 0 {
		return fmt.Errorf("%w: invalid build name: %s", utils.ErrBadRequest, strings.Join(errs, "; "))
	}
	if req == nil {
		return fmt.Errorf("%w: request body is required", utils.ErrBadRequest)
	}
	image, endpoints, err := parseBuildWorkload(req.Workload)
	if err != nil {
		return err
	}
	overrides, err := parseBuildWorkloadOverrides(req.WorkloadOverrides)
	if err != nil {
		return err
	}

	ocCtx, done := s.openChoreoContext(ctx, ouID)
	defer done()

	// A suspended organization may not publish: publishing would deploy or
	// repin the agent's first environment. Refused before anything is looked up
	// or recorded, and answered as the suspension it is (a final 403 for the
	// build step, not a retryable failure).
	if err := workloadpolicy.Check(ocCtx, ouID); err != nil {
		return err
	}

	run, err := s.ocClient.GetBuildRun(ocCtx, ouID, buildName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return fmt.Errorf("%w: build %q", utils.ErrNotFound, buildName)
		}
		return upstreamFailure("look up build "+buildName, err)
	}
	if run.ProjectName == "" || run.ComponentName == "" {
		return fmt.Errorf("%w: build %q", ErrBuildNotAgent, buildName)
	}
	if !slices.Contains(s.cfg.Workflows, run.WorkflowName) {
		return fmt.Errorf("%w: build %q runs workflow %q", ErrBuildWorkflowNotEnabled, buildName, run.WorkflowName)
	}
	if slices.Contains(terminalBuildStatuses, run.Status) {
		return fmt.Errorf("%w: build %q has already finished (%s)", utils.ErrConflict, buildName, run.Status)
	}

	agent, err := s.ocClient.GetComponent(ocCtx, ouID, run.ProjectName, run.ComponentName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return fmt.Errorf("%w: build %q", ErrBuildNotAgent, buildName)
		}
		return upstreamFailure("look up the agent of build "+buildName, err)
	}
	if agent.KindName != "" || agent.Provisioning.Type != string(utils.InternalAgent) {
		return fmt.Errorf("%w: build %q", ErrAgentNotSourceBuilt, buildName)
	}

	// This route carries no rbac.Permission and the actor is a machine client, so
	// this record is the only account of which build changed the agent's first
	// environment and to which image. Publishing repins what that environment
	// runs, so like agent:deploy it is recorded fail-closed: the intent is written
	// first, and the publish is refused if it cannot be.
	attempt, err := audit.Begin(ctx, audit.ActionBuildWorkloadPublish,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgent, agent.UUID, run.ComponentName),
		audit.Project(run.ProjectName),
		audit.SurfaceOpt(audit.SurfacePublisher),
		audit.Actor(audit.ActorService, buildTokenClientID(ctx), ""),
		audit.AuthMethod("build-token"),
		audit.Detail("agentName", run.ComponentName),
		audit.Detail("buildName", run.Name),
		audit.Detail("imageId", image),
	)
	if err != nil {
		return err
	}
	publishErr := s.ocClient.PublishBuildWorkload(ocCtx, ouID, client.PublishBuildWorkloadRequest{
		RunName:           run.Name,
		ProjectName:       run.ProjectName,
		ComponentName:     run.ComponentName,
		Image:             image,
		Endpoints:         endpoints,
		WorkloadOverrides: overrides,
		HealthChecks:      run.HealthChecks,
	})
	attempt.Complete(ctx, publishErr)
	if publishErr != nil {
		if errors.Is(publishErr, workloadpolicy.ErrSuspended) {
			return publishErr
		}
		if errors.Is(publishErr, client.ErrBuildWorkloadAlreadyPublished) {
			return fmt.Errorf("%w: build %q", ErrBuildWorkloadAlreadyPublished, buildName)
		}
		if errors.Is(publishErr, utils.ErrBadRequest) {
			// OpenChoreo refused the Workload or binding as invalid: the caller's
			// document is at fault, so it is the caller's 400.
			// The upstream error is reported as text: only ErrBadRequest classifies the response.
			return fmt.Errorf("%w: the workload of build %q was rejected: %v", utils.ErrBadRequest, buildName, publishErr) //nolint:errorlint // deliberately not wrapped, see upstreamFailure
		}
		return upstreamFailure("publish the workload of build "+buildName, publishErr)
	}
	s.logger.Info("Published build workload",
		"ouID", ouID, "projectName", run.ProjectName, "agentName", run.ComponentName, "buildName", run.Name)
	return nil
}

// buildTokenClientID names the client the request's build token was issued to,
// the actor of a publish (see jwtassertion.BuildClientAuthMiddleware).
func buildTokenClientID(ctx context.Context) string {
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		return strings.TrimSpace(claims.ClientID)
	}
	return ""
}

// ErrBuildWorkloadAlreadyPublished is returned when the build has already
// published a different Workload: a publish is safe to repeat only with the
// same one. The endpoint answers it with 409.
var ErrBuildWorkloadAlreadyPublished = errors.New("build has already published a different workload")

// ErrBuildPublisherDisabled is returned when build publishing is turned off
// (AMP_BUILD_CLIENT_ENABLED), for example after a rollback while a build that
// was given a token is still running. It wraps utils.ErrNotFound, so the
// endpoint still answers 404.
var ErrBuildPublisherDisabled = fmt.Errorf("%w: build publishing through Agent Manager is turned off", utils.ErrNotFound)

// The reasons a build may not publish. Each wraps utils.ErrForbidden, so the
// endpoint still answers 403; they let it say which check refused the build.
var (
	// ErrBuildNotAgent: the run names no agent, or names one that does not exist
	// in the caller's organization.
	ErrBuildNotAgent = fmt.Errorf("%w: the build does not belong to an agent", utils.ErrForbidden)
	// ErrBuildWorkflowNotEnabled: the run's workflow is not listed in
	// AMP_BUILD_CLIENT_WORKFLOWS.
	ErrBuildWorkflowNotEnabled = fmt.Errorf("%w: the build's workflow does not publish through Agent Manager", utils.ErrForbidden)
	// ErrAgentNotSourceBuilt: the run's agent is not an internal agent built from
	// source (an external agent, or one created from a kind).
	ErrAgentNotSourceBuilt = fmt.Errorf("%w: the agent is not built from source", utils.ErrForbidden)
)

// errBuildPublishUpstream marks a failure of the calls made on the caller's behalf
// (OpenChoreo or a platform API in front of it).
var errBuildPublishUpstream = errors.New("build publisher upstream failure")

// upstreamFailure reports a failed upstream call as this service's own failure.
// The upstream error is kept as text, deliberately not wrapped: its sentinel
// (utils.ErrNotFound for a missing project, utils.ErrForbidden for a refused
// service credential, ...) describes this service's call, not the build's
// request, and must not turn into a 404 or 403 for the build.
func upstreamFailure(operation string, err error) error {
	return fmt.Errorf("%w: failed to %s: %v", errBuildPublishUpstream, operation, err) //nolint:errorlint // deliberately not wrapped, see above
}

// parseBuildWorkload extracts the image and endpoints from the generated Workload.
// Anything else the step could set on the Workload is refused rather than ignored:
// the Workload is inherited by every environment, so configuration must reach a
// binding instead (see amp-generate-workload's Step 2).
func parseBuildWorkload(raw json.RawMessage) (string, map[string]gen.WorkloadEndpoint, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil, fmt.Errorf("%w: workload is required", utils.ErrBadRequest)
	}
	var workload gen.Workload
	if err := decodeStrict(raw, &workload); err != nil {
		return "", nil, fmt.Errorf("%w: invalid workload: %w", utils.ErrBadRequest, err)
	}
	spec := workload.Spec
	if spec == nil || spec.Container == nil || strings.TrimSpace(spec.Container.Image) == "" {
		return "", nil, fmt.Errorf("%w: workload spec.container.image is required", utils.ErrBadRequest)
	}
	c := spec.Container
	if c.Env != nil || c.Files != nil || c.Command != nil || c.Args != nil {
		return "", nil, fmt.Errorf("%w: workload container may set only image; env, files, command and args are not accepted",
			utils.ErrBadRequest)
	}
	if spec.Dependencies != nil {
		return "", nil, fmt.Errorf("%w: workload dependencies are not accepted", utils.ErrBadRequest)
	}
	var endpoints map[string]gen.WorkloadEndpoint
	if spec.Endpoints != nil {
		endpoints = *spec.Endpoints
	}
	for name, ep := range endpoints {
		if strings.TrimSpace(name) == "" {
			return "", nil, fmt.Errorf("%w: workload endpoint name is required", utils.ErrBadRequest)
		}
		if ep.Port < 1 || ep.Port > 65535 {
			return "", nil, fmt.Errorf("%w: workload endpoint %q port %d is out of range", utils.ErrBadRequest, name, ep.Port)
		}
	}
	return strings.TrimSpace(c.Image), endpoints, nil
}

// parseBuildWorkloadOverrides decodes the build's environment configuration; an
// absent or null document is no configuration.
func parseBuildWorkloadOverrides(raw json.RawMessage) (*gen.WorkloadOverrides, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil //nolint:nilnil // documented: no overrides is a valid, empty answer
	}
	var overrides gen.WorkloadOverrides
	if err := decodeStrict(trimmed, &overrides); err != nil {
		return nil, fmt.Errorf("%w: invalid workloadOverrides: %w", utils.ErrBadRequest, err)
	}
	return &overrides, nil
}

// decodeStrict decodes exactly one JSON value into v, rejecting unknown fields.
func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}
