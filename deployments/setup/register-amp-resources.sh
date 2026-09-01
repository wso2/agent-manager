#!/bin/bash
# Registers the Agent Manager (amp) resource server with its full permission
# tree in Thunder, plus the MCP resource servers (RFC 8707 resource indicators)
# with the permission slices their MCP clients are allowed.
# Runs against the external Thunder endpoint using amp-system-client credentials.
#
# This mirrors the helm bootstrap's 60-amp-resource-server.sh
# (wso2-amp-thunder-extension/templates/amp-thunder-bootstrap.yaml), which is
# authoritative — nothing invokes this script automatically, so a change to the
# identifiers or the permission tree has to land in both.
#
# Usage:
#   ./register-amp-resources.sh
#   THUNDER_URL=http://thunder.amp.localhost:8080 ./register-amp-resources.sh

set -e

THUNDER_URL="${THUNDER_URL:-http://thunder.amp.localhost:8080}"
CLIENT_ID="${THUNDER_CLIENT_ID:-amp-system-client}"
CLIENT_SECRET="${THUNDER_CLIENT_SECRET:-amp-system-client-secret}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log_info()    { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} ✓ $1" >&2; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} ⚠ $1" >&2; }
log_error()   { echo -e "${RED}[ERROR]${NC} ✗ $1" >&2; }

command -v jq >/dev/null 2>&1 || { log_error "jq is required but not installed"; exit 1; }

