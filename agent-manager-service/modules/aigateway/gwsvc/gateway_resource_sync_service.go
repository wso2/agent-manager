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

package gwsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// Failure reasons a gateway resource push reports per resource.
const (
	GatewayResourceErrInvalid       = "INVALID_RESOURCE"
	GatewayResourceErrUnsupported   = "UNSUPPORTED_KIND"
	GatewayResourceErrDuplicateName = "DUPLICATE_NAME"
	// BudgetErrAlreadyExists: the name is taken by another artifact: a control-plane
	// budget, another gateway's, or another kind.
	BudgetErrAlreadyExists = "BUDGET_ALREADY_EXISTS"
	// GatewayResourceErrReferenceNotFound: the project-id annotation names a project the
	// organization does not have.
	GatewayResourceErrReferenceNotFound = "REFERENCE_NOT_FOUND"
)

// maxResourceNameLength matches the gateway's limit on metadata.name.
const maxResourceNameLength = 253

// GatewayResourceKindSyncer stores the resources of one kind that a gateway pushes,
// matched by name within that gateway.
type GatewayResourceKindSyncer interface {
	Kind() string
	// Save stores the resource and returns the control plane's ID for it, or a failure
	// reason when it cannot be stored.
	Save(ctx context.Context, gateway *gwmodels.Gateway, name string, item gwmodels.GatewayResourceItem) (id, failure string, err error)
	// Delete removes the gateway's resources with the given names.
	Delete(ctx context.Context, gateway *gwmodels.Gateway, names []string) error
	// Keep removes the gateway's resources whose name is not in names.
	Keep(ctx context.Context, gateway *gwmodels.Gateway, names []string) error
}

// GatewayResourceSyncService receives a gateway's resource push and hands each resource
// to the syncer of its kind.
type GatewayResourceSyncService struct {
	gatewayRepo gwrepo.GatewayRepository
	kinds       map[string]GatewayResourceKindSyncer
}

// NewGatewayResourceSyncService creates the service with the supported kinds: budgets.
func NewGatewayResourceSyncService(
	gatewayRepo gwrepo.GatewayRepository,
	budgetRepo gwrepo.BudgetRepository,
	platform gwrepo.PlatformRepository,
) *GatewayResourceSyncService {
	s := &GatewayResourceSyncService{gatewayRepo: gatewayRepo, kinds: map[string]GatewayResourceKindSyncer{}}
	for _, k := range []GatewayResourceKindSyncer{newGatewayBudgetSyncer(budgetRepo, platform)} {
		s.kinds[k.Kind()] = k
	}
	return s
}

// resourceDocument is the part of a gateway resource document the push reads.
type resourceDocument struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

// pushedResource is a pushed item with its kind and name read and checked.
type pushedResource struct {
	item       gwmodels.GatewayResourceItem
	kind, name string
}

