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

package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/auth"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/cloud"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

type loginHarness struct {
	opts   *LoginOptions
	cfg    *config.Config
	got    *auth.LoginOptions
	stderr func() string
}

func newLoginHarness(t *testing.T, instances map[string]config.Instance, current string) *loginHarness {
	t.Helper()
	ios, _, _, errOut := iostreams.Test()
	cfg := &config.Config{
		Path:            filepath.Join(t.TempDir(), "config.yaml"),
		CurrentInstance: current,
		Instances:       instances,
	}
	h := &loginHarness{cfg: cfg, stderr: errOut.String}
	h.opts = &LoginOptions{
		IO:     ios,
		Config: func() (*config.Config, error) { return cfg, nil },
		Authenticate: func(_ context.Context, o auth.LoginOptions) (*config.Instance, error) {
			h.got = &o
			return &config.Instance{URL: o.URL, Auth: config.AuthConfig{ClientID: o.ClientID, ClientSecret: o.ClientSecret}}, nil
		},
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return nil, errors.New("offline")
		},
		Getenv: func(string) string { return "" },
	}
	return h
}

const devGateway = "https://development-wso2cloud.gateway.dev.cloud.wso2.com"

func TestLogin_LogsInToCloud(t *testing.T) {
	work := map[string]config.Instance{
		"work": {URL: "https://amp.example.com", AuthServer: "https://idp.example.com"},
	}
	tests := []struct {
		name      string
		instances map[string]config.Instance
		current   string
		cloudFlag bool
		gateway   string
		wantURL   string
	}{
		{name: "first use, production by default", wantURL: cloud.DefaultGatewayURL + cloud.APIPath},
		{name: "first use, gateway from env", gateway: devGateway + "/", wantURL: devGateway + cloud.APIPath},
		{name: "current context missing from config", instances: work, current: "gone", wantURL: cloud.DefaultGatewayURL + cloud.APIPath},
		{name: "--cloud from a self-hosted context", instances: work, current: "work", cloudFlag: true, wantURL: cloud.DefaultGatewayURL + cloud.APIPath},
		{name: "--cloud with gateway from env", instances: work, current: "work", cloudFlag: true, gateway: devGateway, wantURL: devGateway + cloud.APIPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, tt.instances, tt.current)
			h.opts.Cloud = tt.cloudFlag
			h.opts.Getenv = func(k string) string {
				if k == cloud.GatewayURLEnv {
					return tt.gateway
				}
				return ""
			}

			if err := runLogin(context.Background(), h.opts); err != nil {
				t.Fatalf("runLogin: %v\n%s", err, h.stderr())
			}

			if h.got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", h.got.URL, tt.wantURL)
			}
			if h.got.Resource != cloud.Resource {
				t.Errorf("Resource = %q, want %q", h.got.Resource, cloud.Resource)
			}
			if h.got.AuthServer != "" {
				t.Errorf("AuthServer = %q, want none (the work instance's must not leak)", h.got.AuthServer)
			}
			if h.cfg.CurrentInstance != cloud.InstanceName {
				t.Errorf("current instance = %q, want %q", h.cfg.CurrentInstance, cloud.InstanceName)
			}
			if h.cfg.Instances[cloud.InstanceName].URL != tt.wantURL {
				t.Errorf("cloud instance URL = %q, want %q", h.cfg.Instances[cloud.InstanceName].URL, tt.wantURL)
			}
			if tt.instances != nil && h.cfg.Instances["work"].URL != "https://amp.example.com" {
				t.Errorf("work instance was modified")
			}
			if strings.Contains(h.stderr(), "amctl login --cloud") {
				t.Errorf("stderr = %q, want no --cloud tip on a cloud login", h.stderr())
			}
		})
	}
}

func TestLogin_WithoutFlagsReusesCurrentInstance(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"default": {URL: "https://other.example.com"},
		"work": {URL: "https://amp.example.com", AuthServer: "https://idp.example.com", Auth: config.AuthConfig{
			GrantType: "client_credentials", ClientID: "bot", ClientSecret: "s3cret",
		}},
	}, "work")

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.URL != "https://amp.example.com" {
		t.Errorf("URL = %q, want the current instance's URL", h.got.URL)
	}
	if h.got.AuthServer != "https://idp.example.com" {
		t.Errorf("AuthServer = %q, want the stored auth server", h.got.AuthServer)
	}
	if h.got.Resource != "" {
		t.Errorf("Resource = %q, want none off cloud", h.got.Resource)
	}
	if h.got.ClientID != "bot" || h.got.ClientSecret != "s3cret" {
		t.Errorf("client = %q/%q, want stored client credentials", h.got.ClientID, h.got.ClientSecret)
	}
	if h.cfg.CurrentInstance != "work" {
		t.Errorf("current instance = %q, want work", h.cfg.CurrentInstance)
	}
	if _, ok := h.cfg.Instances[cloud.InstanceName]; ok {
		t.Errorf("a cloud instance was created")
	}
	if !strings.Contains(h.stderr(), "run 'amctl login --cloud'") {
		t.Errorf("stderr = %q, want a tip about --cloud", h.stderr())
	}
}

