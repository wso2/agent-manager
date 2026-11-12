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

// Package ampinit writes the configuration files of the processes that run
// beside Agent Manager in a standalone install — ThunderID and the AI
// Gateway — from environment variables. Every run regenerates the files from
// the same inputs; key material and generated tokens are created once and
// kept.
package ampinit

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/gatewaytoken"
)

// Client is an OAuth client Thunder registers for Agent Manager.
type Client struct {
	ID           string
	Secret       string
	RedirectURIs []string
}

// User is a ThunderID user the bootstrap creates.
type User struct {
	Username   string
	Password   string
	Email      string
	GivenName  string
	FamilyName string
	// Admin adds the user to the Agent Manager admins group.
	Admin bool
}

// ThunderConfig is everything the ThunderID files depend on.
type ThunderConfig struct {
	// ThunderPublicURL is the URL browsers and tokens use (the issuer).
	ThunderPublicURL string
	ThunderScheme    string
	ThunderHost      string
	ThunderPort      int
	// ThunderListenPort is the port the server binds inside its container.
	ThunderListenPort int

	AMPPublicURL      string
	ObserverPublicURL string
	ConsoleURL        string

	ConsoleRedirectURIs []string
	CORSOrigins         []string

	APIClient       Client
	ConsoleClient   Client
	PublisherClient Client
	SystemClient    Client
	CLIClient       Client

	// Admin is Thunder's built-in admin account; its password is set by Thunder's setup.
	Admin User
	Users []User
}

// GatewayConfig is everything the AI Gateway files depend on.
type GatewayConfig struct {
	// ControlPlaneHost is host:port of Agent Manager's gateway WebSocket listener.
	ControlPlaneHost string
	// RegistrationToken is the gateway's control-plane token; generated and kept when empty.
	RegistrationToken string
	AdminPassword     string
	AMPJWKSURL        string
	ThunderIssuer     string
	ThunderJWKSURL    string
	// Host is the gateway's public host name (MCP auth metadata, listener certificate).
	Host         string
	ListenerPort int
}

type envReader struct{ errs []error }