# ---------------------------------------------------------------------------
# Get a system-scoped token
# ---------------------------------------------------------------------------
log_info "Obtaining system token from $THUNDER_URL ..."
TOKEN_RESPONSE=$(curl -s -X POST "$THUNDER_URL/oauth2/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -d "grant_type=client_credentials&scope=system")

TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
if [[ -z "$TOKEN" ]]; then
  log_error "Failed to obtain system token. Response: $TOKEN_RESPONSE"
  exit 1
fi
log_success "System token obtained."

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
api_call() {
  local method="$1" endpoint="$2" data="${3:-}"
  if [[ -z "$data" ]]; then
    curl -s -w "\n%{http_code}" -X "$method" "$THUNDER_URL$endpoint" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo -e "\n000"
  else
    curl -s -w "\n%{http_code}" -X "$method" "$THUNDER_URL$endpoint" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      -d "$data" 2>/dev/null || echo -e "\n000"
  fi
}

create_or_get_rs() {
  local name="$1" handle="$2" identifier="$3" description="$4" ou_id="$5"
  local payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"identifier\":\"${identifier}\",\"description\":\"${description}\",\"ouId\":\"${ou_id}\"}"
  local response http_code body id

  response=$(api_call POST "/resource-servers" "$payload")
  http_code="${response: -3}"; body="${response%???}"

  if [[ "$http_code" == "201" ]] || [[ "$http_code" == "200" ]]; then
    id=$(echo "$body" | jq -r '.id')
    log_success "Resource server '${identifier}' created (id: $id)"
    echo "$id"; return 0
  fi

  if [[ "$http_code" == "409" ]]; then
    log_warning "Resource server '${identifier}' already exists, retrieving ID..."
    response=$(api_call GET "/resource-servers")
    http_code="${response: -3}"; body="${response%???}"
    [[ "$http_code" != "200" ]] && { log_error "Failed to list resource servers (HTTP $http_code)"; exit 1; }
    id=$(echo "$body" | jq -r --arg ident "$identifier" '.resourceServers[] | select(.identifier == $ident) | .id')
    [[ -z "$id" ]] && { log_error "Resource server '${identifier}' not found after 409"; exit 1; }
    log_success "Found existing resource server '${identifier}' (id: $id)"
    echo "$id"; return 0
  fi

  log_error "Failed to create resource server '${identifier}' (HTTP $http_code): $body"
  exit 1
}

create_or_get_resource() {
  local rs_id="$1" name="$2" handle="$3" description="$4" parent_id="${5:-}"
  local payload list_url response http_code body id

  if [[ -n "$parent_id" ]]; then
    payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"description\":\"${description}\",\"parent\":\"${parent_id}\"}"
    list_url="/resource-servers/${rs_id}/resources?parentId=${parent_id}"
  else
    payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"description\":\"${description}\"}"
    list_url="/resource-servers/${rs_id}/resources"
  fi

  response=$(api_call POST "/resource-servers/${rs_id}/resources" "$payload")
  http_code="${response: -3}"; body="${response%???}"

  if [[ "$http_code" == "201" ]] || [[ "$http_code" == "200" ]]; then
    id=$(echo "$body" | jq -r '.id')
    echo "$id"; return 0
  fi

  if [[ "$http_code" == "409" ]]; then
    log_warning "Resource '${handle}' already exists, retrieving ID..."
    response=$(api_call GET "${list_url}")
    http_code="${response: -3}"; body="${response%???}"
    [[ "$http_code" != "200" ]] && { log_error "Failed to list resources (HTTP $http_code)"; exit 1; }
    id=$(echo "$body" | jq -r --arg h "$handle" '.resources[] | select(.handle == $h) | .id')
    [[ -z "$id" ]] && { log_error "Resource '${handle}' not found after 409"; exit 1; }
    log_success "Found existing resource '${handle}' (id: $id)"
    echo "$id"; return 0
  fi

  log_error "Failed to create resource '${handle}' (HTTP $http_code): $body"
  exit 1
}

create_action() {
  local rs_id="$1" res_id="$2" name="$3" handle="$4" description="$5"
  local payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"description\":\"${description}\"}"
  local response http_code body

  response=$(api_call POST "/resource-servers/${rs_id}/resources/${res_id}/actions" "$payload")
  http_code="${response: -3}"; body="${response%???}"

  if [[ "$http_code" == "201" ]] || [[ "$http_code" == "200" ]] || [[ "$http_code" == "409" ]]; then
    return 0
  fi

  log_error "Failed to create action '${handle}' on resource ${res_id} (HTTP $http_code): $body"
  exit 1
}

# delete_action_if_present retires an action a released version of the platform
# registered and this one no longer enforces. It exists because the console's
# role editor lists assignable permissions from Thunder's own action tree (see
# ListAMPPermissions in clients/thundersvc/identity_client.go), not from the
# service's catalog: an action left behind after a rename keeps offering an admin
# a scope that grants nothing, and the role they build with it silently does less
# than it says.
#
# Absence is success, so re-running this script is safe and so is running it
# against a fresh Thunder that never had the action.
delete_action_if_present() {
  local rs_id="$1" res_id="$2" handle="$3"
  local response http_code body action_id

  # limit is explicit because the endpoint paginates and the default page size is
  # Thunder's to choose; one page well above any resource's action count keeps
  # this a single call.
  response=$(api_call GET "/resource-servers/${rs_id}/resources/${res_id}/actions?limit=200")
  http_code="${response: -3}"; body="${response%???}"
  [[ "$http_code" == "404" ]] && return 0
  [[ "$http_code" != "200" ]] && { log_error "Failed to list actions on resource ${res_id} (HTTP $http_code): $body"; exit 1; }

  action_id=$(echo "$body" | jq -r --arg h "$handle" '.actions[]? | select(.handle == $h) | .id')
  [[ -z "$action_id" ]] && return 0

  response=$(api_call DELETE "/resource-servers/${rs_id}/resources/${res_id}/actions/${action_id}")
  http_code="${response: -3}"; body="${response%???}"
  case "$http_code" in
    200|204|404)
      log_success "Retired deprecated action '${handle}'"
      return 0
      ;;
  esac
  log_error "Failed to delete deprecated action '${handle}' on resource ${res_id} (HTTP $http_code): $body"
  exit 1
}