// Sync applies a gateway's resource push: deletions first, then the created or updated
// resources, then — for a complete push — removal of the gateway's resources not listed.
// Each resource is processed on its own and its outcome reported against its dpid.
// ouID is the organization of the gateway's credential; a gateway of another
// organization is reported as not found.
func (s *GatewayResourceSyncService) Sync(ctx context.Context, gatewayID, ouID string,
	req gwmodels.GatewayResourcesRequest,
) (*gwmodels.GatewayResourcesResponse, error) {
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to load gateway %s: %w", gatewayID, err)
	}
	if gateway == nil || gateway.OUID != ouID {
		return nil, gwerr.ErrGatewayNotFound
	}

	resp := &gwmodels.GatewayResourcesResponse{
		Total:     len(req.Resources) + len(req.Deleted),
		Resources: map[string]gwmodels.GatewayResourceResult{},
	}
	fail := func(dpid, reason string) {
		resp.Resources[dpid] = gwmodels.GatewayResourceResult{Status: gwmodels.GatewayResourceStatusFailed, Error: reason}
		resp.Failed++
	}
	synced := func(dpid, id string) {
		resp.Resources[dpid] = gwmodels.GatewayResourceResult{ID: id, Status: gwmodels.GatewayResourceStatusSynced}
		resp.Success++
	}

	// Deletions, grouped by kind.
	deleted := map[string][]pushedResource{}
	for _, r := range s.read(req.Deleted, fail) {
		deleted[r.kind] = append(deleted[r.kind], r)
	}
	for _, kind := range sortedKinds(deleted) {
		names := make([]string, 0, len(deleted[kind]))
		for _, r := range deleted[kind] {
			names = append(names, r.name)
		}
		if err := s.kinds[kind].Delete(ctx, gateway, names); err != nil {
			return nil, fmt.Errorf("failed to delete %s resources of gateway %s: %w", kind, gatewayID, err)
		}
		for _, r := range deleted[kind] {
			synced(r.item.DPID, "")
		}
	}

	// Created or updated resources. A name listed twice for a kind is refused.
	listed := map[string][]string{}
	seen := map[string]bool{}
	for _, r := range s.read(req.Resources, fail) {
		if seen[r.kind+"/"+r.name] {
			fail(r.item.DPID, GatewayResourceErrDuplicateName)
			continue
		}
		seen[r.kind+"/"+r.name] = true
		listed[r.kind] = append(listed[r.kind], r.name)
		id, failure, err := s.kinds[r.kind].Save(ctx, gateway, r.name, r.item)
		if err != nil {
			return nil, fmt.Errorf("failed to store %s %q of gateway %s: %w", r.kind, r.name, gatewayID, err)
		}
		if failure != "" {
			fail(r.item.DPID, failure)
			continue
		}
		synced(r.item.DPID, id)
	}

	// A complete push lists everything the gateway created: remove the rest.
	if req.Complete {
		for kind, syncer := range s.kinds {
			if err := syncer.Keep(ctx, gateway, listed[kind]); err != nil {
				return nil, fmt.Errorf("failed to remove stale %s resources of gateway %s: %w", kind, gatewayID, err)
			}
		}
	}
	return resp, nil
}

// read checks each item's kind and name, reporting the items that fail.
func (s *GatewayResourceSyncService) read(items []gwmodels.GatewayResourceItem, fail func(dpid, reason string)) []pushedResource {
	out := make([]pushedResource, 0, len(items))
	for _, item := range items {
		var doc resourceDocument
		if item.DPID == "" || json.Unmarshal(item.Configuration, &doc) != nil {
			fail(item.DPID, GatewayResourceErrInvalid)
			continue
		}
		if _, ok := s.kinds[doc.Kind]; !ok {
			fail(item.DPID, GatewayResourceErrUnsupported)
			continue
		}
		name := strings.TrimSpace(doc.Metadata.Name)
		if name == "" || len(name) > maxResourceNameLength {
			fail(item.DPID, GatewayResourceErrInvalid)
			continue
		}
		out = append(out, pushedResource{item: item, kind: doc.Kind, name: name})
	}
	return out
}

