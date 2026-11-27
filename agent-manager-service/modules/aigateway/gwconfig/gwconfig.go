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

// Package gwconfig holds the AI Gateway's settings. Each is a module setting the Runtime reads as
// MODULE_AI_GATEWAY_<KEY> (environment variable or configuration file); Load parses and checks them all.
package gwconfig

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

// envPrefix is how the Runtime names this module's settings; it is used only in error messages.
const envPrefix = "MODULE_AI_GATEWAY_"

// Settings are the gateway's settings.
type Settings struct {
	DefaultGateway DefaultGateway
	// A2ACardFetchDialAddr, when set, is the host:port every platform card fetch dials instead of its URL host.
	A2ACardFetchDialAddr string
	ManifestCache        ManifestCache
	WebSocket            WebSocket
	DeploymentTimeout    DeploymentTimeout
	Reconcile            Reconcile
	Failure              Failure
	// AgentCORS is the CORS policy an agent's gateway route gets unless its settings say otherwise.
	AgentCORS CORS
}

// DefaultGateway registers one gateway at startup, so a bundled gateway connects with no manual step. An empty
// Name turns it off.
type DefaultGateway struct {
	Name        string
	DisplayName string
	Vhost       string
	// FunctionalityType is INGRESS, EGRESS or BOTH.
	FunctionalityType string
	// OrgOUID is the Thunder OU of the organization the gateway belongs to,
	// the core's default organization unless set.
	OrgOUID string
	// Token is the registration token the gateway connects with, read from TokenFile when empty.
	Token     string
	TokenFile string
}

// ManifestCache selects the gateway-manifest cache: "memory" (one replica only) or "redis" (required with
// several replicas, so they agree on which policies gateways report).
type ManifestCache struct {
	Backend string
	Redis   Redis
}

// Redis configures the "redis" manifest cache.
type Redis struct {
	Host       string
	Port       int
	Password   string `json:"-"`
	DB         int
	TLSEnabled bool
}

// WebSocket limits the gateway controllers' connections.
type WebSocket struct {
	MaxConnections int
	// ConnectionTimeoutSeconds is how long a connection may stay silent before it is dropped.
	ConnectionTimeoutSeconds int
	// RateLimitPerMin is how many connection attempts one gateway may make per minute.
	RateLimitPerMin int
}

// DeploymentTimeout sets how often the timeout sweeper runs and how long a DEPLOYING or UNDEPLOYING row may wait
// for the gateway's ack before it is FAILED with DEPLOYMENT_TIMEOUT.
type DeploymentTimeout struct {
	SweepInterval time.Duration
	Timeout       time.Duration
}

// Reconcile sets the reconcile worker's intervals, the render claim's lease and the render attempt budget.
type Reconcile struct {
	// RenderInterval is how often due renders are claimed; a save also nudges the worker.
	RenderInterval time.Duration
	// PlaceInterval is how often the fleet-wide placement pass runs; mapping a gateway places at once.
	PlaceInterval time.Duration
	// DriftInterval is how often one replica checks deployed artifacts for drift.
	DriftInterval time.Duration
	// RenderLease is how long a claimed render is held before another replica may claim it again.
	RenderLease time.Duration
	// RenderAttemptBudget is how many failed renders give up a deploy. Waiting for an input never counts.
	RenderAttemptBudget int
}

// Failure sets the platform gateway failure summary: a gateway disconnected for longer than Threshold, and for
// less than MaxAge, counts as failed, and the fleet is unhealthy once PercentageThreshold percent have failed.
type Failure struct {
	Threshold           time.Duration
	MaxAge              time.Duration
	PercentageThreshold float64
}

// CORS is a CORS policy; the lists are comma-separated.
type CORS struct {
	AllowOrigin      string
	AllowMethods     string
	AllowHeaders     string
	AllowCredentials bool
}

