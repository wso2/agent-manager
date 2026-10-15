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

package services

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/policyhub"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// policyAPIKind selects which listing a policy catalog is built for.
type policyAPIKind string

const (
	policyAPIKindLLM policyAPIKind = "llm"
	policyAPIKindMCP policyAPIKind = "mcp"
)

const (
	hubCategoryMCP      = "MCP"
	hubCategorySecurity = "Security"
	// mcpPolicyNamePrefix identifies MCP-only policies the hub doesn't know (custom
	// policies, or every policy while the hub is unreachable).
	mcpPolicyNamePrefix = "mcp-"
)

// policyCatalogEntry is one display-ready policy: the gateway's own definition (the
// source of truth for existence, version and parameter schema) enriched with hub
// metadata matched by name.
type policyCatalogEntry struct {
	gatewayPolicyManifestItem
	Categories []string
	// RequiredSystemConfig lists the gateway config keys the policy reads with no
	// fallback default (see requiredSystemConfigKeys), sorted. Empty means it works
	// without operator setup.
	RequiredSystemConfig []string
}

// systemConfigRefPattern matches a system parameter's gateway config reference, e.g.
// "${config.azurecontentsafety_endpoint}", capturing the key.
var systemConfigRefPattern = regexp.MustCompile(`^\$\{config\.([^}]+)\}$`)

const (
	schemaKeyConfigRef = "wso2/defaultValue"
	schemaKeyDefault   = "default"
)

// buildPolicyCatalog filters the gateway-available policies down to those that apply
// to kind and enriches them from the policy hub. The hub is enrichment only: when it is
// unavailable the listing is still served, with raw names and name-based applicability.
// Entries are sorted by name, then version.
func buildPolicyCatalog(
	ctx context.Context,
	hub policyhub.Client,
	available map[string]gatewayPolicyManifestItem,
	kind policyAPIKind,
	orgUUID string,
) []policyCatalogEntry {
	// A nil hub is treated like a disabled one (no enrichment).
	var hubPolicies map[string]policyhub.Policy
	if hub != nil {
		var err error
		hubPolicies, err = hub.ListPolicies(ctx)
		if err != nil {
			logger.GetLogger(ctx).Warn("buildPolicyCatalog: policy hub unavailable, listing gateway policies without hub metadata",
				"orgUUID", orgUUID, "kind", kind, "error", err)
			hubPolicies = nil
		}
	}

	entries := make([]policyCatalogEntry, 0, len(available))
	for _, item := range sortedGatewayPolicyManifestItems(available) {
		hubPolicy, inHub := hubPolicies[item.Name]
		if !policyAppliesTo(kind, item.Name, hubPolicy.Categories, inHub) {
			continue
		}
		// The gateway's own metadata wins; the hub fills what the manifest left blank.
		if item.DisplayName == "" {
			item.DisplayName = hubPolicy.DisplayName
		}
		if item.DisplayName == "" {
			item.DisplayName = item.Name
		}
		if item.Description == "" {
			item.Description = hubPolicy.Description
		}
		entries = append(entries, policyCatalogEntry{
			gatewayPolicyManifestItem: item,
			Categories:                slices.Clone(hubPolicy.Categories),
			RequiredSystemConfig:      requiredSystemConfigKeys(item.SystemParameters),
		})
	}
	return entries
}

// policyAppliesTo reports whether a policy belongs in kind's listing.
//
// When the hub knows the policy, its categories decide: the MCP listing takes policies
// tagged MCP; the LLM listing takes everything except MCP-exclusive policies. When the
// hub doesn't know it, the name decides: "mcp-" policies are MCP-only, and anything
// else (e.g. a custom policy) appears in both listings.
func policyAppliesTo(kind policyAPIKind, name string, hubCategories []string, inHub bool) bool {
	if !inHub {
		if kind == policyAPIKindLLM {
			return !strings.HasPrefix(name, mcpPolicyNamePrefix)
		}
		return true
	}
	if kind == policyAPIKindMCP {
		return slices.Contains(hubCategories, hubCategoryMCP)
	}
	return !isMCPExclusive(hubCategories)
}

// isMCPExclusive reports whether hub categories mark a policy as MCP-only: tagged MCP,
// with Security as the only other tag (mcp-auth, mcp-ratelimit, ...). A policy that is
// also tagged AI, Logging, Transformation, etc. (cors, log-message, set-headers) is
// general-purpose and stays in the LLM listing.
func isMCPExclusive(hubCategories []string) bool {
	if !slices.Contains(hubCategories, hubCategoryMCP) {
		return false
	}
	for _, category := range hubCategories {
		if category != hubCategoryMCP && category != hubCategorySecurity {
			return false
		}
	}
	return true
}

// requiredSystemConfigKeys returns the gateway config keys a policy's system parameters
// read with nothing to fall back on, sorted and de-duplicated. The gateway resolves each
// system parameter from a "${config.<key>}" reference in its schema; a parameter needs
// operator setup when it has such a reference, no literal default, and must be
// non-empty (listed as required, or minLength >= 1). Nested object schemas are walked
// with their own required lists.
//
// It cannot tell whether the keys are set on a given gateway (the manifest does not
// report that), only that the policy depends on them. It also cannot express
// alternatives (e.g. access keys OR a role), so it may list keys an operator
// legitimately leaves unset.
func requiredSystemConfigKeys(systemParameters map[string]interface{}) []string {
	keys := map[string]struct{}{}
	collectRequiredSystemConfigKeys(systemParameters, keys)
	if len(keys) == 0 {
		return nil
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func collectRequiredSystemConfigKeys(schema map[string]interface{}, keys map[string]struct{}) {
	properties, _ := schema["properties"].(map[string]interface{})
	required := map[string]bool{}
	if list, ok := schema["required"].([]interface{}); ok {
		for _, name := range list {
			if s, ok := name.(string); ok {
				required[s] = true
			}
		}
	}
	for name, raw := range properties {
		property, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if ref, ok := property[schemaKeyConfigRef].(string); ok {
			match := systemConfigRefPattern.FindStringSubmatch(strings.TrimSpace(ref))
			_, hasDefault := property[schemaKeyDefault]
			if match != nil && !hasDefault && (required[name] || minLength(property) >= 1) {
				keys[match[1]] = struct{}{}
			}
		}
		if property["type"] == "object" {
			collectRequiredSystemConfigKeys(property, keys)
		}
	}
}

// minLength reads a JSON-Schema minLength, which decodes from JSON as float64.
func minLength(property map[string]interface{}) float64 {
	switch v := property["minLength"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}
