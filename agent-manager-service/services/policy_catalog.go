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
	"sort"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// Hub categories that decide which picker a policy belongs to. Matched exactly as the hub sends them.
const (
	hubCategoryMCP        = "MCP"
	hubCategoryAI         = "AI"
	hubCategoryGuardrails = "Guardrails"
)

// hubPolicyMatchesKind reports whether a hub policy's categories put it in the given catalog kind.
func hubPolicyMatchesKind(kind models.PolicyCatalogKind, categories []string) bool {
	for _, c := range categories {
		switch kind {
		case models.PolicyCatalogKindMCP:
			if c == hubCategoryMCP {
				return true
			}
		case models.PolicyCatalogKindLLM:
			if c == hubCategoryAI || c == hubCategoryGuardrails {
				return true
			}
		}
	}
	return false
}

// hubPolicyByName keeps one hub entry per name: the one flagged latest, else the highest version.
func hubPolicyByName(hub []HubPolicy) map[string]HubPolicy {
	byName := make(map[string]HubPolicy, len(hub))
	for _, p := range hub {
		cur, ok := byName[p.Name]
		if !ok || hubPolicyPreferred(p, cur) {
			byName[p.Name] = p
		}
	}
	return byName
}

// hubPolicyPreferred reports whether candidate should replace current.
func hubPolicyPreferred(candidate, current HubPolicy) bool {
	if candidate.IsLatest != current.IsLatest {
		return candidate.IsLatest
	}
	return compareVersions(candidate.Version, current.Version) > 0
}

// firstNonEmpty returns the first non-empty value, or "".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return append([]string{}, s...)
}

