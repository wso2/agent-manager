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
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// llmPolicyManifestItem is one policy definition reported by a gateway's pushed
// manifest. LLM guardrail and MCP policy listings both read manifests through this
// path, merged with the policy hub by buildCatalogResponse.
type llmPolicyManifestItem struct {
	Name             string
	Version          string
	DisplayName      string
	Description      string
	ManagedBy        string
	Parameters       map[string]interface{}
	SystemParameters map[string]interface{}
}

// listCatalogGateways is the gateways whose reported policies make up an org-wide policy catalog: the active managed
// ones. A self-hosted gateway runs whatever bundle its operator installed; one older bundle would narrow the catalog
// for the whole org.
func listCatalogGateways(gatewayRepo repositories.GatewayRepository, orgUUID string) ([]*models.Gateway, error) {
	active := true
	gateways, err := gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: orgUUID,
		Status:         &active,
		Hosting:        models.GatewayHostingManaged,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list active gateways: %w", err)
	}
	return gateways, nil
}

// resolveCatalogGateways returns the gateways a policy catalog is checked against. When the artifacts are placed on at
// least one gateway of the org, those gateways (of any hosting) are returned with the deployed scope. Otherwise (no
// artifacts, no deployment repository, or none placed) the org's active managed gateways are returned with the
// org_managed scope. A resource deployed through several artifacts, as an MCP proxy is, passes all of them.
func resolveCatalogGateways(ctx context.Context, gatewayRepo repositories.GatewayRepository, deploymentRepo repositories.DeploymentRepository, orgUUID string, artifactUUIDs []uuid.UUID) ([]*models.Gateway, models.PolicyAvailabilityScope, error) {
	if gatewayRepo == nil {
		return []*models.Gateway{}, models.PolicyAvailabilityScopeOrgManaged, nil
	}
	if deploymentRepo != nil && len(artifactUUIDs) > 0 {
		var gatewayUUIDs []string
		seen := map[string]struct{}{}
		for _, artifactUUID := range artifactUUIDs {
			placed, err := deploymentRepo.PlacedGateways(ctx, orgUUID, artifactUUID)
			if err != nil {
				return nil, "", fmt.Errorf("failed to list deployed gateways: %w", err)
			}
			for _, id := range placed {
				if _, dup := seen[id]; !dup {
					seen[id] = struct{}{}
					gatewayUUIDs = append(gatewayUUIDs, id)
				}
			}
		}

		gateways := make([]*models.Gateway, 0, len(gatewayUUIDs))
		for _, gatewayUUID := range gatewayUUIDs {
			gateway, err := gatewayRepo.GetByUUID(gatewayUUID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// A deployment row can outlive the gateway it points to (e.g. the
					// gateway was since deleted); skip it rather than failing the whole
					// listing over one stale reference.
					continue
				}
				return nil, "", fmt.Errorf("failed to get deployed gateway %s: %w", gatewayUUID, err)
			}
			// Defense in depth: GetByUUID isn't org-scoped, so verify the gateway we
			// fetched actually belongs to the caller's org before including its
			// policies, even though gatewayUUIDs itself was already org-filtered.
			if gateway != nil && gateway.OUID == orgUUID {
				gateways = append(gateways, gateway)
			}
		}
		if len(gateways) > 0 {
			return gateways, models.PolicyAvailabilityScopeDeployed, nil
		}
	}

	gateways, err := listCatalogGateways(gatewayRepo, orgUUID)
	if err != nil {
		return nil, "", err
	}
	return gateways, models.PolicyAvailabilityScopeOrgManaged, nil
}

// gatewayRef identifies a gateway in catalog results.
type gatewayRef = models.PolicyCatalogGatewayRef

// installedPolicies is the per-gateway presence of policies across a set of gateways.
type installedPolicies struct {
	// ByName is the available policy per name (see intersectLLMPolicies).
	ByName map[string]llmPolicyManifestItem
	// Custom is true for names that any reporting gateway marks managedBy "organization".
	Custom map[string]bool
	// MissingOn lists, for names seen on at least one but not all reporting gateways, the gateways lacking them.
	MissingOn map[string][]gatewayRef
	// Reporting lists the gateways that have reported a manifest.
	Reporting []gatewayRef
}