func (r *envReader) str(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func (r *envReader) required(key string) string {
	v := r.str(key, "")
	if v == "" {
		r.errs = append(r.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (r *envReader) list(key string, def []string) []string {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (r *envReader) port(key string, def int) int {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 || n > 65535 {
		r.errs = append(r.errs, fmt.Errorf("%s: %q is not a port", key, v))
		return def
	}
	return n
}

func (r *envReader) url(key string) *url.URL {
	v := strings.TrimRight(r.required(key), "/")
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme == "" || u.Host == "" {
		r.errs = append(r.errs, fmt.Errorf("%s: %q is not an absolute URL", key, v))
		return nil
	}
	return u
}

func (r *envReader) err() error { return errors.Join(r.errs...) }

// defaultPort is the URL's port, or its scheme's.
func defaultPort(u *url.URL) int {
	if p, err := strconv.Atoi(u.Port()); err == nil {
		return p
	}
	if u.Scheme == "https" {
		return 443
	}
	return 80
}

// LoadThunderConfig reads the ThunderID inputs from the environment.
func LoadThunderConfig() (*ThunderConfig, error) {
	r := &envReader{}
	cfg := &ThunderConfig{
		AMPPublicURL:      strings.TrimRight(r.required("AMP_PUBLIC_URL"), "/"),
		ObserverPublicURL: strings.TrimRight(r.required("OBSERVER_PUBLIC_URL"), "/"),
		ConsoleURL:        strings.TrimRight(r.required("CONSOLE_URL"), "/"),
		APIClient: Client{
			ID: r.str("AMP_API_CLIENT_ID", "amp-api-client"), Secret: r.required("AMP_API_CLIENT_SECRET"),
		},
		ConsoleClient: Client{
			ID: r.str("AMP_CONSOLE_CLIENT_ID", "amp-console-client"), Secret: r.required("AMP_CONSOLE_CLIENT_SECRET"),
		},
		PublisherClient: Client{
			ID: r.str("AMP_PUBLISHER_CLIENT_ID", "amp-publisher-client"), Secret: r.required("AMP_PUBLISHER_CLIENT_SECRET"),
		},
		SystemClient: Client{
			ID: r.str("AMP_SYSTEM_CLIENT_ID", "amp-system-client"), Secret: r.required("AMP_SYSTEM_CLIENT_SECRET"),
		},
		CLIClient: Client{
			ID:           r.str("AMCTL_CLIENT_ID", "amctl"),
			RedirectURIs: r.list("AMCTL_REDIRECT_URIS", []string{"http://127.0.0.1:10325/callback"}),
		},
		Admin: User{
			Username:   r.str("THUNDER_ADMIN_USERNAME", "admin"),
			Email:      r.str("THUNDER_ADMIN_EMAIL", "admin@example.com"),
			GivenName:  r.str("THUNDER_ADMIN_GIVEN_NAME", "Admin"),
			FamilyName: r.str("THUNDER_ADMIN_FAMILY_NAME", "User"),
		},
	}
	if u := r.url("THUNDER_PUBLIC_URL"); u != nil {
		cfg.ThunderPublicURL = u.String()
		cfg.ThunderScheme = u.Scheme
		cfg.ThunderHost = u.Hostname()
		cfg.ThunderPort = defaultPort(u)
		cfg.ThunderListenPort = r.port("THUNDER_PORT", cfg.ThunderPort)
	}
	cfg.ConsoleRedirectURIs = r.list("CONSOLE_REDIRECT_URIS", []string{cfg.ConsoleURL + "/login"})
	cfg.CORSOrigins = r.list("THUNDER_CORS_ORIGINS", []string{cfg.ConsoleURL})
	users, err := parseUsers(r.str("AMP_DEFAULT_USERS", ""))
	if err != nil {
		r.errs = append(r.errs, err)
	}
	cfg.Users = users
	return cfg, r.err()
}

// parseUsers reads AMP_DEFAULT_USERS: comma-separated username:password[:admin]
// entries. Neither field may contain ',' or ':'.
func parseUsers(spec string) ([]User, error) {
	var users []User
	seen := map[string]bool{}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" ||
			(len(parts) == 3 && parts[2] != "admin") {
			return nil, fmt.Errorf("AMP_DEFAULT_USERS: entry %q is not username:password[:admin]", parts[0])
		}
		if seen[parts[0]] {
			return nil, fmt.Errorf("AMP_DEFAULT_USERS: user %q is listed twice", parts[0])
		}
		seen[parts[0]] = true
		users = append(users, User{
			Username: parts[0], Password: parts[1], Email: parts[0] + "@example.com",
			GivenName: parts[0], Admin: len(parts) == 3,
		})
	}
	return users, nil
}

// LoadGatewayConfig reads the AI Gateway inputs from the environment.
func LoadGatewayConfig() (*GatewayConfig, error) {
	r := &envReader{}
	cfg := &GatewayConfig{
		ControlPlaneHost:  r.required("GATEWAY_CONTROLPLANE_HOST"),
		RegistrationToken: r.str("DEFAULT_GATEWAY_TOKEN", ""),
		AdminPassword:     r.required("GATEWAY_ADMIN_PASSWORD"),
		AMPJWKSURL:        r.required("GATEWAY_AMP_JWKS_URL"),
		ThunderIssuer:     strings.TrimRight(r.required("THUNDER_PUBLIC_URL"), "/"),
		ThunderJWKSURL:    r.required("GATEWAY_THUNDER_JWKS_URL"),
		Host:              r.required("GATEWAY_HOST"),
		ListenerPort:      r.port("GATEWAY_LISTENER_PORT", 19080),
	}
	if cfg.RegistrationToken != "" {
		if _, err := gatewaytoken.Prefix(cfg.RegistrationToken); err != nil {
			r.errs = append(r.errs, fmt.Errorf("DEFAULT_GATEWAY_TOKEN: %w", err))
		}
	}
	return cfg, r.err()
}

// SampleThunderConfig is a complete configuration with the local-development
// values, for checks that read the rendered resources.
func SampleThunderConfig() *ThunderConfig {
	return &ThunderConfig{
		ThunderPublicURL: "http://thunder.amp.localhost:8080", ThunderScheme: "http",
		ThunderHost: "thunder.amp.localhost", ThunderPort: 8080, ThunderListenPort: 8080,
		AMPPublicURL: "http://localhost:9000", ObserverPublicURL: "http://localhost:9098",
		ConsoleURL:          "http://localhost:3000",
		ConsoleRedirectURIs: []string{"http://localhost:3000/login"},
		CORSOrigins:         []string{"http://localhost:3000"},
		APIClient:           Client{ID: "amp-api-client", Secret: "amp-api-client-secret"},
		ConsoleClient:       Client{ID: "amp-console-client", Secret: "amp-console-client-secret"},
		PublisherClient:     Client{ID: "amp-publisher-client", Secret: "amp-publisher-client-secret"},
		SystemClient:        Client{ID: "amp-system-client", Secret: "amp-system-client-secret"},
		CLIClient:           Client{ID: "amctl", RedirectURIs: []string{"http://127.0.0.1:10325/callback"}},
		Admin:               User{Username: "admin", Email: "admin@example.com", GivenName: "Admin", FamilyName: "User"},
	}
}

// Run writes the ThunderID files into thunderDir and the AI Gateway files
// into gatewayDir, skipping an empty one.
func Run(thunderDir, gatewayDir string) error {
	if thunderDir != "" {
		cfg, err := LoadThunderConfig()
		if err != nil {
			return fmt.Errorf("ThunderID configuration: %w", err)
		}
		if err := WriteThunder(cfg, thunderDir); err != nil {
			return fmt.Errorf("ThunderID configuration: %w", err)
		}
		slog.Info("Wrote the ThunderID configuration", "dir", thunderDir)
	}
	if gatewayDir != "" {
		cfg, err := LoadGatewayConfig()
		if err != nil {
			return fmt.Errorf("AI Gateway configuration: %w", err)
		}
		if err := WriteGateway(cfg, gatewayDir); err != nil {
			return fmt.Errorf("AI Gateway configuration: %w", err)
		}
		slog.Info("Wrote the AI Gateway configuration", "dir", gatewayDir)
	}
	return nil
}
