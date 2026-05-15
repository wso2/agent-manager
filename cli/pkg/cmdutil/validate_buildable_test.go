// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0.
package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func newGetAgentServer(t *testing.T, agent *amsvc.AgentResponse, status int) *amsvc.ClientWithResponses {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK && agent != nil {
			_ = json.NewEncoder(w).Encode(agent)
		}
		if status == http.StatusNotFound {
			_ = json.NewEncoder(w).Encode(amsvc.ErrorResponse{Message: "agent not found"})
		}
	}))
	t.Cleanup(server.Close)
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func TestValidateBuildable_InternalPasses(t *testing.T) {
	agent := &amsvc.AgentResponse{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		ProjectName:  "proj",
		Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeInternal},
		CreatedAt:    time.Now().UTC(),
	}
	client := newGetAgentServer(t, agent, http.StatusOK)

	err := ValidateBuildable(context.Background(), client, "org", "proj", "my-agent")
	if err != nil {
		t.Errorf("ValidateBuildable = %v, want nil for internal agent", err)
	}
}

func TestValidateBuildable_ExternalFails(t *testing.T) {
	agent := &amsvc.AgentResponse{
		Name:         "ext-agent",
		DisplayName:  "Ext",
		ProjectName:  "proj",
		Provisioning: amsvc.Provisioning{Type: "external"},
		CreatedAt:    time.Now().UTC(),
	}
	client := newGetAgentServer(t, agent, http.StatusOK)

	err := ValidateBuildable(context.Background(), client, "org", "proj", "ext-agent")
	if err == nil {
		t.Fatal("ValidateBuildable = nil, want error for external agent")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error is not CLIError: %T", err)
	}
	if cliErr.Code != clierr.Validation {
		t.Errorf("code = %q, want %q", cliErr.Code, clierr.Validation)
	}
	if !strings.Contains(cliErr.Message, "externally provisioned") {
		t.Errorf("message = %q, want to contain 'externally provisioned'", cliErr.Message)
	}
	if !strings.Contains(cliErr.Message, "Builds") {
		t.Errorf("message = %q, want to contain 'Builds'", cliErr.Message)
	}
}

func TestValidateRuntimeManaged_InternalPasses(t *testing.T) {
	agent := &amsvc.AgentResponse{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		ProjectName:  "proj",
		Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeInternal},
		CreatedAt:    time.Now().UTC(),
	}
	client := newGetAgentServer(t, agent, http.StatusOK)

	err := ValidateRuntimeManaged(context.Background(), client, "org", "proj", "my-agent")
	if err != nil {
		t.Errorf("ValidateRuntimeManaged = %v, want nil for internal agent", err)
	}
}

func TestValidateRuntimeManaged_ExternalFails(t *testing.T) {
	agent := &amsvc.AgentResponse{
		Name:         "ext-agent",
		DisplayName:  "Ext",
		ProjectName:  "proj",
		Provisioning: amsvc.Provisioning{Type: "external"},
		CreatedAt:    time.Now().UTC(),
	}
	client := newGetAgentServer(t, agent, http.StatusOK)

	err := ValidateRuntimeManaged(context.Background(), client, "org", "proj", "ext-agent")
	if err == nil {
		t.Fatal("ValidateRuntimeManaged = nil, want error for external agent")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error is not CLIError: %T", err)
	}
	if cliErr.Code != clierr.Validation {
		t.Errorf("code = %q, want %q", cliErr.Code, clierr.Validation)
	}
	if !strings.Contains(cliErr.Message, "Runtime logs and metrics") {
		t.Errorf("message = %q, want to contain 'Runtime logs and metrics'", cliErr.Message)
	}
}

func TestValidateBuildable_NotFoundReturnsServerError(t *testing.T) {
	client := newGetAgentServer(t, nil, http.StatusNotFound)

	err := ValidateBuildable(context.Background(), client, "org", "proj", "missing")
	if err == nil {
		t.Fatal("ValidateBuildable = nil, want error for 404")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error is not CLIError: %T", err)
	}
	if cliErr.Code == clierr.Validation {
		t.Error("404 should not produce Validation code")
	}
}