# ---------------------------------------------------------------------------
# Helper: register the amp permission tree under a resource server
# Usage: register_amp_permissions <rs_id> <mode> [parent_res_id]
#   mode: full                    — all resources and actions
#         amp-minus-observability — everything except the observability
#                                   resource and its actions
#         observability-only      — only the observability resource and
#                                   its actions
# When parent_res_id is given, resources are created as children of it so
# the derived permission strings start at that resource's permission (used
# by the MCP resource servers, which carry no handle of their own).
# Mirrors register_amp_permissions in the helm bootstrap
# (wso2-amp-thunder-extension, 60-amp-resource-server.sh) — keep in sync.
# ---------------------------------------------------------------------------
register_amp_permissions() {
  local rs_id="$1" mode="$2" parent_id="${3:-}"
  local r_org r_project r_env r_gw r_dp r_pipe r_git r_llmt r_llm r_mcp \
        r_scope r_agentid r_proxy r_eval r_agent r_agent_kind r_mon r_obs \
        r_role r_group r_cat r_repo r_profile

  case "$mode" in
    full|amp-minus-observability|observability-only) ;;
    *)
      log_error "register_amp_permissions: unrecognized mode '${mode}' (expected full, amp-minus-observability, or observability-only)"
      exit 1
      ;;
  esac

  if [[ "$mode" != "observability-only" ]]; then
    log_info "Creating resources..."

    r_org=$(create_or_get_resource        "$rs_id" "Organization"            "org"                    "Organizational unit management"       "$parent_id")
    r_project=$(create_or_get_resource    "$rs_id" "Project"                  "project"                "Project management"                   "$parent_id")
    r_env=$(create_or_get_resource        "$rs_id" "Environment"              "environment"            "Environment management"               "$parent_id")
    r_gw=$(create_or_get_resource         "$rs_id" "Gateway"                  "gateway"                "Gateway management"                   "$parent_id")
    r_dp=$(create_or_get_resource         "$rs_id" "Data Plane"               "data-plane"             "Data plane visibility"                "$parent_id")
    r_pipe=$(create_or_get_resource       "$rs_id" "Deployment Pipeline"      "deployment-pipeline"    "Deployment pipeline visibility"       "$parent_id")
    r_git=$(create_or_get_resource        "$rs_id" "Git Secret"               "git-secret"             "Git credential management"            "$parent_id")
    r_llmt=$(create_or_get_resource       "$rs_id" "LLM Provider Template"    "llm-provider-template"  "LLM provider template management"     "$parent_id")
    r_llm=$(create_or_get_resource        "$rs_id" "LLM Provider"             "llm-provider"           "LLM provider management"              "$parent_id")
    r_mcp=$(create_or_get_resource        "$rs_id" "MCP Server"               "mcp-server"             "MCP server management"                "$parent_id")
    r_scope=$(create_or_get_resource      "$rs_id" "Scope"                    "scope"                  "Scope catalog management"             "$parent_id")
    r_agentid=$(create_or_get_resource    "$rs_id" "Agent Identity"           "agent-identity"         "Agent identity management"            "$parent_id")
    r_proxy=$(create_or_get_resource      "$rs_id" "LLM Proxy"                "llm-proxy"              "LLM proxy management"                 "$parent_id")
    r_eval=$(create_or_get_resource       "$rs_id" "Evaluator"                "evaluator"              "Evaluator management"                 "$parent_id")
    r_agent=$(create_or_get_resource      "$rs_id" "Agent"                    "agent"                  "Agent management"                     "$parent_id")
    r_agent_kind=$(create_or_get_resource "$rs_id" "Agent Kind"               "agent-kind"             "Agent kind management"                "$parent_id")
    r_mon=$(create_or_get_resource        "$rs_id" "Monitor"                  "monitor"                "Monitor management"                   "$parent_id")
    r_role=$(create_or_get_resource       "$rs_id" "Role"                     "role"                   "Role management"                      "$parent_id")
    r_group=$(create_or_get_resource      "$rs_id" "Group"                    "group"                  "Group management"                     "$parent_id")
    r_cat=$(create_or_get_resource        "$rs_id" "Catalog"                  "catalog"                "Resource catalog"                     "$parent_id")
    r_repo=$(create_or_get_resource       "$rs_id" "Repository"               "repository"             "Source repository browsing"           "$parent_id")
    r_profile=$(create_or_get_resource    "$rs_id" "Profile"                  "profile"                "User profile management"              "$parent_id")

    log_info "Creating actions..."

    # org actions  → amp:org:<action>
    create_action "$rs_id" "$r_org"          "View"                   "view"                   "View organizational details"
    create_action "$rs_id" "$r_org"          "Modify Settings"        "modify-settings"        "Modify organizational settings"
    create_action "$rs_id" "$r_org"          "Invite Member"          "invite-member"          "Invite members to the organization"
    create_action "$rs_id" "$r_org"          "Remove Member"          "remove-member"          "Remove members from the organization"
    create_action "$rs_id" "$r_org"          "Assign Role"            "assign-role"            "Assign or revoke roles"
    create_action "$rs_id" "$r_org"          "Manage IDP"             "manage-idp"             "Configure Identity Provider"
    create_action "$rs_id" "$r_org"          "Manage Service Account" "manage-service-account" "Manage service accounts"

    # profile actions  → amp:profile:<action>
    create_action "$rs_id" "$r_profile"      "Read"                     "read"                 "View user profile information"
    create_action "$rs_id" "$r_profile"      "Update Attributes"        "update-attributes"    "Update user attributes"

    # project actions  → amp:project:<action>
    create_action "$rs_id" "$r_project"      "Create"                 "create"                 "Create a project"
    create_action "$rs_id" "$r_project"      "Read"                   "read"                   "View project details"
    create_action "$rs_id" "$r_project"      "Update"                 "update"                 "Update project settings"
    create_action "$rs_id" "$r_project"      "Delete"                 "delete"                 "Delete a project"

    # environment actions  → amp:environment:<action>
    create_action "$rs_id" "$r_env"          "Create"                 "create"                 "Create an environment"
    create_action "$rs_id" "$r_env"          "Read"                   "read"                   "View environment details"
    create_action "$rs_id" "$r_env"          "Update"                 "update"                 "Update an environment"
    create_action "$rs_id" "$r_env"          "Delete"                 "delete"                 "Delete an environment"

    # gateway actions  → amp:gateway:<action>
    create_action "$rs_id" "$r_gw"           "Create"                 "create"                 "Register a gateway"
    create_action "$rs_id" "$r_gw"           "Read"                   "read"                   "View gateway details and status"
    create_action "$rs_id" "$r_gw"           "Update"                 "update"                 "Update gateway configuration"
    create_action "$rs_id" "$r_gw"           "Delete"                 "delete"                 "Delete a gateway"
    create_action "$rs_id" "$r_gw"           "Manage Token"           "token-manage"           "Create, list, and delete gateway tokens"

    # data-plane actions  → amp:data-plane:<action>
    create_action "$rs_id" "$r_dp"           "Read"                   "read"                   "View data planes"

    # deployment-pipeline actions  → amp:deployment-pipeline:<action>
    create_action "$rs_id" "$r_pipe"         "Create"                 "create"                 "Create a deployment pipeline"
    create_action "$rs_id" "$r_pipe"         "Read"                   "read"                   "View deployment pipelines"
    create_action "$rs_id" "$r_pipe"         "Update"                 "update"                 "Update a deployment pipeline"
    create_action "$rs_id" "$r_pipe"         "Delete"                 "delete"                 "Delete a deployment pipeline"

    # git-secret actions  → amp:git-secret:<action>
    create_action "$rs_id" "$r_git"          "Create"                 "create"                 "Create a git secret"
    create_action "$rs_id" "$r_git"          "Read"                   "read"                   "List git secrets"
    create_action "$rs_id" "$r_git"          "Delete"                 "delete"                 "Delete a git secret"

    # llm-provider-template actions  → amp:llm-provider-template:<action>
    create_action "$rs_id" "$r_llmt"         "Create"                 "create"                 "Create a provider template"
    create_action "$rs_id" "$r_llmt"         "Read"                   "read"                   "View provider templates"
    create_action "$rs_id" "$r_llmt"         "Update"                 "update"                 "Update a provider template"
    create_action "$rs_id" "$r_llmt"         "Delete"                 "delete"                 "Delete a provider template"

    # llm-provider actions  → amp:llm-provider:<action>
    create_action "$rs_id" "$r_llm"          "Create"                 "create"                 "Create an LLM provider"
    create_action "$rs_id" "$r_llm"          "Read"                   "read"                   "View LLM providers and deployments"
    create_action "$rs_id" "$r_llm"          "Update"                 "update"                 "Update an LLM provider"
    create_action "$rs_id" "$r_llm"          "Delete"                 "delete"                 "Delete an LLM provider"
    create_action "$rs_id" "$r_llm"          "Configure Guardrail"    "configure-guardrail"    "Configure guardrails, rate limits, and budgets"
    create_action "$rs_id" "$r_llm"          "Connect"                "connect"                "Connect an agent to an LLM provider"
    create_action "$rs_id" "$r_llm"          "Deploy"                 "deploy"                 "Deploy, undeploy, and restore an LLM provider"
    create_action "$rs_id" "$r_llm"          "Manage API Key"         "api-key-manage"         "Create, update, and delete LLM provider API keys"

    # mcp-server actions  → amp:mcp-server:<action>
    create_action "$rs_id" "$r_mcp"          "Create"                 "create"                 "Create an MCP server"
    create_action "$rs_id" "$r_mcp"          "Read"                   "read"                   "View MCP servers"
    create_action "$rs_id" "$r_mcp"          "Update"                 "update"                 "Update an MCP server"
    create_action "$rs_id" "$r_mcp"          "Delete"                 "delete"                 "Delete an MCP server"
    create_action "$rs_id" "$r_mcp"          "Configure Guardrail"    "configure-guardrail"    "Configure guardrails on an MCP server"
    create_action "$rs_id" "$r_mcp"          "Connect"                "connect"                "Connect an agent to an MCP server"
    create_action "$rs_id" "$r_mcp"          "Manage API Key"         "api-key-manage"         "Create, update, and delete MCP server API keys"

    # scope actions  → amp:scope:<action>
    create_action "$rs_id" "$r_scope"        "Create"                 "create"                 "Create a scope"
    create_action "$rs_id" "$r_scope"        "Read"                   "read"                   "View scopes"
    create_action "$rs_id" "$r_scope"        "Update"                 "update"                 "Update a scope"
    create_action "$rs_id" "$r_scope"        "Delete"                 "delete"                 "Delete a scope"

    # agent-identity actions  → amp:agent-identity:<action>
    create_action "$rs_id" "$r_agentid"      "Create"                 "create"                 "Create an agent identity"
    create_action "$rs_id" "$r_agentid"      "Read"                   "read"                   "View agent identities"
    create_action "$rs_id" "$r_agentid"      "Update"                 "update"                 "Update an agent identity"
    create_action "$rs_id" "$r_agentid"      "Delete"                 "delete"                 "Delete an agent identity"

    # llm-proxy actions  → amp:llm-proxy:<action>
    create_action "$rs_id" "$r_proxy"        "Create"                 "create"                 "Create an LLM proxy"
    create_action "$rs_id" "$r_proxy"        "Read"                   "read"                   "View LLM proxies and deployments"
    create_action "$rs_id" "$r_proxy"        "Update"                 "update"                 "Update an LLM proxy"
    create_action "$rs_id" "$r_proxy"        "Delete"                 "delete"                 "Delete an LLM proxy"
    create_action "$rs_id" "$r_proxy"        "Deploy"                 "deploy"                 "Deploy, undeploy, and restore an LLM proxy"
    create_action "$rs_id" "$r_proxy"        "Manage API Key"         "api-key-manage"         "Create, update, and delete LLM proxy API keys"

    # evaluator actions  → amp:evaluator:<action>
    create_action "$rs_id" "$r_eval"         "Create"                 "create"                 "Create a custom evaluator"
    create_action "$rs_id" "$r_eval"         "Read"                   "read"                   "View evaluators"
    create_action "$rs_id" "$r_eval"         "Update"                 "update"                 "Update a custom evaluator"
    create_action "$rs_id" "$r_eval"         "Delete"                 "delete"                 "Delete a custom evaluator"

    # The environment tier replaced three actions: promote and the two
    # deploy-<tier> grants became env-non-production and env-production, which
    # gate deploy, promote AND deployment-state changes on where the action
    # lands. The old handles are retired before the new ones are created so an
    # installation upgraded by re-running this script stops offering them.
    delete_action_if_present "$rs_id" "$r_agent" "promote"
    delete_action_if_present "$rs_id" "$r_agent" "deploy-non-production"
    delete_action_if_present "$rs_id" "$r_agent" "deploy-production"

    # agent actions  → amp:agent:<action>
    create_action "$rs_id" "$r_agent"        "Create"                 "create"                 "Create an agent"
    create_action "$rs_id" "$r_agent"        "Read"                   "read"                   "View agent details, builds, deployments, and configs"
    create_action "$rs_id" "$r_agent"        "Update"                 "update"                 "Update agent configuration and resource configs"
    create_action "$rs_id" "$r_agent"        "Delete"                 "delete"                 "Delete an agent"
    create_action "$rs_id" "$r_agent"        "Build"                  "build"                  "Trigger an agent build"
    create_action "$rs_id" "$r_agent"        "Rollback"               "rollback"               "Rollback an agent deployment"
    create_action "$rs_id" "$r_agent"        "Suspend"                "suspend"                "Suspend or stop an agent deployment"
    create_action "$rs_id" "$r_agent"        "Act on Non-Production Environments" "env-non-production" "Deploy, promote, or suspend an agent in a non-production environment"
    create_action "$rs_id" "$r_agent"        "Act on Production Environments"     "env-production"     "Deploy, promote, or suspend an agent in an environment flagged isProduction"
    create_action "$rs_id" "$r_agent"        "Manage Token"           "token-manage"           "Generate agent tokens"
    create_action "$rs_id" "$r_agent"        "Manage API Key"         "api-key-manage"         "Create, update, and delete agent API keys"

    # agent-kind actions  → amp:agent-kind:<action>
    create_action "$rs_id" "$r_agent_kind"   "Create"                 "create"                 "Create an agent kind"
    create_action "$rs_id" "$r_agent_kind"   "Read"                   "read"                   "View agent kinds and versions"
    create_action "$rs_id" "$r_agent_kind"   "Update"                 "update"                 "Update an agent kind"
    create_action "$rs_id" "$r_agent_kind"   "Delete"                 "delete"                 "Delete an agent kind"

    # monitor actions  → amp:monitor:<action>
    create_action "$rs_id" "$r_mon"          "Create"                 "create"                 "Create a monitor"
    create_action "$rs_id" "$r_mon"          "Read"                   "read"                   "View monitors, runs, and run logs"
    create_action "$rs_id" "$r_mon"          "Update"                 "update"                 "Update a monitor"
    create_action "$rs_id" "$r_mon"          "Delete"                 "delete"                 "Delete a monitor"
    create_action "$rs_id" "$r_mon"          "Execute"                "execute"                "Start, stop, and rerun monitors"
    create_action "$rs_id" "$r_mon"          "Read Score"             "score-read"             "View scores, breakdowns, and timeseries"
    create_action "$rs_id" "$r_mon"          "Publish Score"          "score-publish"          "Publish monitor scores (internal/system use)"

    # role actions  → amp:role:<action>
    create_action "$rs_id" "$r_role"         "Create"                 "create"                 "Create a custom role"
    create_action "$rs_id" "$r_role"         "Read"                   "read"                   "View roles and their permissions"
    create_action "$rs_id" "$r_role"         "Update"                 "update"                 "Update a custom role"
    create_action "$rs_id" "$r_role"         "Delete"                 "delete"                 "Delete a custom role"

    # group actions  → amp:group:<action>
    create_action "$rs_id" "$r_group"        "Create"                 "create"                 "Create a group"
    create_action "$rs_id" "$r_group"        "Read"                   "read"                   "View groups and their members"
    create_action "$rs_id" "$r_group"        "Update"                 "update"                 "Update a group"
    create_action "$rs_id" "$r_group"        "Delete"                 "delete"                 "Delete a group"

    # catalog actions  → amp:catalog:<action>
    create_action "$rs_id" "$r_cat"          "Read"                   "read"                   "View the resource catalog"

    # repository actions  → amp:repository:<action>
    create_action "$rs_id" "$r_repo"         "Read"                   "read"                   "Browse repository branches and commits"
  fi

  if [[ "$mode" != "amp-minus-observability" ]]; then
    # observability resource and actions  → amp:observability:<action>
    r_obs=$(create_or_get_resource "$rs_id" "Observability" "observability" "Observability dashboards and metrics" "$parent_id")

    create_action "$rs_id" "$r_obs"          "Trace Read"             "trace-read"             "Read traces and spans"
    create_action "$rs_id" "$r_obs"          "Log Read"               "log-read"               "Read runtime logs"
    create_action "$rs_id" "$r_obs"          "Build Log Read"         "build-log-read"         "Read build logs"
    create_action "$rs_id" "$r_obs"          "Metric Read"            "metric-read"            "Read runtime and infrastructure metrics"
  fi
}

