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
package secretmanagersvc

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingSecrets is a SecretsClient that records what the management client passes down.
type recordingSecrets struct {
	err         error
	gotLocation SecretLocation
	gotData     map[string]string
	gotDeleted  []string
	gotMetadata *SecretMetadata
	gotOrg      string
	gotRefName  string
	info        *SecretInfo
}

func (r *recordingSecrets) PushSecret(_ context.Context, loc SecretLocation, data map[string]string, md *SecretMetadata) (string, error) {
	r.gotLocation, r.gotData, r.gotMetadata = loc, data, md
	return loc.SecretRefName(), r.err
}

func (r *recordingSecrets) PatchSecret(_ context.Context, loc SecretLocation, data map[string]string, del []string, md *SecretMetadata) (string, error) {
	r.gotLocation, r.gotData, r.gotDeleted, r.gotMetadata = loc, data, del, md
	return loc.SecretRefName(), r.err
}

func (r *recordingSecrets) DeleteSecret(_ context.Context, loc SecretLocation, md *SecretMetadata) error {
	r.gotLocation, r.gotMetadata = loc, md
	return r.err
}

func (r *recordingSecrets) DeleteSecretByName(_ context.Context, org, name string, md *SecretMetadata) error {
	r.gotOrg, r.gotRefName, r.gotMetadata = org, name, md
	return r.err
}

func (r *recordingSecrets) GetSecret(_ context.Context, loc SecretLocation) (*SecretInfo, error) {
	r.gotLocation = loc
	if r.err != nil {
		return nil, r.err
	}
	return r.info, nil
}

func (r *recordingSecrets) Close(context.Context) error { return nil }

type stubProvider struct {
	client SecretsClient
	err    error
}

func (p stubProvider) NewClient(*StoreConfig) (SecretsClient, error) { return p.client, p.err }
func (p stubProvider) ValidateConfig(*StoreConfig) error             { return nil }
func (p stubProvider) Capabilities() StoreCapabilities               { return StoreCapabilityReadWrite }

func newMgmtClient(t *testing.T, low SecretsClient) SecretManagementClient {
	t.Helper()
	c, err := NewSecretManagementClient(&StoreConfig{Provider: "stub"}, stubProvider{client: low})
	require.NoError(t, err)
	return c
}

func TestNewSecretManagementClientValidates(t *testing.T) {
	_, err := NewSecretManagementClient(nil, stubProvider{})
	require.ErrorContains(t, err, "config is required")
	_, err = NewSecretManagementClient(&StoreConfig{}, nil)
	require.ErrorContains(t, err, "provider is required")

	errInit := errors.New("store unreachable")
	_, err = NewSecretManagementClient(&StoreConfig{}, stubProvider{err: errInit})
	require.ErrorIs(t, err, errInit)
	require.ErrorContains(t, err, "failed to create secrets client")
}

func TestManagementClientStampsOwnershipAndWrapsErrors(t *testing.T) {
	ctx := context.Background()
	loc := SecretLocation{OrgName: "org", ProjectName: "p", EnvironmentName: "dev", AgentName: "a", EntityName: "llm"}

	low := &recordingSecrets{}
	c := newMgmtClient(t, low)

	ref, err := c.CreateSecret(ctx, loc, map[string]string{"api-key": "v"})
	require.NoError(t, err)
	assert.Equal(t, "llm-dev-secrets", ref)
	assert.Equal(t, loc, low.gotLocation)
	assert.Equal(t, DefaultManagedBy, low.gotMetadata.ManagedBy)

	_, err = c.PatchSecret(ctx, loc, map[string]string{"k": "v"}, []string{"old"})
	require.NoError(t, err)
	assert.Equal(t, []string{"old"}, low.gotDeleted)
	assert.Equal(t, DefaultManagedBy, low.gotMetadata.ManagedBy)

	require.NoError(t, c.DeleteSecret(ctx, loc, "ignored"))
	assert.Equal(t, DefaultManagedBy, low.gotMetadata.ManagedBy)

	require.NoError(t, c.DeleteSecretByName(ctx, "org", "llm-dev-secrets"))
	assert.Equal(t, "org", low.gotOrg)
	assert.Equal(t, "llm-dev-secrets", low.gotRefName)

	low.err = ErrSecretNotFound
	_, err = c.CreateSecret(ctx, loc, nil)
	require.ErrorIs(t, err, ErrSecretNotFound)
	require.ErrorContains(t, err, "failed to upsert secret")
	_, err = c.PatchSecret(ctx, loc, nil, nil)
	require.ErrorIs(t, err, ErrSecretNotFound)
	require.ErrorContains(t, err, "failed to patch secret")
	err = c.DeleteSecret(ctx, loc, "")
	require.ErrorIs(t, err, ErrSecretNotFound)
	err = c.DeleteSecretByName(ctx, "org", "ref")
	require.ErrorIs(t, err, ErrSecretNotFound)
	require.ErrorContains(t, err, `"ref"`)
}

