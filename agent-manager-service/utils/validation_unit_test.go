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

package utils

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestValidateResourceName(t *testing.T) {
	valid := []string{"a", "agent-1", "x9", strings.Repeat("a", MaxResourceNameLength)}
	for _, name := range valid {
		assert.NoError(t, ValidateResourceName(name, "agent"), name)
	}

	for _, tc := range []struct{ name, msg string }{
		{"", "Agent name cannot be empty"},
		{strings.Repeat("a", MaxResourceNameLength+1), "Agent name must be at most 25 characters, got 26"},
		{"Agent", "Agent name must contain only lowercase alphanumeric characters or '-'"},
		{"my_agent", "Agent name must contain only lowercase alphanumeric characters or '-'"},
		{"1agent", "Agent name must start with an alphabetic character"},
		{"-agent", "Agent name must start with an alphabetic character"},
		{"agent-", "Agent name must end with an alphanumeric character"},
	} {
		err := ValidateResourceName(tc.name, "agent")
		require.Error(t, err, tc.name)
		assert.Equal(t, tc.msg, err.Error())
	}
	assert.EqualError(t, ValidateResourceName("", "project"), "Project name cannot be empty", "the message names the resource")
}

func TestValidateResourceDisplayName(t *testing.T) {
	assert.NoError(t, ValidateResourceDisplayName("Anything Goes!", "agent"))
	assert.EqualError(t, ValidateResourceDisplayName("", "project"), "Project display name cannot be empty")
	assert.EqualError(t, ValidateResourceDisplayName("", ""), " display name cannot be empty")
}

func TestValidateUpdatePayloads(t *testing.T) {
	assert.NoError(t, ValidateProjectUpdatePayload(spec.UpdateProjectRequest{DisplayName: "P"}))
	assert.Error(t, ValidateProjectUpdatePayload(spec.UpdateProjectRequest{}))

	assert.NoError(t, ValidateAgentBasicInfoUpdatePayload(spec.UpdateAgentBasicInfoRequest{DisplayName: "A"}))
	assert.EqualError(t, ValidateAgentBasicInfoUpdatePayload(spec.UpdateAgentBasicInfoRequest{}), "Agent display name cannot be empty")
	good := map[string]string{"team": "a"}
	assert.NoError(t, ValidateAgentBasicInfoUpdatePayload(spec.UpdateAgentBasicInfoRequest{DisplayName: "A", Labels: &good}))
	bad := map[string]string{"Bad Key": "a"}
	err := ValidateAgentBasicInfoUpdatePayload(spec.UpdateAgentBasicInfoRequest{DisplayName: "A", Labels: &bad})
	require.Error(t, err)
	assert.NotNil(t, httpx.IsValidationError(err), "a bad label is a validation error with its own message")
}

func TestValidateAgentCreatePayload(t *testing.T) {
	external := spec.AgentType{Type: string(AgentTypeExternalAPI)}
	base := func() spec.CreateAgentRequest {
		return spec.CreateAgentRequest{
			Name: "agent", DisplayName: "Agent", Provisioning: spec.Provisioning{Type: string(ExternalAgent)}, AgentType: &external,
		}
	}
	assert.NoError(t, ValidateAgentCreatePayload(base()))

	for name, mutate := range map[string]func(*spec.CreateAgentRequest){
		"bad name":         func(r *spec.CreateAgentRequest) { r.Name = "Agent" },
		"no display name":  func(r *spec.CreateAgentRequest) { r.DisplayName = "" },
		"internal":         func(r *spec.CreateAgentRequest) { r.Provisioning.Type = string(InternalAgent) },
		"no agent type":    func(r *spec.CreateAgentRequest) { r.AgentType = nil },
		"unknown type":     func(r *spec.CreateAgentRequest) { r.AgentType = &spec.AgentType{Type: "robot"} },
		"bad label":        func(r *spec.CreateAgentRequest) { r.Labels = &map[string]string{"": "x"} },
		"internal type":    func(r *spec.CreateAgentRequest) { r.AgentType = &spec.AgentType{Type: string(AgentTypeAPI)} },
		"has health check": func(r *spec.CreateAgentRequest) { r.HealthChecks = &spec.HealthChecks{} },
	} {
		req := base()
		mutate(&req)
		assert.Error(t, ValidateAgentCreatePayload(req), name)
	}
	req := base()
	req.Provisioning.Type = string(InternalAgent)
	err := ValidateAgentCreatePayload(req)
	assert.Equal(t, "Only external agents can be created", httpx.IsValidationError(err).Message)
}

