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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestIsA2AAgentSubType(t *testing.T) {
	assert.True(t, IsA2AAgentSubType("a2a-agent"))
	assert.False(t, IsA2AAgentSubType("chat-api"))
	assert.False(t, IsA2AAgentSubType("custom-api"))
	assert.False(t, IsA2AAgentSubType(""))
}

func TestValidateAgentTypeAcceptsA2A(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	err := validateAgentType(spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType})
	assert.NoError(t, err)
}

// An a2a-agent serves its own agent card and has no OpenAPI document, so the
// custom-api schema requirement must not reach it.
func TestValidateInputInterfaceA2ANeedsNoSchema(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	port := int32(9099)
	err := validateInputInterface(
		spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType},
		&spec.InputInterface{Type: "HTTP", Port: &port},
	)
	assert.NoError(t, err)
}

func TestValidateInputInterfaceA2APort(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	agentType := spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType}
	portOf := func(p int32) *int32 { return &p }

	tests := []struct {
		name    string
		port    *int32
		wantErr bool
	}{
		{name: "missing port", port: nil, wantErr: true},
		{name: "zero port", port: portOf(0), wantErr: true},
		{name: "port above range", port: portOf(70000), wantErr: true},
		{name: "lowest valid port", port: portOf(1), wantErr: false},
		{name: "highest valid port", port: portOf(65535), wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputInterface(agentType, &spec.InputInterface{Type: "HTTP", Port: tt.port})
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.NotNil(t, IsValidationError(err))
		})
	}
}

// A chat-api port is optional, but one that is set must be a valid port: it
// becomes the gateway's upstream port.
func TestValidateInputInterfaceChatAPIPort(t *testing.T) {
	subType := string(AgentSubTypeChatAPI)
	agentType := spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType}
	portOf := func(p int32) *int32 { return &p }

	tests := []struct {
		name    string
		port    *int32
		wantErr bool
	}{
		{name: "omitted port uses the default", port: nil, wantErr: false},
		{name: "valid port", port: portOf(9090), wantErr: false},
		{name: "zero port", port: portOf(0), wantErr: true},
		{name: "port above range", port: portOf(65536), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputInterface(agentType, &spec.InputInterface{Type: "HTTP", Port: tt.port})
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.NotNil(t, IsValidationError(err))
		})
	}
}

func TestValidateInputInterfaceCustomAPI(t *testing.T) {
	subType := string(AgentSubTypeCustomAPI)
	agentType := spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType}
	portOf := func(p int32) *int32 { return &p }
	schema := &spec.InputInterfaceSchema{Path: StrAsStrPointer("/openapi.yaml")}

	tests := []struct {
		name    string
		iface   spec.InputInterface
		wantErr string
	}{
		{name: "valid", iface: spec.InputInterface{Type: "HTTP", Port: portOf(8080), BasePath: StrAsStrPointer("/api"), Schema: schema}},
		{name: "missing schema", iface: spec.InputInterface{Type: "HTTP", Port: portOf(8080), BasePath: StrAsStrPointer("/api")}, wantErr: "inputInterface.schema.path"},
		{name: "missing port", iface: spec.InputInterface{Type: "HTTP", BasePath: StrAsStrPointer("/api"), Schema: schema}, wantErr: "inputInterface.port"},
		{name: "port above range", iface: spec.InputInterface{Type: "HTTP", Port: portOf(70000), BasePath: StrAsStrPointer("/api"), Schema: schema}, wantErr: "inputInterface.port"},
		{name: "missing base path", iface: spec.InputInterface{Type: "HTTP", Port: portOf(8080), Schema: schema}, wantErr: "inputInterface.basePath is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iface := tt.iface
			err := validateInputInterface(agentType, &iface)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// Build-parameter updates reuse the create validation, so a portless A2A update is rejected.
func TestValidateAgentBuildParametersUpdateRejectsA2AWithoutPort(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	payload := spec.UpdateAgentBuildParametersRequest{
		Provisioning: spec.Provisioning{
			Type: string(InternalAgent),
			Repository: &spec.RepositoryConfig{
				Url:     "https://github.com/wso2/agent-manager",
				Branch:  "main",
				AppPath: "/",
			},
		},
		AgentType:      spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType},
		InputInterface: spec.InputInterface{Type: "HTTP"},
	}
	err := ValidateAgentBuildParametersUpdatePayload(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inputInterface.port")
}
