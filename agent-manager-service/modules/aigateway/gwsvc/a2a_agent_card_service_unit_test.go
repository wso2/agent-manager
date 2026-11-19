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

package gwsvc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/ssrf"
)

// proxyHooksStub records what the card service hands the proxy service, which owns every external-source write.
type proxyHooksStub struct {
	stored    []string
	removed   int
	refreshed int
	err       error
}

func (s *proxyHooksStub) StoreCardSource(_ context.Context, _ *modkit.Agent, _, _ string, _ *modkit.Environment, sourceURL string) error {
	s.stored = append(s.stored, sourceURL)
	return s.err
}

func (s *proxyHooksStub) RemoveCardSource(context.Context, *modkit.Agent, string, string, *modkit.Environment) error {
	s.removed++
	return s.err
}

func (s *proxyHooksStub) RefreshCardSource(context.Context, *modkit.Agent, string, string, *modkit.Environment) error {
	s.refreshed++
	return s.err
}

// cardServiceFor builds the service over an agent of the given provisioning and subtype.
const cardTestEnvUUID = "55555555-5555-5555-5555-555555555555"

func cardServiceFor(provisioning, subType string) (*a2aAgentCardService, *gwrepomocks.A2AAgentCardRepositoryMock, *gwtest.Core) {
	return cardServiceWithHooks(provisioning, subType, &proxyHooksStub{})
}

func cardServiceWithHooks(
	provisioning, subType string, hooks A2AProxyHooks,
) (*a2aAgentCardService, *gwrepomocks.A2AAgentCardRepositoryMock, *gwtest.Core) {
	oc := &gwtest.Core{
		GetAgentFunc: func(_ context.Context, _, project, name string) (*modkit.Agent, error) {
			return &modkit.Agent{
				ID: "agent-uuid", Name: name, ProjectName: project,
				Provisioning: provisioning,
			}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*modkit.Environment, error) {
			return &modkit.Environment{ID: cardTestEnvUUID, Name: name}, nil
		},
	}
	repo := &gwrepomocks.A2AAgentCardRepositoryMock{
		EnqueueFunc:           func(context.Context, *gwmodels.A2AAgentCard) error { return nil },
		RequeueFunc:           func(context.Context, string, string, string, string) error { return nil },
		DeleteForAgentEnvFunc: func(context.Context, string, string, string, string) error { return nil },
	}
	return NewA2AAgentCardService(oc, repo, hooks, testLogger(), tierPermissions()).(*a2aAgentCardService), repo, oc
}

func TestGetA2AAgentCardReturnsTheStoredRow(t *testing.T) {
	svc, repo, oc := cardServiceFor("internal", "a2a-agent")
	stored := &gwmodels.A2AAgentCard{Status: gwmodels.A2AAgentCardStatusFetched, Card: json.RawMessage(validTestCard)}
	repo.GetFunc = func(context.Context, string, string, string, string) (*gwmodels.A2AAgentCard, error) {
		return stored, nil
	}
	oc.GetEnvironmentFunc = nil // a stored row proves the environment
	oc.GetAgentFunc = nil       // and the agent, so a 5s poll costs no OpenChoreo call

	got, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	require.NoError(t, err)
	assert.Same(t, stored, got)
}

// No row means no fetch is queued, so a synthetic pending would be polled forever.
func TestGetA2AAgentCardIsNotFoundForAPlatformAgentWithNoRow(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*gwmodels.A2AAgentCard, error) {
		return nil, gwrepo.ErrA2AAgentCardNotFound
	}

	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, gwerr.ErrAgentCardNotFound)
	assert.Empty(t, repo.EnqueueCalls(), "a read never queues a fetch")
}

func TestGetA2AAgentCardIsNotFoundForAnExternalAgentWithNoSource(t *testing.T) {
	svc, repo, _ := cardServiceFor("external", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*gwmodels.A2AAgentCard, error) {
		return nil, gwrepo.ErrA2AAgentCardNotFound
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, gwerr.ErrAgentCardNotFound)
}

func TestGetA2AAgentCardIsNotFoundForANonA2AAgent(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "chat-api")
	repo.GetFunc = func(context.Context, string, string, string, string) (*gwmodels.A2AAgentCard, error) {
		return nil, gwrepo.ErrA2AAgentCardNotFound
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, gwerr.ErrAgentCardNotFound)
}

func TestGetA2AAgentCardDoesNotMaskRealErrors(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*gwmodels.A2AAgentCard, error) {
		return nil, assert.AnError
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, gwerr.ErrAgentCardNotFound)
}