// Load reads the settings through get, which returns a setting's value by its module-relative key ("" when
// unset), and reports every invalid one.
func Load(get func(key string) string) (Settings, error) {
	r := reader{get: get}
	s := Settings{
		DefaultGateway: DefaultGateway{
			Name:              strings.TrimSpace(r.str("DEFAULT_GATEWAY_NAME", "")),
			DisplayName:       r.str("DEFAULT_GATEWAY_DISPLAY_NAME", "Default AI Gateway"),
			Vhost:             r.str("DEFAULT_GATEWAY_VHOST", ""),
			FunctionalityType: r.str("DEFAULT_GATEWAY_TYPE", "BOTH"),
			OrgOUID:           r.str("DEFAULT_GATEWAY_ORG_OU_ID", "00000000-0000-0000-0000-000000000001"),
			Token:             strings.TrimSpace(r.str("DEFAULT_GATEWAY_TOKEN", "")),
			TokenFile:         r.str("DEFAULT_GATEWAY_TOKEN_FILE", ""),
		},
		A2ACardFetchDialAddr: r.str("A2A_CARD_FETCH_DIAL_ADDR", ""),
		ManifestCache: ManifestCache{
			Backend: strings.TrimSpace(r.str("MANIFEST_CACHE_BACKEND", "memory")),
			Redis: Redis{
				Host:       r.str("MANIFEST_CACHE_REDIS_HOST", ""),
				Port:       r.positive("MANIFEST_CACHE_REDIS_PORT", 6379),
				Password:   r.str("MANIFEST_CACHE_REDIS_PASSWORD", ""),
				DB:         r.integer("MANIFEST_CACHE_REDIS_DB", 0),
				TLSEnabled: r.boolean("MANIFEST_CACHE_REDIS_TLS_ENABLED", false),
			},
		},
		WebSocket: WebSocket{
			MaxConnections:           r.positive("WEBSOCKET_MAX_CONNECTIONS", 1000),
			ConnectionTimeoutSeconds: r.positive("WEBSOCKET_CONNECTION_TIMEOUT", 30),
			RateLimitPerMin:          r.positive("WEBSOCKET_RATE_LIMIT_PER_MIN", 10),
		},
		DeploymentTimeout: DeploymentTimeout{
			SweepInterval: r.seconds("DEPLOYMENT_TIMEOUT_SWEEP_INTERVAL_SECONDS", 60),
			Timeout:       r.seconds("DEPLOYMENT_TIMEOUT_SECONDS", 300),
		},
		Reconcile: Reconcile{
			RenderInterval:      r.seconds("RECONCILE_RENDER_INTERVAL_SECONDS", 5),
			PlaceInterval:       r.seconds("RECONCILE_PLACE_INTERVAL_SECONDS", 600),
			DriftInterval:       r.seconds("RECONCILE_DRIFT_INTERVAL_SECONDS", 300),
			RenderLease:         r.seconds("RECONCILE_RENDER_LEASE_SECONDS", 120),
			RenderAttemptBudget: r.positive("RECONCILE_RENDER_ATTEMPT_BUDGET", 30),
		},
		Failure: Failure{
			Threshold: r.seconds("FAILURE_THRESHOLD_SECONDS", 300),
			// 604800 = 7 days.
			MaxAge:              r.seconds("FAILURE_MAX_AGE_SECONDS", 604800),
			PercentageThreshold: r.float("FAILURE_PERCENTAGE_THRESHOLD", 10),
		},
		AgentCORS: CORS{
			AllowOrigin:      r.str("AGENT_CORS_ALLOWED_ORIGIN", "*"),
			AllowMethods:     r.str("AGENT_CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,PATCH,OPTIONS"),
			AllowHeaders:     r.str("AGENT_CORS_ALLOWED_HEADERS", "authorization,Content-Type,Origin,X-API-Key"),
			AllowCredentials: r.boolean("AGENT_CORS_ALLOW_CREDENTIALS", false),
		},
	}
	s.check(&r)
	return s, errors.Join(r.errs...)
}

func (s Settings) check(r *reader) {
	if g := s.DefaultGateway; g.Name != "" && (g.Vhost == "" || (g.Token == "" && g.TokenFile == "")) {
		r.fail("%sDEFAULT_GATEWAY_NAME needs %sDEFAULT_GATEWAY_VHOST and %sDEFAULT_GATEWAY_TOKEN or %sDEFAULT_GATEWAY_TOKEN_FILE",
			envPrefix, envPrefix, envPrefix, envPrefix)
	}
	if addr := s.A2ACardFetchDialAddr; addr != "" {
		if host, port, err := net.SplitHostPort(addr); err != nil {
			r.fail("%sA2A_CARD_FETCH_DIAL_ADDR must be host:port: %v", envPrefix, err)
		} else if host == "" || port == "" {
			r.fail("%sA2A_CARD_FETCH_DIAL_ADDR must contain a non-empty host and port", envPrefix)
		}
	}
	switch s.ManifestCache.Backend {
	case "memory":
	case "redis":
		// Only the host is mandatory: an empty password is a legitimate Redis deployment.
		if strings.TrimSpace(s.ManifestCache.Redis.Host) == "" {
			r.fail("%sMANIFEST_CACHE_REDIS_HOST is required when %sMANIFEST_CACHE_BACKEND=redis", envPrefix, envPrefix)
		}
	default:
		r.fail("%sMANIFEST_CACHE_BACKEND %q is not valid (must be \"memory\" or \"redis\")", envPrefix, s.ManifestCache.Backend)
	}
	// An upper bound at or below the lower one leaves an empty window that reports every fleet healthy.
	if s.Failure.MaxAge <= s.Failure.Threshold {
		r.fail("%sFAILURE_MAX_AGE_SECONDS (%v) must be greater than %sFAILURE_THRESHOLD_SECONDS (%v), "+
			"or no gateway can ever be reported as failed", envPrefix, s.Failure.MaxAge, envPrefix, s.Failure.Threshold)
	}
	// Either end would make the verdict a constant; NaN passes a range test, so it is checked by name.
	if p := s.Failure.PercentageThreshold; math.IsNaN(p) || p <= 0 || p > 100 {
		r.fail("%sFAILURE_PERCENTAGE_THRESHOLD must be greater than 0 and at most 100, got %v", envPrefix, p)
	}
	if c := s.AgentCORS; c.AllowOrigin == "*" && c.AllowCredentials {
		r.fail("%sAGENT_CORS_ALLOW_CREDENTIALS cannot be true when %sAGENT_CORS_ALLOWED_ORIGIN is \"*\"", envPrefix, envPrefix)
	}
}

type reader struct {
	get  func(string) string
	errs []error
}

func (r *reader) fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf(format, args...))
}

func (r *reader) str(key, def string) string {
	if v := strings.TrimSpace(r.get(key)); v != "" {
		return v
	}
	return def
}

func (r *reader) integer(key string, def int) int {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.fail("%s%s must be an integer, got %q", envPrefix, key, v)
		return def
	}
	return n
}

func (r *reader) positive(key string, def int) int {
	n := r.integer(key, def)
	if n <= 0 {
		r.fail("%s%s must be greater than 0, got %d", envPrefix, key, n)
	}
	return n
}

func (r *reader) seconds(key string, def int) time.Duration {
	return time.Duration(r.positive(key, def)) * time.Second
}

func (r *reader) float(key string, def float64) float64 {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		r.fail("%s%s must be a number, got %q", envPrefix, key, v)
		return def
	}
	return f
}

func (r *reader) boolean(key string, def bool) bool {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.fail("%s%s must be true or false, got %q", envPrefix, key, v)
		return def
	}
	return b
}