func sortedKinds[T any](m map[string]T) []string {
	kinds := make([]string, 0, len(m))
	for kind := range m {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// gatewayBudgetSyncer stores the budgets a gateway created through its own API as
// gateway-origin budgets owned by that gateway.
type gatewayBudgetSyncer struct {
	repo     gwrepo.BudgetRepository
	platform gwrepo.PlatformRepository
}

func newGatewayBudgetSyncer(repo gwrepo.BudgetRepository, platform gwrepo.PlatformRepository) *gatewayBudgetSyncer {
	return &gatewayBudgetSyncer{repo: repo, platform: platform}
}

func (b *gatewayBudgetSyncer) Kind() string { return gwmodels.GatewayResourceKindBudget }

func (b *gatewayBudgetSyncer) Save(ctx context.Context, gateway *gwmodels.Gateway, name string,
	item gwmodels.GatewayResourceItem,
) (string, string, error) {
	// Same names as budgets created here (at most 63 characters; lowercase letters, digits
	// and hyphens), so every budget can be opened, edited and imported by name here.
	if !budgetNamePattern.MatchString(name) {
		return "", GatewayResourceErrInvalid, nil
	}
	cfg, project, err := gatewayBudgetConfig(name, item.Configuration)
	if err != nil {
		return "", GatewayResourceErrInvalid, nil //nolint:nilerr // a bad document fails the item, not the push
	}
	// The org-id annotation, when sent, must name the gateway's own organization; left
	// out, the budget is the gateway's organization's.
	if org := gatewayBudgetOrg(item.Configuration); org != "" {
		owner, err := b.platform.GetOrganization(ctx, gateway.OUID)
		if err != nil {
			return "", "", fmt.Errorf("failed to get organization: %w", err)
		}
		if org != owner.Name {
			return "", GatewayResourceErrInvalid, nil
		}
	}
	// The project-id annotation makes it a project budget, as the artifact import reads it
	// for project-scoped kinds (Interfaces 4.3).
	var projectID *uuid.UUID
	if project != "" {
		projectID, err = budgetProjectID(ctx, b.platform, gateway.OUID, project, "")
		var refused *BudgetError
		if errors.As(err, &refused) {
			return "", GatewayResourceErrReferenceNotFound, nil
		}
		if err != nil {
			return "", "", err
		}
		if ownByProject(&cfg, project) != nil {
			return "", GatewayResourceErrInvalid, nil //nolint:nilerr // a budget outside its project fails the item, not the push
		}
	}
	gatewayID := gateway.UUID
	budget := &gwmodels.Budget{
		OrgID: gateway.OUID, Origin: gwmodels.BudgetOriginGateway, GatewayID: &gatewayID, ProjectID: projectID,
		ProjectName: project, Configuration: cfg,
	}
	stored, err := b.repo.SaveForGateway(ctx, budget, name)
	if err != nil {
		return "", "", err
	}
	if !stored {
		return "", BudgetErrAlreadyExists, nil
	}
	return budget.ID.String(), "", nil
}

func (b *gatewayBudgetSyncer) Delete(ctx context.Context, gateway *gwmodels.Gateway, names []string) error {
	return b.repo.DeleteForGateway(ctx, gateway.OUID, gateway.UUID, names)
}

func (b *gatewayBudgetSyncer) Keep(ctx context.Context, gateway *gwmodels.Gateway, names []string) error {
	return b.repo.DeleteForGatewayExcept(ctx, gateway.OUID, gateway.UUID, names)
}

// gatewayBudgetOrg is the organization a pushed Budget kind's org-id annotation names,
// or empty when it has none.
func gatewayBudgetOrg(raw json.RawMessage) string {
	var doc struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.Metadata.Annotations[AnnotationOrgID])
}

// gatewayBudgetConfig maps a pushed Budget kind onto the stored model: its spec, with the
// defaults the REST API fills in, and the project its project-id annotation names (empty
// for an organization budget). A gateway budget has no display name, so its name is shown
// (Interfaces 1.1).
func gatewayBudgetConfig(name string, raw json.RawMessage) (gwmodels.BudgetConfig, string, error) {
	var doc struct {
		Metadata struct {
			Labels      []string          `json:"labels"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec gwmodels.BudgetConfig `json:"spec"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return gwmodels.BudgetConfig{}, "", err
	}
	project := strings.TrimSpace(doc.Metadata.Annotations[AnnotationProjectID])
	cfg := doc.Spec
	if len(doc.Metadata.Labels) > 0 {
		cfg.Labels = doc.Metadata.Labels
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = name
	}
	if cfg.Status == "" {
		cfg.Status = budgetStatusActive
	}
	if cfg.Limit.Currency == "" {
		cfg.Limit.Currency = budgetCurrencyUSD
	}
	if cfg.Filters == nil {
		cfg.Filters = []gwmodels.BudgetFilter{}
	}
	if cfg.Exemptions == nil {
		cfg.Exemptions = []gwmodels.BudgetExemption{}
	}
	return cfg, project, nil
}