# ---------------------------------------------------------------------------
# Helper: register an MCP resource server (empty handle) with its slice of
# the amp permission tree. Replaces a legacy resource server carrying a
# non-empty handle: the handle is immutable and would prefix every derived
# permission (e.g. amp-am:amp:project:read), producing scope strings
# agent-manager-service will never accept. MCP resource servers are pure
# mirrors with no independent state, so delete and recreate is safe.
# An empty identifier skips the resource server, mirroring the chart's
# `optional: true` entries.
# Usage: register_mcp_resource_server <name> <identifier> <description> <permission_set>
# ---------------------------------------------------------------------------
register_mcp_resource_server() {
  local name="$1" identifier="$2" description="$3" permission_set="$4"
  local rs_id response http_code body handle amp_root

  if [[ -z "$identifier" ]]; then
    log_info "Skipping MCP resource server '$name' (no identifier configured)"
    return 0
  fi

  rs_id=$(create_or_get_rs "$name" "" "$identifier" "$description" "$DEFAULT_OU_ID")
  log_info "MCP resource server '$identifier' ready (id: $rs_id)"

  response=$(api_call GET "/resource-servers/${rs_id}")
  http_code="${response: -3}"; body="${response%???}"
  [[ "$http_code" != "200" ]] && { log_error "Failed to fetch resource server ${rs_id} (HTTP $http_code): $body"; exit 1; }
  handle=$(echo "$body" | jq -r '.handle // empty')
  if [[ -n "$handle" ]]; then
    log_warning "Resource server '$identifier' has legacy handle '$handle'; deleting and recreating it with an empty handle..."
    response=$(api_call DELETE "/resource-servers/${rs_id}")
    http_code="${response: -3}"; body="${response%???}"
    if [[ "$http_code" != "200" ]] && [[ "$http_code" != "204" ]]; then
      log_error "Failed to delete legacy resource server ${rs_id} (HTTP $http_code): $body"
      exit 1
    fi
    rs_id=$(create_or_get_rs "$name" "" "$identifier" "$description" "$DEFAULT_OU_ID")
    log_info "MCP resource server '$identifier' recreated (id: $rs_id)"
  fi

  amp_root=$(create_or_get_resource "$rs_id" "Agent Manager" "amp" "Root of the amp permission tree")
  register_amp_permissions "$rs_id" "$permission_set" "$amp_root"
  log_info "MCP resource server '$identifier': '$permission_set' permission set registered."
}

