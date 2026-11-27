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
package aigateway

import "github.com/wso2/agent-manager/agent-manager-service/modkit"

// settings documents what the module reads with Runtime.Config, and what
// "amp init ai-gateway" reads for the bundled gateway's config.toml.
var settings = []modkit.Setting{
	{Key: "DEFAULT_GATEWAY_NAME", Description: "Registers a gateway with this name at startup, so a bundled gateway connects with no manual step; empty registers none."},
	{Key: "DEFAULT_GATEWAY_DISPLAY_NAME", Default: "Default AI Gateway", Description: "The registered gateway's display name."},
	{Key: "DEFAULT_GATEWAY_VHOST", Description: "The registered gateway's virtual host, the URL agents reach it at."},
	{Key: "DEFAULT_GATEWAY_TYPE", Default: "BOTH", Description: "What the registered gateway handles: INGRESS, EGRESS or BOTH."},
	{Key: "DEFAULT_GATEWAY_ORG_OU_ID", Default: "00000000-0000-0000-0000-000000000001", Description: "The organization the registered gateway belongs to: by default the trial organization file sign-in names. Change it together with the core's default.org_ou_id."},
	{Key: "DEFAULT_GATEWAY_TOKEN", Description: "The registered gateway's registration token; generated and kept by amp init when unset.", Secret: true},
	{Key: "DEFAULT_GATEWAY_TOKEN_FILE", Description: "A file holding the registration token, read when DEFAULT_GATEWAY_TOKEN is unset."},
	{Key: "A2A_CARD_FETCH_DIAL_ADDR", Description: "host:port every A2A agent card fetch dials instead of the card URL's host."},
	{Key: "MANIFEST_CACHE_BACKEND", Default: "memory", Description: "Where deployment manifests are cached: memory or redis."},
	{Key: "MANIFEST_CACHE_REDIS_HOST", Description: "Redis host for the manifest cache."},
	{Key: "MANIFEST_CACHE_REDIS_PORT", Default: "6379", Description: "Redis port for the manifest cache."},
	{Key: "MANIFEST_CACHE_REDIS_PASSWORD", Description: "Redis password for the manifest cache.", Secret: true},
	{Key: "MANIFEST_CACHE_REDIS_DB", Default: "0", Description: "Redis database number for the manifest cache."},
	{Key: "MANIFEST_CACHE_REDIS_TLS_ENABLED", Default: "false", Description: "Connect to Redis over TLS."},
	{Key: "WEBSOCKET_MAX_CONNECTIONS", Default: "1000", Description: "Most gateway connections the control plane accepts."},
	{Key: "WEBSOCKET_CONNECTION_TIMEOUT", Default: "30", Description: "Seconds a gateway connection may stay silent before it is dropped."},
	{Key: "WEBSOCKET_RATE_LIMIT_PER_MIN", Default: "10", Description: "Connection attempts one gateway may make per minute."},
	{Key: "DEPLOYMENT_TIMEOUT_SWEEP_INTERVAL_SECONDS", Default: "60", Description: "How often deployments that never completed are looked for."},
	{Key: "DEPLOYMENT_TIMEOUT_SECONDS", Default: "300", Description: "How long a deployment may stay in progress before it is marked failed."},
	{Key: "RECONCILE_RENDER_INTERVAL_SECONDS", Default: "5", Description: "How often due renders are claimed; a save also starts one."},
	{Key: "RECONCILE_PLACE_INTERVAL_SECONDS", Default: "600", Description: "How often the fleet-wide placement pass runs."},
	{Key: "RECONCILE_DRIFT_INTERVAL_SECONDS", Default: "300", Description: "How often deployed artifacts are checked for drift."},
	{Key: "RECONCILE_RENDER_LEASE_SECONDS", Default: "120", Description: "How long a claimed render is held before another replica may claim it."},
	{Key: "RECONCILE_RENDER_ATTEMPT_BUDGET", Default: "30", Description: "Failed renders that give up a deploy; waiting for an input never counts."},
	{Key: "FAILURE_THRESHOLD_SECONDS", Default: "300", Description: "A gateway disconnected for longer than this counts as failed."},
	{Key: "FAILURE_MAX_AGE_SECONDS", Default: "604800", Description: "A gateway disconnected for longer than this no longer counts as failed."},
	{Key: "FAILURE_PERCENTAGE_THRESHOLD", Default: "10", Description: "Percentage of failed gateways at which the fleet is reported unhealthy."},
	{Key: "AGENT_CORS_ALLOWED_ORIGIN", Default: "*", Description: "CORS origins an agent's gateway route allows unless its settings say otherwise."},
	{Key: "AGENT_CORS_ALLOWED_METHODS", Default: "GET,POST,PUT,DELETE,PATCH,OPTIONS", Description: "CORS methods an agent's gateway route allows."},
	{Key: "AGENT_CORS_ALLOWED_HEADERS", Default: "authorization,Content-Type,Origin,X-API-Key", Description: "CORS headers an agent's gateway route allows."},
	{Key: "AGENT_CORS_ALLOW_CREDENTIALS", Default: "false", Description: "Whether an agent's gateway route allows credentials."},
	{Key: "CONTROLPLANE_HOST", Description: "amp init: host:port of Agent Manager's gateway listener, written into the gateway's config.toml."},
	{Key: "ADMIN_PASSWORD", Description: "amp init: the gateway's admin password; generated and kept when unset.", Secret: true},
	{Key: "AMP_JWKS_URL", Description: "amp init: Agent Manager's key set, which the gateway validates agent tokens with."},
	{Key: "THUNDER_ISSUER", Description: "amp init: ThunderID's token issuer, as the gateway checks it."},
	{Key: "THUNDER_JWKS_URL", Description: "amp init: ThunderID's key set URL, as the gateway dials it."},
	{Key: "HOST", Description: "amp init: the gateway's public host name, for MCP auth metadata and its listener certificate."},
	{Key: "LISTENER_PORT", Default: "19080", Description: "amp init: the gateway's listener port."},
	{Key: "FILE_OWNER", Description: "amp init: uid:gid the gateway's files are given."},
}