func TestManagementClientGetSecretParsesThePath(t *testing.T) {
	ctx := context.Background()
	low := &recordingSecrets{info: &SecretInfo{ID: "id-1"}}
	c := newMgmtClient(t, low)

	info, err := c.GetSecret(ctx, "org/p/dev/agent/cfg/llm/api-key")
	require.NoError(t, err)
	assert.Equal(t, "id-1", info.ID)
	assert.Equal(t, SecretLocation{
		OrgName: "org", ProjectName: "p", EnvironmentName: "dev", AgentName: "agent",
		ConfigName: "cfg", EntityName: "llm", SecretKey: "api-key",
	}, low.gotLocation)

	_, err = c.GetSecret(ctx, "just-one-segment")
	require.ErrorContains(t, err, "unrecognized KV path format")

	low.err = ErrSecretNotFound
	_, err = c.GetSecret(ctx, "org/llm")
	require.ErrorIs(t, err, ErrSecretNotFound, "not-found stays detectable through the wrap")
}

// Every shape KVPath produces must parse back to the same location.
func TestParseKVPathRoundTrip(t *testing.T) {
	for _, loc := range []SecretLocation{
		{OrgName: "o", EntityName: "e"},
		{OrgName: "o", EntityName: "e", SecretKey: "k"},
		{OrgName: "o", ProjectName: "p", EnvironmentName: "env", EntityName: "e"},
		{OrgName: "o", ProjectName: "p", EnvironmentName: "env", AgentName: "a", EntityName: "e"},
		{OrgName: "o", ProjectName: "p", EnvironmentName: "env", AgentName: "a", ConfigName: "c", EntityName: "e"},
		{OrgName: "o", ProjectName: "p", EnvironmentName: "env", AgentName: "a", ConfigName: "c", EntityName: "e", SecretKey: "k"},
	} {
		path, err := loc.KVPath()
		require.NoError(t, err)
		got, err := ParseKVPath(path)
		require.NoError(t, err, path)
		assert.Equal(t, loc, got, path)
	}
	_, err := ParseKVPath(strings.Repeat("x/", 7) + "x")
	require.ErrorContains(t, err, "got 8")
}

func TestKVPathValidation(t *testing.T) {
	_, err := SecretLocation{EntityName: "e"}.KVPath()
	require.ErrorContains(t, err, "OrgName is required")
	_, err = SecretLocation{OrgName: "o", EntityName: "  "}.KVPath()
	require.ErrorContains(t, err, "is required")

	for field, loc := range map[string]SecretLocation{
		"ProjectName":     {OrgName: "o", EntityName: "e", ProjectName: ".."},
		"EnvironmentName": {OrgName: "o", EntityName: "e", EnvironmentName: "a/b"},
		"AgentName":       {OrgName: "o", EntityName: "e", AgentName: "."},
		"Config name":     {OrgName: "o", EntityName: "e", ConfigName: "x/y"},
		"Entity name":     {OrgName: "o", EntityName: ".."},
		"SecretKey":       {OrgName: "o", EntityName: "e", SecretKey: "../k"},
		"OrgName":         {OrgName: "a/b", EntityName: "e"},
	} {
		_, err := loc.KVPath()
		require.ErrorContains(t, err, "invalid "+field, field)
	}

	// Whitespace-only optional segments are dropped rather than producing "o//e".
	path, err := SecretLocation{OrgName: " o ", ProjectName: " ", EntityName: "e"}.KVPath()
	require.NoError(t, err)
	assert.Equal(t, "o/e", path)

	seg, err := SanitizeSegment("  name ")
	require.NoError(t, err)
	assert.Equal(t, "name", seg)
}

func TestSecretRefName(t *testing.T) {
	assert.Equal(t, "llm-secrets", SecretLocation{EntityName: "LLM"}.SecretRefName())
	assert.Equal(t, "my-proxy-staging-secrets", SecretLocation{EntityName: "my_proxy", EnvironmentName: "Staging"}.SecretRefName())
	assert.Equal(t, "cfg-dev-llm-secrets", SecretLocation{EntityName: "llm", EnvironmentName: "dev", ConfigName: "cfg"}.SecretRefName())

	long := SecretLocation{EntityName: strings.Repeat("a", 60) + "-b", EnvironmentName: "dev"}.SecretRefName()
	assert.LessOrEqual(t, len(long), 63, "fits a Kubernetes name")
	assert.False(t, strings.HasSuffix(long, "-"), "truncation leaves no trailing dash")
}

func TestRegistry(t *testing.T) {
	name := "registry-test-" + t.Name()
	_, ok := GetProvider(name)
	assert.False(t, ok)

	p := stubProvider{}
	Register(name, p)
	got, ok := GetProvider(name)
	require.True(t, ok)
	assert.Equal(t, p, got)
	assert.True(t, slices.Contains(GetProviders(), name))

	assert.PanicsWithValue(t, "provider already registered: "+name, func() { Register(name, p) })
}