func TestValidateResourceNameRequest(t *testing.T) {
	empty := ""
	proj := "apps"
	for _, ok := range []spec.ResourceNameRequest{
		{DisplayName: "A", ResourceType: "agent", ProjectName: &proj},
		{DisplayName: "A", ResourceType: "agent"},
		{DisplayName: "P", ResourceType: "project"},
		{DisplayName: "E", ResourceType: "environment"},
	} {
		assert.NoError(t, ValidateResourceNameRequest(ok), ok.ResourceType)
	}
	assert.EqualError(t, ValidateResourceNameRequest(spec.ResourceNameRequest{ResourceType: "agent"}),
		"invalid resource display name: Resource display name cannot be empty")
	assert.EqualError(t, ValidateResourceNameRequest(spec.ResourceNameRequest{DisplayName: "x", ResourceType: "planet"}), "invalid resource type")
	assert.EqualError(t, ValidateResourceNameRequest(spec.ResourceNameRequest{DisplayName: "x", ResourceType: "agent", ProjectName: &empty}),
		"projectName cannot be empty for agent resource type")
}

func TestGenerateCandidateName(t *testing.T) {
	for in, want := range map[string]string{
		"My Agent":                        "my-agent",
		"  Spaced   Out  ":                "spaced-out",
		"Héllo, Wörld!":                   "hllo-wrld",
		"--Already-Hyphenated--":          "already-hyphenated",
		"A very long display name indeed": "a-very-long-display-name",
		"!!!":                             "",
	} {
		got := GenerateCandidateName(in)
		assert.Equal(t, want, got, in)
		assert.LessOrEqual(t, len(got), MaxResourceNameLength)
	}
}

func TestGenerateUniqueNameWithSuffix(t *testing.T) {
	t.Run("first free candidate wins", func(t *testing.T) {
		var tried []string
		name, err := GenerateUniqueNameWithSuffix("agent", func(n string) (bool, error) {
			tried = append(tried, n)
			return len(tried) == 3, nil
		})
		require.NoError(t, err)
		assert.Len(t, tried, 3)
		assert.Equal(t, tried[2], name)
		assert.Regexp(t, `^agent-[a-z]{2}$`, name)
	})

	t.Run("a long base is cut so the result is still a valid name", func(t *testing.T) {
		name, err := GenerateUniqueNameWithSuffix(strings.Repeat("b", 40), func(string) (bool, error) { return true, nil })
		require.NoError(t, err)
		assert.Len(t, name, MaxResourceNameLength)
		assert.NoError(t, ValidateResourceName(name, "agent"))
	})

	t.Run("the checker's error is returned", func(t *testing.T) {
		boom := errors.New("db down")
		_, err := GenerateUniqueNameWithSuffix("agent", func(string) (bool, error) { return false, boom })
		assert.ErrorIs(t, err, boom)
	})

	t.Run("gives up after the attempt limit", func(t *testing.T) {
		calls := 0
		_, err := GenerateUniqueNameWithSuffix("agent", func(string) (bool, error) { calls++; return false, nil })
		assert.EqualError(t, err, "failed to generate unique name after 10 attempts")
		assert.Equal(t, MaxNameGenerationAttempts, calls)
	})
}

func TestMaxEnvNameLength(t *testing.T) {
	assert.Equal(t, 22-len("acme"), MaxEnvNameLength("acme"))
	assert.Equal(t, 1, MaxEnvNameLength(strings.Repeat("o", 30)), "never less than one")
	assert.Equal(t, 1, MaxEnvNameLength(strings.Repeat("o", 21)))
}

func TestValidateUserID(t *testing.T) {
	for _, ok := range []string{"alice", "a.b@example.com", "550e8400-e29b-41d4-a716-446655440000", "user_1"} {
		assert.NoError(t, ValidateUserID(ok), ok)
	}
	for id, msg := range map[string]string{
		"":                       "user ID cannot be empty",
		strings.Repeat("u", 256): "user ID length must not exceed 255 characters",
		"bob\r\nX-Injected: 1":   "user ID contains invalid control characters",
		"bob\x7f":                "user ID contains invalid control characters",
		"bob smith":              "user ID contains invalid characters",
		"bob/../admin":           "user ID contains invalid characters",
	} {
		assert.EqualError(t, ValidateUserID(id), msg, "%q", id)
	}
}

func TestValidateLabelValue(t *testing.T) {
	assert.NoError(t, ValidateLabelValue("v1.2_3-x", "kindVersion"))
	assert.NoError(t, ValidateLabelValue("", "kindVersion"), "an empty label value is allowed")
	for _, bad := range []string{"-lead", "trail.", "has space", strings.Repeat("a", 64)} {
		err := ValidateLabelValue(bad, "kindVersion")
		require.Error(t, err, bad)
		ve := httpx.IsValidationError(err)
		require.NotNil(t, ve)
		assert.True(t, strings.HasPrefix(ve.Message, "kindVersion must be"), "the message names the field")
	}
}

func TestAgentTypeForProvisioning(t *testing.T) {
	assert.Equal(t, AgentTypeAPI, AgentTypeForProvisioning(string(InternalAgent)))
	assert.Equal(t, AgentTypeExternalAPI, AgentTypeForProvisioning(string(ExternalAgent)))
	assert.Equal(t, AgentTypeExternalAPI, AgentTypeForProvisioning(""))
}