// collectInstalledPolicies computes which policies are installed, custom or missing on which of the given gateways.
// A gateway that has not reported a manifest is left out of every result, as in intersectLLMPolicies.
func collectInstalledPolicies(gateways []*models.Gateway) installedPolicies {
	result := installedPolicies{
		ByName:    map[string]llmPolicyManifestItem{},
		Custom:    map[string]bool{},
		MissingOn: map[string][]gatewayRef{},
		Reporting: []gatewayRef{},
	}
	// Several versions of one name can survive the intersection (every gateway reports both); take the highest, so
	// the result does not depend on map iteration order.
	for _, item := range intersectLLMPolicies(gateways) {
		if current, ok := result.ByName[item.Name]; ok && compareVersions(item.Version, current.Version) <= 0 {
			continue
		}
		result.ByName[item.Name] = item
	}

	type reported struct {
		ref   gatewayRef
		names map[string]struct{}
	}
	var reporting []reported
	allNames := map[string]struct{}{}
	for _, gateway := range gateways {
		manifest, known := gatewayManifest(gateway)
		if !known {
			continue
		}
		name := gateway.DisplayName
		if name == "" {
			name = gateway.Name
		}
		entry := reported{ref: gatewayRef{ID: gateway.UUID.String(), Name: name}, names: map[string]struct{}{}}
		for _, policy := range extractLLMPolicyManifestItems(manifest) {
			entry.names[policy.Name] = struct{}{}
			allNames[policy.Name] = struct{}{}
			if policy.ManagedBy == "organization" {
				result.Custom[policy.Name] = true
			}
		}
		reporting = append(reporting, entry)
		result.Reporting = append(result.Reporting, entry.ref)
	}

	for name := range allNames {
		for _, entry := range reporting {
			if _, ok := entry.names[name]; !ok {
				result.MissingOn[name] = append(result.MissingOn[name], entry.ref)
			}
		}
	}
	return result
}

// intersectLLMPolicies returns, keyed by "name\x00version", the full policy definitions
// available across the given gateways. A policy name+version is "available" if every given
// gateway advertises that exact version (strict intersection). For a policy name where the
// given gateways disagree on version — e.g. a rolling gateway upgrade where some gateways
// still report the old version — policies are assumed backward-compatible: the name falls
// back to the LOWEST version reported by any of the given gateways, using that gateway's
// own metadata, so a previously-applied policy doesn't disappear from the catalog mid-rollout.
// A gateway that has not reported a manifest yet (see gatewayManifest) is left out of the
// intersection rather than emptying it; if none has reported, nothing is available.
func intersectLLMPolicies(gateways []*models.Gateway) map[string]llmPolicyManifestItem {
	var perGatewayPolicies []map[string]llmPolicyManifestItem
	// byName collects every (gateway, version) sighting per policy name, used for the
	// backward-compatible fallback when strict intersection yields nothing for that name.
	byName := map[string][]llmPolicyManifestItem{}
	// gatewayCountByName counts, per policy name, how many distinct gateways reported it
	// (at any version) — the fallback only applies when EVERY gateway has the name, just
	// disagreeing on version. A name genuinely missing from some gateway stays excluded.
	gatewayCountByName := map[string]int{}
	gatewayCount := 0

	for _, gateway := range gateways {
		manifest, known := gatewayManifest(gateway)
		if !known {
			continue
		}
		gatewayCount++
		gatewayPolicies := map[string]llmPolicyManifestItem{}
		namesOnThisGateway := map[string]struct{}{}
		for _, policy := range extractLLMPolicyManifestItems(manifest) {
			if policy.Name == "" || policy.Version == "" {
				continue
			}
			key := policy.Name + "\x00" + policy.Version
			gatewayPolicies[key] = policy
			byName[policy.Name] = append(byName[policy.Name], policy)
			namesOnThisGateway[policy.Name] = struct{}{}
		}
		for name := range namesOnThisGateway {
			gatewayCountByName[name]++
		}
		perGatewayPolicies = append(perGatewayPolicies, gatewayPolicies)
	}

	available := map[string]llmPolicyManifestItem{}
	for i, gatewayPolicies := range perGatewayPolicies {
		if i == 0 {
			maps.Copy(available, gatewayPolicies)
			continue
		}
		for key := range available {
			if _, ok := gatewayPolicies[key]; !ok {
				delete(available, key)
			}
		}
	}

	namesInIntersection := map[string]struct{}{}
	for _, policy := range available {
		namesInIntersection[policy.Name] = struct{}{}
	}

	for name, sightings := range byName {
		if _, ok := namesInIntersection[name]; ok {
			continue
		}
		if gatewayCountByName[name] != gatewayCount {
			// Not on every gateway — genuinely unavailable, not a version mismatch.
			continue
		}
		lowest := lowestVersionLLMPolicy(sightings)
		key := lowest.Name + "\x00" + lowest.Version
		available[key] = lowest
	}

	return available
}