# ===========================================================================
# 0. Fetch default OU ID (required by Thunder for resource server creation)
# ===========================================================================
log_info "Fetching default organization unit ID..."
OU_RESPONSE=$(api_call GET "/organization-units/tree/default")
OU_HTTP="${OU_RESPONSE: -3}"; OU_BODY="${OU_RESPONSE%???}"
if [[ "$OU_HTTP" != "200" ]]; then
  log_error "Failed to fetch default OU (HTTP $OU_HTTP): $OU_BODY"
  exit 1
fi
DEFAULT_OU_ID=$(echo "$OU_BODY" | jq -r '.id')
if [[ -z "$DEFAULT_OU_ID" ]]; then
  log_error "Could not extract default OU ID from response"
  exit 1
fi
log_success "Default OU ID: $DEFAULT_OU_ID"

# ===========================================================================
# 1. Resource server
# ===========================================================================
log_info "Creating 'amp' resource server..."
RS_ID=$(create_or_get_rs "Agent Manager API" "amp" "urn:wso2:amp" "Agent Manager platform permissions" "$DEFAULT_OU_ID")
log_info "Resource server ready (id: $RS_ID)"

# ===========================================================================
# 2. Register the amp permission tree
# ===========================================================================
register_amp_permissions "$RS_ID" full

