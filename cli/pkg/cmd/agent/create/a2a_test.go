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

package create

import (
	"testing"
)

// An a2a agent serves its own agent card and has no OpenAPI document, so the
// spec flag is not merely optional — passing it is a mistake worth naming.
func TestPrepare_A2ARejectsOpenAPISpec(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = subTypeA2A
	opts.PortSet = true
	opts.Port = 9099
	opts.OpenAPISpec = "/openapi.yaml"

	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--openapi-spec is not allowed for subtype a2a-agent")
}

// The port is where the agent's A2A server listens; the gateway's upstream is
// derived from the endpoint it declares, so there is no default to fall back on.
func TestValidateRequest_A2ARequiresPort(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr(subTypeA2A)
	req.InputInterface.Port = nil

	v := validateRequest(req)
	assertViolation(t, v, "spec.inputInterface.port is required for subtype a2a-agent")
}

// a2a-agent is a legal subtype, so it must not fall through to the unknown-subtype
// branch the way "grpc" does.
func TestValidateRequest_A2AIsAValidSubType(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr(subTypeA2A)
	req.InputInterface.Port = intPtr(9099)

	for _, violation := range validateRequest(req) {
		if violation == `spec.agentType.subType must be "chat-api", "custom-api" or "a2a-agent", got "a2a-agent"` {
			t.Fatal("a2a-agent must be accepted as a subtype")
		}
	}
}

// An a2a agent carries no OpenAPI document, so the builder must never attach a
// schema to its interface — the gateway reads the agent's own card instead.
func TestBuildInterface_A2ACarriesPortWithoutSchema(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = subTypeA2A
	opts.PortSet = true
	opts.Port = 9099

	req, err := prepare(opts)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if req.InputInterface == nil {
		t.Fatal("an a2a agent declares an interface")
	}
	if req.InputInterface.Port == nil || *req.InputInterface.Port != 9099 {
		t.Errorf("port = %v, want 9099", req.InputInterface.Port)
	}
	if req.InputInterface.Schema != nil {
		t.Error("an a2a agent carries no OpenAPI schema")
	}
}