// lowestVersionLLMPolicy returns the sighting with the lowest Version among policies that
// share the same Name, using semver-aware numeric comparison per dot-separated segment and
// falling back to a plain string comparison for any version that doesn't parse as numeric
// segments. sightings must be non-empty.
func lowestVersionLLMPolicy(sightings []llmPolicyManifestItem) llmPolicyManifestItem {
	lowest := sightings[0]
	for _, candidate := range sightings[1:] {
		if compareVersions(candidate.Version, lowest.Version) < 0 {
			lowest = candidate
		}
	}
	return lowest
}

// compareVersions compares two dot-separated version strings segment by segment,
// numerically where possible (so "1.10.0" > "1.9.0", unlike a plain string compare).
// A single optional leading "v"/"V" is stripped from each side first, so "v10" and
// "v2" also compare numerically. If either version has a non-numeric segment, it
// falls back to a plain string compare of the full (original, unstripped) version.
func compareVersions(a, b string) int {
	segmentsA := strings.Split(strings.TrimPrefix(strings.TrimPrefix(a, "v"), "V"), ".")
	segmentsB := strings.Split(strings.TrimPrefix(strings.TrimPrefix(b, "v"), "V"), ".")

	for i := 0; i < len(segmentsA) && i < len(segmentsB); i++ {
		numA, errA := strconv.Atoi(segmentsA[i])
		numB, errB := strconv.Atoi(segmentsB[i])
		if errA != nil || errB != nil {
			return strings.Compare(a, b)
		}
		if numA != numB {
			return numA - numB
		}
	}
	return len(segmentsA) - len(segmentsB)
}

// extractLLMPolicyManifestItems recursively walks a gateway's arbitrarily-shaped,
// self-reported manifest JSON and pulls out every policy definition it can find,
// including the metadata (displayName/description/parameters/systemParameters) the
// LLM guardrail picker needs to render without a policy-hub round-trip. It tolerates
// a few different key names ("name"/"policyName"/"id", "version"/"policyVersion")
// since the manifest shape is not schema-enforced across gateway versions.
func extractLLMPolicyManifestItems(value interface{}) []llmPolicyManifestItem {
	seen := map[string]struct{}{}
	items := make([]llmPolicyManifestItem, 0)
	var walk func(interface{})

	stringValue := func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}

	mapValue := func(v interface{}) map[string]interface{} {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
		return nil
	}

	lookup := func(values map[string]interface{}, keys ...string) interface{} {
		for _, key := range keys {
			if value, ok := values[key]; ok {
				return value
			}
		}
		return nil
	}

	add := func(entry map[string]interface{}, name, version string) {
		name = strings.TrimSpace(name)
		version = strings.TrimSpace(version)
		if name == "" || version == "" {
			return
		}
		key := name + "\x00" + version
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, llmPolicyManifestItem{
			Name:             name,
			Version:          version,
			DisplayName:      stringValue(lookup(entry, "displayName")),
			Description:      stringValue(lookup(entry, "description")),
			ManagedBy:        stringValue(lookup(entry, "managedBy")),
			Parameters:       mapValue(lookup(entry, "parameters")),
			SystemParameters: mapValue(lookup(entry, "systemParameters")),
		})
	}

	// consumedKeys are a policy's own metadata / JSON-Schema leaves — never
	// containers of OTHER policies. They're skipped during the generic recursive
	// descent so a nested schema object carrying coincidental name+version keys
	// (e.g. a default example {"name":"gpt-4","version":"1.0"} inside a parameter
	// schema) isn't surfaced as a bogus, unrelated policy.
	consumedKeys := map[string]struct{}{
		"parameters":       {},
		"systemParameters": {},
		"displayName":      {},
		"description":      {},
	}

	walk = func(current interface{}) {
		switch typed := current.(type) {
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		case []map[string]interface{}:
			for _, item := range typed {
				walk(item)
			}
		case map[string]interface{}:
			name := stringValue(lookup(typed, "name", "policyName", "id"))
			version := stringValue(lookup(typed, "version", "policyVersion"))
			if name != "" && version != "" {
				add(typed, name, version)
			}
			if name != "" {
				if versions, ok := lookup(typed, "versions", "policyVersions").([]interface{}); ok {
					for _, rawVersion := range versions {
						add(typed, name, stringValue(rawVersion))
					}
				}
				if versions, ok := lookup(typed, "versions", "policyVersions").([]string); ok {
					for _, rawVersion := range versions {
						add(typed, name, rawVersion)
					}
				}
			}
			for key, nested := range typed {
				if _, skip := consumedKeys[key]; skip {
					continue
				}
				walk(nested)
			}
		}
	}

	walk(value)
	return items
}