func TestLogin_CloudReloginReusesStoredClientOnSameGateway(t *testing.T) {
	cloudURL := cloud.DefaultGatewayURL + cloud.APIPath
	tests := []struct {
		name       string
		storedURL  string
		flagName   string
		cloudFlag  bool
		wantClient string
		wantSecret string
	}{
		{name: "same gateway", storedURL: cloudURL, wantClient: "bot", wantSecret: "s3cret"},
		{name: "same gateway via --cloud", storedURL: cloudURL, cloudFlag: true, wantClient: "bot", wantSecret: "s3cret"},
		{name: "same gateway via --name cloud", storedURL: cloudURL, flagName: cloud.InstanceName, wantClient: "bot", wantSecret: "s3cret"},
		{name: "gateway changed", storedURL: devGateway + cloud.APIPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, map[string]config.Instance{
				cloud.InstanceName: {URL: tt.storedURL, Auth: config.AuthConfig{
					GrantType: "client_credentials", ClientID: "bot", ClientSecret: "s3cret",
				}},
			}, cloud.InstanceName)
			h.opts.Name = tt.flagName
			h.opts.Cloud = tt.cloudFlag

			if err := runLogin(context.Background(), h.opts); err != nil {
				t.Fatalf("runLogin: %v\n%s", err, h.stderr())
			}

			if h.got.URL != cloudURL {
				t.Errorf("URL = %q, want %q", h.got.URL, cloudURL)
			}
			if h.got.ClientID != tt.wantClient || h.got.ClientSecret != tt.wantSecret {
				t.Errorf("client = %q/%q, want %q/%q", h.got.ClientID, h.got.ClientSecret, tt.wantClient, tt.wantSecret)
			}
		})
	}
}

func TestLogin_WithNameReusesStoredInstance(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"default": {URL: "https://other.example.com"},
		"work": {URL: "https://amp.example.com", AuthServer: "https://idp.example.com", Auth: config.AuthConfig{
			Resource: "urn:example",
		}},
	}, "default")
	h.opts.Name = "work"

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.URL != "https://amp.example.com" {
		t.Errorf("URL = %q, want the stored instance's URL", h.got.URL)
	}
	if h.got.AuthServer != "https://idp.example.com" {
		t.Errorf("AuthServer = %q, want the stored auth server", h.got.AuthServer)
	}
	if h.got.Resource != "urn:example" {
		t.Errorf("Resource = %q, want the stored resource", h.got.Resource)
	}
	if h.cfg.Instances["work"].AuthServer != "https://idp.example.com" {
		t.Errorf("stored AuthServer lost on re-login")
	}
	if h.cfg.CurrentInstance != "work" {
		t.Errorf("current instance = %q, want work", h.cfg.CurrentInstance)
	}
	if h.cfg.Instances["default"].URL != "https://other.example.com" {
		t.Errorf("default instance was modified")
	}
}

func TestLogin_WithNameReusesStoredClient(t *testing.T) {
	tests := []struct {
		name       string
		auth       config.AuthConfig
		wantClient string
		wantSecret string
	}{
		{
			name:       "client credentials",
			auth:       config.AuthConfig{GrantType: "client_credentials", ClientID: "bot", ClientSecret: "s3cret"},
			wantClient: "bot", wantSecret: "s3cret",
		},
		{
			name:       "custom client id",
			auth:       config.AuthConfig{GrantType: "authorization_code", ClientID: "custom-cli"},
			wantClient: "custom-cli",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, map[string]config.Instance{
				"ci": {URL: "https://amp.example.com", Auth: tt.auth},
			}, "ci")
			h.opts.Name = "ci"

			if err := runLogin(context.Background(), h.opts); err != nil {
				t.Fatalf("runLogin: %v\n%s", err, h.stderr())
			}

			if h.got.ClientID != tt.wantClient || h.got.ClientSecret != tt.wantSecret {
				t.Errorf("client = %q/%q, want %q/%q", h.got.ClientID, h.got.ClientSecret, tt.wantClient, tt.wantSecret)
			}
		})
	}
}

