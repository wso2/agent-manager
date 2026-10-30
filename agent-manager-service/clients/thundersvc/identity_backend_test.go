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

package thundersvc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	backendClient         struct{ ThunderClient }
	backendIdentityClient struct{ EnvIdentityClient }
)

func (*backendClient) ActivateAgentIdentity(context.Context, string) error     { return nil }
func (*backendClient) ActivateMCPResourceServer(context.Context, string) error { return nil }
func (*backendClient) ActivateDeletedMCPResourceServer(context.Context, string) error {
	return nil
}
func (*backendIdentityClient) ActivateAgentIdentity(context.Context, string) error { return nil }
func (*backendIdentityClient) ActivateMCPResourceServer(context.Context, string) error {
	return nil
}

func (*backendIdentityClient) ActivateDeletedMCPResourceServer(context.Context, string) error {
	return nil
}

type legacyBackendStub struct {
	calls         int
	identityCalls int
}

func (s *legacyBackendStub) Resolve(context.Context, string, string, string) (ThunderClient, error) {
	s.calls++
	return &backendClient{}, nil
}

func (s *legacyBackendStub) ResolveIdentity(context.Context, string, string, string) (EnvIdentityClient, error) {
	s.identityCalls++
	return &backendIdentityClient{}, nil
}

type targetResolverStub struct {
	target EnvironmentIdentityTarget
	err    error
}

func (s targetResolverStub) ResolveIdentityTarget(context.Context, string, string, string) (EnvironmentIdentityTarget, error) {
	return s.target, s.err
}

type cpBackendStub struct {
	calls         int
	identityCalls int
	target        EnvironmentIdentityTarget
	err           error
	noActivator   bool
}

func (s *cpBackendStub) Resolve(_ context.Context, target EnvironmentIdentityTarget) (ThunderClient, error) {
	s.calls++
	s.target = target
	if s.err != nil {
		return nil, s.err
	}
	if s.noActivator {
		return &struct{ ThunderClient }{}, nil
	}
	return &backendClient{}, nil
}

func (s *cpBackendStub) ResolveIdentity(_ context.Context, target EnvironmentIdentityTarget) (EnvIdentityClient, error) {
	s.identityCalls++
	s.target = target
	if s.err != nil {
		return nil, s.err
	}
	if s.noActivator {
		return &struct{ EnvIdentityClient }{}, nil
	}
	return &backendIdentityClient{}, nil
}

func cpTarget() EnvironmentIdentityTarget {
	return EnvironmentIdentityTarget{
		Mode: IdentityManagementCPManaged, OUID: "ou-1", OrgNamespace: "org", Environment: "staging",
		DeploymentID: "ou-1", GatewayID: "gateway-stage",
		ManagementURL: "https://cp.example.test", IssuerURL: "https://stage.example.test",
		TokenURL: "https://stage.example.test/oauth2/token", JWKSURL: "https://stage.example.test/oauth2/jwks",
		DefaultEnvironment: true,
	}
}

func TestRoutingEnvThunderResolverDefaultsToLegacy(t *testing.T) {
	legacy := &legacyBackendStub{}
	resolver, err := NewRoutingEnvThunderResolver("", nil, legacy, nil)
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
	require.NoError(t, err)
	_, err = resolver.ResolveIdentity(context.Background(), "ou-1", "org", "staging")
	require.NoError(t, err)
	require.Equal(t, 1, legacy.calls)
	require.Equal(t, 1, legacy.identityCalls)
}

func TestRoutingEnvThunderResolverRoutesCPWithoutLegacyFallback(t *testing.T) {
	legacy := &legacyBackendStub{}
	cp := &cpBackendStub{err: errors.New("CP unavailable")}
	target := cpTarget()
	resolver, err := NewRoutingEnvThunderResolver(IdentityManagementMixed, targetResolverStub{target: target}, legacy, cp)
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
	require.ErrorContains(t, err, "CP unavailable")
	_, err = resolver.ResolveIdentity(context.Background(), "ou-1", "org", "staging")
	require.ErrorContains(t, err, "CP unavailable")
	require.Equal(t, 0, legacy.calls)
	require.Equal(t, 0, legacy.identityCalls)
	require.Equal(t, 1, cp.calls)
	require.Equal(t, 1, cp.identityCalls)
	require.Equal(t, target, cp.target)
}

func TestRoutingEnvThunderResolverRefusesCPManagementOfNonDefaultEnvironment(t *testing.T) {
	legacy := &legacyBackendStub{}
	cp := &cpBackendStub{}
	target := cpTarget()
	target.DefaultEnvironment = false
	resolver, err := NewRoutingEnvThunderResolver(IdentityManagementCPManaged, targetResolverStub{target: target}, legacy, cp)
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
	require.ErrorIs(t, err, ErrIdentityEnvironmentUnsupported)
	require.ErrorIs(t, err, ErrIdentityCapabilityUnavailable)
	_, err = resolver.ResolveIdentity(context.Background(), "ou-1", "org", "staging")
	require.ErrorIs(t, err, ErrIdentityEnvironmentUnsupported)
	require.Zero(t, legacy.calls+legacy.identityCalls, "a refused CP call must not reach the DP")
	require.Zero(t, cp.calls+cp.identityCalls, "a write that cannot reach the environment must not reach the CP")
}