log_success "Agent Manager resource server registration complete (all amp permissions registered)."

# ===========================================================================
# 3. MCP resource servers (RFC 8707 resource indicators)
# ===========================================================================
# MCP clients (Claude Code etc.) send the MCP endpoint URL as the OAuth
# `resource` parameter. Thunder rejects the
# authorize request with invalid_target unless that value exactly matches a
# registered resource-server identifier, so each MCP endpoint's public URL is
# registered here as its own resource server.
#
# Thunder also downscopes granted scopes to the permissions DEFINED on the
# resource server named in `resource`, so each MCP resource server mirrors
# the slice of the amp permission tree its client is allowed. The handle MUST
# stay empty: Thunder prefixes derived permissions with the resource-server
# handle, and the scope strings must come out identical to the amp resource
# server's (amp:project:read etc.). The tree is rooted under a top-level
# "amp" resource instead.
# Identifiers match exactly, so each origin amp-api is reachable on needs its
# own entry; set one to "" to skip it. The dev origin is the docker-compose
# stack's published host port.
# Unset-only defaults, so an explicit "" survives to the skip above.
AM_MCP_RESOURCE="${AM_MCP_RESOURCE-http://api.amp.localhost:8080/mcp}"
AM_MCP_DEV_RESOURCE="${AM_MCP_DEV_RESOURCE-http://localhost:9000/mcp}"
OBSERVER_MCP_RESOURCE="${OBSERVER_MCP_RESOURCE-http://traces.amp.localhost:11080/mcp}"

log_info "Registering MCP resource servers..."
register_mcp_resource_server "AMP Agent Manager MCP" "$AM_MCP_RESOURCE" "Resource identifier for the agent-manager MCP endpoint" "amp-minus-observability"
register_mcp_resource_server "AMP Agent Manager MCP (dev)" "$AM_MCP_DEV_RESOURCE" "Resource identifier for the agent-manager MCP endpoint on the docker-compose dev stack" "amp-minus-observability"
register_mcp_resource_server "AMP Observer MCP" "$OBSERVER_MCP_RESOURCE" "Resource identifier for the observer MCP endpoint" "observability-only"
log_success "MCP resource servers registered."