func TestLogin_WithURLSavesUnderName(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"work": {URL: "https://amp.example.com"},
	}, "work")
	h.opts.URL = "https://new.example.com"
	h.opts.Name = "staging"

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.Resource != "" {
		t.Errorf("Resource = %q, want none off cloud", h.got.Resource)
	}
	if h.cfg.Instances["staging"].URL != "https://new.example.com" {
		t.Errorf("staging instance URL = %q", h.cfg.Instances["staging"].URL)
	}
	if h.cfg.CurrentInstance != "staging" {
		t.Errorf("current instance = %q, want staging", h.cfg.CurrentInstance)
	}
	if h.cfg.Instances["work"].URL != "https://amp.example.com" {
		t.Errorf("work instance was overwritten")
	}
}

func TestLogin_RejectsBadFlagCombinations(t *testing.T) {
	tests := []struct {
		name      string
		instances map[string]config.Instance
		url       string
		flagName  string
		cloudFlag bool
		wantErr   string
	}{
		{
			name:      "--cloud with --url",
			url:       "https://amp.example.com",
			flagName:  "work",
			cloudFlag: true,
			wantErr:   "--url cannot be used with --cloud",
		},
		{
			name:      "--cloud with another --name",
			flagName:  "work",
			cloudFlag: true,
			wantErr:   "--name cannot be used with --cloud",
		},
		{
			name:    "url without name",
			url:     "https://amp.example.com",
			wantErr: "--name is required with --url",
		},
		{
			name:     "url with the reserved cloud name",
			url:      "https://amp.example.com",
			flagName: cloud.InstanceName,
			wantErr:  "context name 'cloud' is reserved for WSO2 Cloud",
		},
		{
			name:     "cloud url passed to --url",
			url:      devGateway + cloud.APIPath,
			flagName: "dev",
			wantErr:  "is a WSO2 Cloud URL; log in with --cloud instead: export AMCTL_CLOUD_URL=" + devGateway + " && amctl login --cloud",
		},
		{
			name:      "unknown named instance",
			instances: map[string]config.Instance{"work": {URL: "https://amp.example.com"}},
			flagName:  "staging",
			wantErr:   "--url is required to log in to new instance 'staging'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, tt.instances, "")
			h.opts.URL = tt.url
			h.opts.Name = tt.flagName
			h.opts.Cloud = tt.cloudFlag

			if err := runLogin(context.Background(), h.opts); err == nil {
				t.Fatal("runLogin succeeded, want an error")
			}
			if h.got != nil {
				t.Errorf("Authenticate was called")
			}
			if !strings.Contains(h.stderr(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", h.stderr(), tt.wantErr)
			}
		})
	}
}

func TestLogin_ReloginReplacesRetiredCloudResource(t *testing.T) {
	// A context saved by an older amctl's --cloud, under another name, with the retired resource server.
	h := newLoginHarness(t, map[string]config.Instance{
		"default": {URL: cloud.DefaultGatewayURL + cloud.APIPath, Auth: config.AuthConfig{Resource: "urn:wso2:amp"}},
	}, "default")

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.Resource != cloud.Resource {
		t.Errorf("Resource = %q, want %q", h.got.Resource, cloud.Resource)
	}
	if strings.Contains(h.stderr(), "amctl login --cloud") {
		t.Errorf("stderr = %q, want no --cloud tip for a context already on cloud", h.stderr())
	}
}

type fakeSelector struct {
	choice  int
	err     error
	calls   int
	options []string
}

func (f *fakeSelector) Select(_ string, options []string, _ int) (int, error) {
	f.calls++
	f.options = options
	return f.choice, f.err
}