func TestRoutingEnvThunderResolverReportsIdentityCreatedByPromotion(t *testing.T) {
	cases := map[string]struct {
		mode    IdentityManagementMode
		target  func() EnvironmentIdentityTarget
		want    bool
		wantErr bool
	}{
		"CP-managed default environment": {IdentityManagementCPManaged, cpTarget, false, false},
		"CP-managed other environment": {IdentityManagementCPManaged, func() EnvironmentIdentityTarget {
			target := cpTarget()
			target.DefaultEnvironment = false
			return target
		}, true, false},
		"legacy target in mixed mode": {IdentityManagementMixed, func() EnvironmentIdentityTarget {
			return EnvironmentIdentityTarget{Mode: IdentityManagementLegacyDirect, OUID: "ou-1", OrgNamespace: "org", Environment: "staging"}
		}, false, false},
		"legacy mode never consults targets": {IdentityManagementLegacyDirect, cpTarget, false, false},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			resolver, err := NewRoutingEnvThunderResolver(test.mode, targetResolverStub{target: test.target()}, &legacyBackendStub{}, &cpBackendStub{})
			require.NoError(t, err)
			placement, ok := resolver.(EnvironmentIdentityPlacement)
			require.True(t, ok)
			got, err := placement.IdentityCreatedByPromotion(context.Background(), "ou-1", "org", "staging")
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestRoutingEnvThunderResolverDoesNotFallbackWhenTargetResolutionFails(t *testing.T) {
	legacy := &legacyBackendStub{}
	cp := &cpBackendStub{}
	resolver, err := NewRoutingEnvThunderResolver(IdentityManagementMixed,
		targetResolverStub{err: errors.New("target unavailable")}, legacy, cp)
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
	require.ErrorContains(t, err, "target unavailable")
	require.Zero(t, legacy.calls)
	require.Zero(t, cp.calls)
}

func TestRoutingEnvThunderResolverRequiresGatewayActivation(t *testing.T) {
	legacy := &legacyBackendStub{}
	cp := &cpBackendStub{noActivator: true}
	resolver, err := NewRoutingEnvThunderResolver(IdentityManagementCPManaged,
		targetResolverStub{target: cpTarget()}, legacy, cp)
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
	require.ErrorContains(t, err, "exact-gateway activation")
	_, err = resolver.ResolveIdentity(context.Background(), "ou-1", "org", "staging")
	require.ErrorContains(t, err, "exact-gateway activation")
	require.Zero(t, legacy.calls)
	require.Zero(t, legacy.identityCalls)
}

func TestRoutingEnvThunderResolverMixedLegacy(t *testing.T) {
	legacy := &legacyBackendStub{}
	cp := &cpBackendStub{}
	target := cpTarget()
	target.Mode = IdentityManagementLegacyDirect
	resolver, err := NewRoutingEnvThunderResolver(IdentityManagementMixed, targetResolverStub{target: target}, legacy, cp)
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
	require.NoError(t, err)
	require.Equal(t, 1, legacy.calls)
	require.Zero(t, cp.calls)
}

func TestRoutingEnvThunderResolverRejectsAmbiguousTargets(t *testing.T) {
	for name, mutate := range map[string]func(*EnvironmentIdentityTarget){
		"organization": func(target *EnvironmentIdentityTarget) { target.OUID = "other-org" },
		"namespace":    func(target *EnvironmentIdentityTarget) { target.OrgNamespace = "other-namespace" },
		"environment":  func(target *EnvironmentIdentityTarget) { target.Environment = "development" },
		"deployment":   func(target *EnvironmentIdentityTarget) { target.DeploymentID = "" },
		"gateway":      func(target *EnvironmentIdentityTarget) { target.GatewayID = "" },
		"runtime":      func(target *EnvironmentIdentityTarget) { target.TokenURL = "" },
		"url-userinfo": func(target *EnvironmentIdentityTarget) { target.ManagementURL = "https://name:secret@cp.example.test" },
		"url-fragment": func(target *EnvironmentIdentityTarget) { target.JWKSURL += "#fragment" },
		"url-scheme":   func(target *EnvironmentIdentityTarget) { target.IssuerURL = "file:///tmp/thunder" },
	} {
		t.Run(name, func(t *testing.T) {
			legacy := &legacyBackendStub{}
			cp := &cpBackendStub{}
			target := cpTarget()
			mutate(&target)
			resolver, err := NewRoutingEnvThunderResolver(IdentityManagementCPManaged, targetResolverStub{target: target}, legacy, cp)
			require.NoError(t, err)
			_, err = resolver.Resolve(context.Background(), "ou-1", "org", "staging")
			require.Error(t, err)
			require.Zero(t, legacy.calls)
			require.Zero(t, cp.calls)
		})
	}
}

func TestRoutingEnvThunderResolverValidatesStartupConfiguration(t *testing.T) {
	legacy := &legacyBackendStub{}
	_, err := NewRoutingEnvThunderResolver(IdentityManagementCPManaged, nil, legacy, nil)
	require.Error(t, err)
	_, err = NewRoutingEnvThunderResolver("unknown", nil, legacy, nil)
	require.Error(t, err)
	_, err = NewRoutingEnvThunderResolver(IdentityManagementLegacyDirect, nil, nil, nil)
	require.Error(t, err)
}