func TestGetA2AAgentCardMapsAMissingAgent(t *testing.T) {
	svc, repo, oc := cardServiceFor("internal", "a2a-agent")
	repo.GetFunc = func(context.Context, string, string, string, string) (*gwmodels.A2AAgentCard, error) {
		return nil, gwrepo.ErrA2AAgentCardNotFound
	}
	oc.GetAgentFunc = func(context.Context, string, string, string) (*modkit.Agent, error) {
		return nil, gwerr.ErrAgentNotFound
	}
	_, err := svc.GetA2AAgentCard(context.Background(), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, gwerr.ErrAgentNotFound)
}

// The proxy service requeues or republishes under its lock; a plain requeue here could un-park a proxied card.
func TestRefreshA2AAgentCardHandsAnExternalAgentToTheProxyService(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, repo, _ := cardServiceWithHooks("external", "a2a-agent", hooks)
	repo.RequeueFunc = nil

	require.NoError(t, svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev"))
	assert.Equal(t, 1, hooks.refreshed)
}

func TestRefreshA2AAgentCardDoesNotMaskAProxyServiceFailure(t *testing.T) {
	hooks := &proxyHooksStub{err: assert.AnError}
	svc, _, _ := cardServiceWithHooks("external", "a2a-agent", hooks)
	assert.ErrorIs(t, svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev"), assert.AnError)
}

func TestRefreshA2AAgentCardRejectsANonA2AAgent(t *testing.T) {
	svc, repo, _ := cardServiceFor("internal", "custom-api")
	err := svc.RefreshA2AAgentCard(tierGrantedCtx(t), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, gwerr.ErrAgentNotA2A)
	assert.Empty(t, repo.EnqueueCalls())
}

func TestRefreshA2AAgentCardEnforcesTheEnvironmentTier(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, repo, oc := cardServiceWithHooks("external", "a2a-agent", hooks)
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*modkit.Environment, error) {
		return &modkit.Environment{Name: name, IsProduction: true}, nil
	}
	err := svc.RefreshA2AAgentCard(tierCtx(t, gwperm.DeployNonProduction), "org", "proj", "agent", "prod")
	assert.ErrorIs(t, err, httpx.ErrForbidden)
	assert.Empty(t, repo.EnqueueCalls())
	assert.Zero(t, hooks.refreshed)
}

// The proxy service stores the URL under its lock, whether or not the environment is proxied.
func TestSetA2ACardSourceHandsTheTrimmedURLToTheProxyService(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, repo, _ := cardServiceWithHooks("external", "a2a-agent", hooks)
	repo.EnqueueFunc = nil // a write outside the proxy service's lock could race a switch to proxied
	const url = "https://93.184.215.14/.well-known/agent-card.json"

	require.NoError(t, svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", "  "+url+" "))

	assert.Equal(t, []string{url}, hooks.stored)
}

// Any external agent can register a card URL; registering one is what makes it an A2A agent there.
func TestSetA2ACardSourceAcceptsAnyExternalAgent(t *testing.T) {
	const url = "https://93.184.215.14/.well-known/agent-card.json"
	for _, subType := range []string{"", "custom-api", "a2a-agent"} {
		t.Run("subtype "+subType, func(t *testing.T) {
			hooks := &proxyHooksStub{}
			svc, _, _ := cardServiceWithHooks("external", subType, hooks)
			require.NoError(t, svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", url))
			assert.Equal(t, []string{url}, hooks.stored)
		})
	}
}

func TestSetA2ACardSourceRejects(t *testing.T) {
	cases := []struct {
		name, provisioning, subType, url string
		want                             error
	}{
		{"platform agent", "internal", "a2a-agent", "https://93.184.215.14/c", gwerr.ErrAgentCardSourceNotExternal},
		{"loopback URL", "external", "a2a-agent", "http://127.0.0.1/c", ssrf.ErrInvalidURL},
		{"non-http scheme", "external", "a2a-agent", "ftp://93.184.215.14/c", ssrf.ErrInvalidURL},
		{"empty URL", "external", "a2a-agent", "   ", ssrf.ErrInvalidURL},
		{"too long", "external", "a2a-agent", "https://93.184.215.14/" + strings.Repeat("a", 2048), ssrf.ErrInvalidURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hooks := &proxyHooksStub{}
			svc, _, _ := cardServiceWithHooks(tc.provisioning, tc.subType, hooks)
			err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", tc.url)
			assert.ErrorIs(t, err, tc.want)
			assert.Empty(t, hooks.stored)
		})
	}
}

func TestSetA2ACardSourceRejectsAnUnknownEnvironment(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	oc.GetEnvironmentFunc = func(context.Context, string, string) (*modkit.Environment, error) {
		return nil, gwerr.ErrEnvironmentNotFound
	}
	err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "nope", "https://93.184.215.14/c")
	assert.ErrorIs(t, err, gwerr.ErrEnvironmentNotFound)
	assert.Empty(t, repo.EnqueueCalls())
}

func TestDeleteA2ACardSourceHandsRemovalToTheProxyService(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, repo, _ := cardServiceWithHooks("external", "a2a-agent", hooks)
	repo.DeleteForAgentEnvFunc = nil // the proxy service deletes the row under its lock

	require.NoError(t, svc.DeleteA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev"))
	assert.Equal(t, 1, hooks.removed)
}

func TestDeleteA2ACardSourceRejectsAPlatformAgent(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, _, _ := cardServiceWithHooks("internal", "a2a-agent", hooks)
	err := svc.DeleteA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev")
	assert.ErrorIs(t, err, gwerr.ErrAgentCardSourceNotExternal)
	assert.Zero(t, hooks.removed)
}

// Unresolvable and internal hosts must look alike, or the reply is an internal-DNS oracle.
func TestSetA2ACardSourceHidesWhyAHostIsRefused(t *testing.T) {
	svc, _, _ := cardServiceFor("external", "a2a-agent")
	ctx := tierGrantedCtx(t)

	internal := svc.SetA2ACardSource(ctx, "org", "proj", "agent", "dev", "https://10.0.0.1/c")
	unresolvable := svc.SetA2ACardSource(ctx, "org", "proj", "agent", "dev", "https://nope.invalid/c")

	require.ErrorIs(t, internal, ssrf.ErrInvalidURL)
	require.ErrorIs(t, unresolvable, ssrf.ErrInvalidURL)
	assert.Equal(t, internal.Error(), unresolvable.Error())
	assert.NotContains(t, unresolvable.Error(), "lookup")
}

// Only callers allowed on the environment may probe URLs through it.
func TestSetA2ACardSourceChecksTheTierBeforeTheURL(t *testing.T) {
	svc, repo, oc := cardServiceFor("external", "a2a-agent")
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*modkit.Environment, error) {
		return &modkit.Environment{Name: name, IsProduction: true}, nil
	}
	err := svc.SetA2ACardSource(tierCtx(t, gwperm.DeployNonProduction), "org", "proj", "agent", "prod", "https://10.0.0.1/c")
	assert.ErrorIs(t, err, httpx.ErrForbidden)
	assert.Empty(t, repo.EnqueueCalls())
}

// The OpenAPI maxLength and the console both count characters, not bytes.
func TestSetA2ACardSourceCountsURLLengthInCharacters(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, _, _ := cardServiceWithHooks("external", "a2a-agent", hooks)
	base := "https://93.184.215.14/"
	url := base + strings.Repeat("é", 2048-len(base))

	require.NoError(t, svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", url))
	assert.Len(t, hooks.stored, 1)
}

// a2aCapable is the one gate the card, proxy, publish, deploy-settings and delete paths share.
func TestA2ACapable(t *testing.T) {
	cases := []struct {
		provisioning, subType string
		want                  bool
	}{
		{"external", "", true},
		{"external", "custom-api", true},
		{"external", "a2a-agent", true},
		{"internal", "a2a-agent", false},
		{"internal", "chat-api", false},
		{"internal", "custom-api", false},
	}
	for _, tc := range cases {
		t.Run(tc.provisioning+"/"+tc.subType, func(t *testing.T) {
			agent := &modkit.Agent{
				Provisioning: tc.provisioning,
			}
			assert.Equal(t, tc.want, a2aCapable(agent))
		})
	}
}

// OpenChoreo resolves an agent by name across the organization, so one owned by another project is not found.
func TestSetA2ACardSourceRejectsAnAgentFromAnotherProject(t *testing.T) {
	hooks := &proxyHooksStub{}
	svc, _, oc := cardServiceWithHooks("external", "", hooks)
	oc.GetAgentFunc = func(_ context.Context, _, _, name string) (*modkit.Agent, error) {
		return &modkit.Agent{Name: name, ProjectName: "other", Provisioning: "external"}, nil
	}
	err := svc.SetA2ACardSource(tierGrantedCtx(t), "org", "proj", "agent", "dev", "https://93.184.215.14/.well-known/agent-card.json")
	assert.ErrorIs(t, err, gwerr.ErrAgentNotFound)
	assert.Empty(t, hooks.stored)
}