func TestLogin_CloudRegion(t *testing.T) {
	usURL := cloud.DefaultGatewayURL + cloud.APIPath
	euURL := cloud.EUGatewayURL + cloud.APIPath
	tests := []struct {
		name       string
		instances  map[string]config.Instance
		current    string
		region     string
		cloudFlag  bool
		gateway    string
		terminal   bool
		json       bool
		choice     int
		wantURL    string
		wantPrompt bool
	}{
		{name: "first login asks on a terminal", terminal: true, choice: 1, wantURL: euURL, wantPrompt: true},
		{name: "first login without a terminal is the US", wantURL: usURL},
		{name: "--json does not ask", terminal: true, json: true, choice: 1, wantURL: usURL},
		{name: "--region picks without asking", terminal: true, region: "eu", wantURL: euURL},
		{
			name:      "--region from a self-hosted context",
			instances: map[string]config.Instance{"work": {URL: "https://amp.example.com"}},
			current:   "work", terminal: true, region: "EU", wantURL: euURL,
		},
		{
			name:      "re-login keeps the stored region",
			instances: map[string]config.Instance{cloud.InstanceName: {URL: euURL}},
			current:   cloud.InstanceName, terminal: true, wantURL: euURL,
		},
		{
			name:      "--cloud keeps the stored region",
			instances: map[string]config.Instance{cloud.InstanceName: {URL: euURL}, "work": {URL: "https://amp.example.com"}},
			current:   "work", cloudFlag: true, terminal: true, wantURL: euURL,
		},
		{
			name:      "--region changes the stored region",
			instances: map[string]config.Instance{cloud.InstanceName: {URL: euURL}},
			current:   cloud.InstanceName, region: "us", wantURL: usURL,
		},
		{
			name:      "stored retired gateway asks again",
			instances: map[string]config.Instance{cloud.InstanceName: {URL: "https://production-wso2cloud.gateway.cloud.wso2.com" + cloud.APIPath}},
			current:   cloud.InstanceName, terminal: true, choice: 0, wantURL: usURL, wantPrompt: true,
		},
		{
			name:      "AMCTL_CLOUD_URL beats the stored region",
			instances: map[string]config.Instance{cloud.InstanceName: {URL: euURL}},
			current:   cloud.InstanceName, terminal: true, gateway: devGateway, wantURL: devGateway + cloud.APIPath,
		},
		{name: "--region beats AMCTL_CLOUD_URL", region: "eu", gateway: devGateway, wantURL: euURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, tt.instances, tt.current)
			h.opts.IO.SetTerminal(tt.terminal, tt.terminal, tt.terminal)
			h.opts.IO.JSON = tt.json
			sel := &fakeSelector{choice: tt.choice}
			h.opts.Selector = sel
			h.opts.Region = tt.region
			h.opts.Cloud = tt.cloudFlag
			h.opts.Getenv = func(k string) string {
				if k == cloud.GatewayURLEnv {
					return tt.gateway
				}
				return ""
			}

			if err := runLogin(context.Background(), h.opts); err != nil {
				t.Fatalf("runLogin: %v\n%s", err, h.stderr())
			}

			if h.got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", h.got.URL, tt.wantURL)
			}
			if h.cfg.Instances[cloud.InstanceName].URL != tt.wantURL {
				t.Errorf("cloud context URL = %q, want %q", h.cfg.Instances[cloud.InstanceName].URL, tt.wantURL)
			}
			if h.cfg.CurrentInstance != cloud.InstanceName {
				t.Errorf("current instance = %q, want %q", h.cfg.CurrentInstance, cloud.InstanceName)
			}
			if (sel.calls == 1) != tt.wantPrompt || sel.calls > 1 {
				t.Errorf("Select called %d times, want prompt=%v", sel.calls, tt.wantPrompt)
			}
			if tt.wantPrompt && len(sel.options) != len(cloud.Regions) {
				t.Errorf("Select options = %v, want one per region", sel.options)
			}
		})
	}
}

func TestLogin_CloudRegionErrors(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		flagName  string
		region    string
		selectErr error
		wantErr   string
	}{
		{name: "unknown region", region: "apac", wantErr: `unknown --region "apac": use one of us, eu`},
		{name: "--region with --url", region: "eu", url: "https://amp.example.com", flagName: "work", wantErr: "--url cannot be used with --cloud or --region"},
		{name: "--region with another --name", region: "eu", flagName: "work", wantErr: "--name cannot be used with --cloud or --region"},
		{name: "no usable answer", selectErr: errors.New("no valid choice"), wantErr: "choose a region: no valid choice; or pass --region us, eu"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, nil, "")
			h.opts.IO.SetTerminal(true, true, true)
			h.opts.Selector = &fakeSelector{err: tt.selectErr}
			h.opts.URL = tt.url
			h.opts.Name = tt.flagName
			h.opts.Region = tt.region

			if err := runLogin(context.Background(), h.opts); err == nil {
				t.Fatal("runLogin succeeded, want an error")
			}
			if h.got != nil {
				t.Errorf("Authenticate was called")
			}
			if !strings.Contains(h.stderr(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", h.stderr(), tt.wantErr)
			}
		})
	}
}