// buildPolicyCatalog merges installed gateway policies with the policy hub for one picker kind.
// Installed policies come first, then hub policies missing from the gateways; both sorted by name.
//
// Which kind a policy belongs to: a custom policy (managedBy organization) is listed in both kinds, whatever the hub
// says about its name. A non-custom policy takes its kind from the hub's categories; when the hub is unavailable or
// does not know the name, it is listed in the LLM kind only, so the MCP catalog never shows more than hub policies
// the gateways carry. A non-custom policy on only some gateways that the hub does not know is listed (LLM only) as
// a disabled row.
func buildPolicyCatalog(kind models.PolicyCatalogKind, installed installedPolicies,
	hub []HubPolicy, hubOK bool,
) []models.PolicyCatalogItem {
	var hubByName map[string]HubPolicy
	if hubOK {
		hubByName = hubPolicyByName(hub)
	}
	items := []models.PolicyCatalogItem{}

	// Installed names: custom policies are in both kinds. Others take their kind from the hub when it knows the
	// name, and are LLM-only when it does not.
	for name, m := range installed.ByName {
		h, known := hubByName[name]
		custom := installed.Custom[name]
		if !custom {
			if known && !hubPolicyMatchesKind(kind, h.Categories) {
				continue
			}
			if !known && kind != models.PolicyCatalogKindLLM {
				continue
			}
		}
		source := models.PolicyCatalogSourceHub
		if custom {
			source = models.PolicyCatalogSourceCustom
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = h.DisplayName
		}
		if displayName == "" {
			displayName = name
		}
		items = append(items, models.PolicyCatalogItem{
			Name:             name,
			Version:          m.Version,
			DisplayName:      displayName,
			Description:      firstNonEmpty(m.Description, h.Description),
			Source:           source,
			Status:           models.PolicyCatalogStatusInstalled,
			Categories:       nonNilStrings(h.Categories),
			Parameters:       m.Parameters,
			SystemParameters: m.SystemParameters,
		})
	}

	// Custom names seen on only some gateways: both kinds. The row shows the definition those gateways reported,
	// falling back to the hub and then the bare name; it carries no settings form because it cannot be selected.
	customMissing := map[string]bool{}
	for name, missing := range installed.MissingOn {
		if !installed.Custom[name] {
			continue
		}
		if _, isInstalled := installed.ByName[name]; isInstalled {
			continue
		}
		customMissing[name] = true
		h := hubByName[name]
		s := installed.Sighted[name]
		items = append(items, models.PolicyCatalogItem{
			Name:        name,
			Version:     firstNonEmpty(s.Version, h.Version),
			DisplayName: firstNonEmpty(s.DisplayName, h.DisplayName, name),
			Description: firstNonEmpty(s.Description, h.Description),
			Source:      models.PolicyCatalogSourceCustom,
			Status:      models.PolicyCatalogStatusNotInstalled,
			MissingOn:   append([]gatewayRef(nil), missing...),
			Categories:  nonNilStrings(h.Categories),
		})
	}

	// Non-custom names on only some gateways that the hub does not know: no hub row will cover them, so show a
	// disabled row from the definition the gateways reported. LLM only (see above).
	if kind == models.PolicyCatalogKindLLM {
		for name, missing := range installed.MissingOn {
			if installed.Custom[name] {
				continue
			}
			if _, isInstalled := installed.ByName[name]; isInstalled {
				continue
			}
			if _, known := hubByName[name]; known {
				continue
			}
			s := installed.Sighted[name]
			items = append(items, models.PolicyCatalogItem{
				Name:        name,
				Version:     s.Version,
				DisplayName: firstNonEmpty(s.DisplayName, name),
				Description: s.Description,
				Source:      models.PolicyCatalogSourceHub,
				Status:      models.PolicyCatalogStatusNotInstalled,
				MissingOn:   append([]gatewayRef(nil), missing...),
				Categories:  []string{},
			})
		}
	}

	// Hub names not installed anywhere reporting (or on only some gateways).
	for name, h := range hubByName {
		if _, isInstalled := installed.ByName[name]; isInstalled || customMissing[name] {
			continue
		}
		if !hubPolicyMatchesKind(kind, h.Categories) {
			continue
		}
		// MissingOn is set only for a name on some, not all, gateways; on none it stays empty.
		missing := installed.MissingOn[name]
		displayName := h.DisplayName
		if displayName == "" {
			displayName = name
		}
		items = append(items, models.PolicyCatalogItem{
			Name:        name,
			Version:     h.Version,
			DisplayName: displayName,
			Description: h.Description,
			Source:      models.PolicyCatalogSourceHub,
			Status:      models.PolicyCatalogStatusNotInstalled,
			MissingOn:   append([]gatewayRef(nil), missing...),
			Categories:  nonNilStrings(h.Categories),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		iInstalled := items[i].Status == models.PolicyCatalogStatusInstalled
		jInstalled := items[j].Status == models.PolicyCatalogStatusInstalled
		if iInstalled != jInstalled {
			return iInstalled
		}
		return items[i].Name < items[j].Name
	})
	return items
}

// buildCatalogResponse resolves the gateways for the artifacts, reads what they have installed and merges it with
// the policy hub snapshot. A nil hub counts as unavailable.
func buildCatalogResponse(ctx context.Context, kind models.PolicyCatalogKind, gatewayRepo repositories.GatewayRepository,
	deploymentRepo repositories.DeploymentRepository, hub PolicyHubClient, orgUUID string, artifactUUIDs []uuid.UUID,
) (*models.PolicyCatalogResponse, error) {
	gateways, scope, err := resolveCatalogGateways(ctx, gatewayRepo, deploymentRepo, orgUUID, artifactUUIDs)
	if err != nil {
		return nil, err
	}
	var hubPolicies []HubPolicy
	hubOK := false
	if hub != nil {
		hubPolicies, hubOK = hub.Snapshot(ctx)
	}
	items := buildPolicyCatalog(kind, collectInstalledPolicies(gateways), hubPolicies, hubOK)
	return &models.PolicyCatalogResponse{
		Count:             int32(len(items)),
		List:              items,
		HubAvailable:      hubOK,
		AvailabilityScope: scope,
	}, nil
}
