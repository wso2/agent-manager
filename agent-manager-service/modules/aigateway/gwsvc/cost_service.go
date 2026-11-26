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
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// AI cost stats: gateways with [ai_cost_management.tracking] on count what every
// LLM request cost and report it in their usage messages (4.2); the totals are kept per
// month and combination of project, agent, API key, user, LLM provider and model, for 3
// months.

// costMonths is how many months are kept and shown; the current one counts as one.
const costMonths = 3

// Cost group and filter types.
const (
	costByOrganization = "organization"
	costByProject      = "project"
	costByAgent        = "agent"
	costByAPIKey       = "apiKey"
	costByUser         = "user"
	costByLLMProvider  = "llmProvider"
	costByModel        = "model"
)

var costGroupTypes = []string{costByOrganization, costByProject, costByAgent, costByAPIKey, costByUser, costByLLMProvider, costByModel}

// GatewayCostUsage is what one combination of values cost in one month, across every
// gateway sharing the counter store.
type GatewayCostUsage struct {
	Month                    string `json:"month"`
	CostKey                  string `json:"costKey"`
	Project                  string `json:"project"`
	Agent                    string `json:"agent"`
	APIKey                   string `json:"apiKey"`
	User                     string `json:"user"`
	LLMProvider              string `json:"llmProvider"`
	Model                    string `json:"model"`
	Cost                     string `json:"cost"`
	MissingPriceRequests     int64  `json:"missingPriceRequests"`
	UsageUnavailableRequests int64  `json:"usageUnavailableRequests"`
}

// CostRecorder stores the cost totals gateways report.
type CostRecorder interface {
	RecordCosts(ctx context.Context, ouID string, costs []GatewayCostUsage) error
}

// CostService keeps and reports AI cost stats.
type CostService interface {
	CostRecorder
	// Costs returns the organization's costs over the kept months, one entry per value of
	// groupBy, limited by every filter (type:value[,value...]). orgName is the organization's
	// handle, the id of its one entry when grouped by organization.
	Costs(ctx context.Context, ouID, orgName, groupBy string, filters []string) (*gwspec.CostReport, error)
}

type costService struct {
	repo gwrepo.CostUsageRepository
	now  func() time.Time

	// keptFrom is, per organization, the first kept month this replica has already
	// deleted the older ones before: the delete runs once when a new month starts, on the
	// organization's first write in it, not on a schedule.
	mu       sync.Mutex
	keptFrom map[string]time.Time
}

// NewCostService creates a CostService.
func NewCostService(repo gwrepo.CostUsageRepository) CostService {
	return &costService{repo: repo, now: time.Now, keptFrom: map[string]time.Time{}}
}

func (s *costService) RecordCosts(ctx context.Context, ouID string, costs []GatewayCostUsage) error {
	rows := make([]gwmodels.CostUsage, 0, len(costs))
	first := costFirstMonth(s.now())
	for _, c := range costs {
		row, err := costRow(ouID, c)
		if err != nil {
			slog.Warn("Ignoring an unreadable cost report", "costKey", c.CostKey, "error", err)
			continue
		}
		if row.Month.Before(first) {
			continue // older than the kept months
		}
		rows = append(rows, row)
	}
	// One transaction per flush; on failure the caller keeps the totals for the next one.
	if err := s.repo.Record(ctx, rows); err != nil {
		slog.Error("Failed to record cost usage", "rows", len(rows), "error", err)
		return err
	}
	s.dropOldMonths(ctx, ouID, first)
	return nil
}

// dropOldMonths deletes the organization's months before first, once per new first
// month: when a month starts, the oldest kept one goes. A failure is tried again with the
// next write.
func (s *costService) dropOldMonths(ctx context.Context, ouID string, first time.Time) {
	s.mu.Lock()
	done := s.keptFrom[ouID].Equal(first)
	s.mu.Unlock()
	if done {
		return
	}
	deleted, err := s.repo.DeleteBefore(ctx, ouID, first)
	if err != nil {
		slog.Error("Failed to delete cost usage older than the kept months", "orgId", ouID, "error", err)
		return
	}
	s.mu.Lock()
	s.keptFrom[ouID] = first
	s.mu.Unlock()
	if deleted > 0 {
		slog.Info("Deleted cost usage older than the kept months", "orgId", ouID, "rows", deleted)
	}
}

// costRow maps a gateway's report onto its row.
func costRow(ouID string, c GatewayCostUsage) (gwmodels.CostUsage, error) {
	month, err := time.Parse(time.DateOnly, c.Month)
	if err != nil {
		return gwmodels.CostUsage{}, fmt.Errorf("month: %w", err)
	}
	key, err := uuid.Parse(c.CostKey)
	if err != nil {
		return gwmodels.CostUsage{}, fmt.Errorf("costKey: %w", err)
	}
	if strings.TrimSpace(c.LLMProvider) == "" {
		return gwmodels.CostUsage{}, fmt.Errorf("llmProvider is empty")
	}
	if _, ok := new(big.Rat).SetString(c.Cost); !ok {
		return gwmodels.CostUsage{}, fmt.Errorf("cost %q is not a decimal", c.Cost)
	}
	optional := func(v string) *string {
		if v = strings.TrimSpace(v); v != "" {
			return &v
		}
		return nil
	}
	return gwmodels.CostUsage{
		OUID: ouID, Month: monthOf(month), CostKey: key,
		Project: optional(c.Project), Agent: optional(c.Agent), APIKey: optional(c.APIKey), UserID: optional(c.User),
		LLMProvider: c.LLMProvider, Model: optional(c.Model), Cost: c.Cost,
		MissingPriceRequests: c.MissingPriceRequests, UsageUnavailableRequests: c.UsageUnavailableRequests,
	}, nil
}

// monthOf is the first of t's UTC month.
func monthOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// costFirstMonth is the oldest kept month: the current one and the ones before it.
func costFirstMonth(now time.Time) time.Time {
	return monthOf(now).AddDate(0, -(costMonths - 1), 0)
}

// costFilter is one filter: rows whose value of its type is any of its values.
type costFilter struct {
	typ    string
	values []string
}

func parseCostFilters(filters []string) ([]costFilter, error) {
	out := make([]costFilter, 0, len(filters))
	for _, f := range filters {
		typ, values, ok := strings.Cut(f, ":")
		if !ok || typ == costByOrganization || !slices.Contains(costGroupTypes, typ) {
			return nil, invalidBudget("filter", "filter must be type:value with type project, agent, apiKey, user, llmProvider or model.")
		}
		var vs []string
		for _, v := range strings.Split(values, ",") {
			if v = strings.TrimSpace(v); v != "" {
				// An agent name is unique only within its project, so it is always project/agent.
				if typ == costByAgent {
					if project, agent, ok := strings.Cut(v, "/"); !ok || project == "" || agent == "" {
						return nil, invalidBudget("filter", "An agent filter value must be project/agent, e.g. agent:api-platform/support-agent; got %q.", v)
					}
				}
				vs = append(vs, v)
			}
		}
		if len(vs) == 0 {
			return nil, invalidBudget("filter", "filter %s names no value.", typ)
		}
		out = append(out, costFilter{typ: typ, values: vs})
	}
	return out, nil
}

// costValue is a row's value of a type; an agent is project/agent.
func costValue(r gwmodels.CostUsage, typ string) string {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	switch typ {
	case costByProject:
		return deref(r.Project)
	case costByAgent:
		if r.Agent == nil {
			return ""
		}
		return deref(r.Project) + "/" + *r.Agent
	case costByAPIKey:
		return deref(r.APIKey)
	case costByUser:
		return deref(r.UserID)
	case costByLLMProvider:
		return r.LLMProvider
	case costByModel:
		return deref(r.Model)
	}
	return ""
}

type costTotals struct {
	// id is the groupBy value (nil when requests carried none); project is an agent's.
	id, project *string
	byMonth     []*big.Rat
	total       *big.Rat
	missing     int64
	unavailable int64
}

func (s *costService) Costs(ctx context.Context, ouID, orgName, groupBy string, filters []string) (*gwspec.CostReport, error) {
	if ouID == "" {
		return nil, gwerr.ErrOrganizationNotFound
	}
	if groupBy == "" {
		groupBy = costByOrganization
	}
	if !slices.Contains(costGroupTypes, groupBy) {
		return nil, invalidBudget("groupBy", "groupBy must be one of %s.", strings.Join(costGroupTypes, ", "))
	}
	matchers, err := parseCostFilters(filters)
	if err != nil {
		return nil, err
	}
	first := costFirstMonth(s.now())
	months := make([]string, costMonths)
	for i := range months {
		months[i] = first.AddDate(0, i, 0).Format("2006-01")
	}
	rows, err := s.repo.Since(ctx, ouID, first)
	if err != nil {
		return nil, err
	}
	groups := map[string]*costTotals{}
	for _, r := range rows {
		if !costMatches(r, matchers) {
			continue
		}
		i := (r.Month.UTC().Year()-first.Year())*12 + int(r.Month.UTC().Month()-first.Month())
		if i < 0 || i >= costMonths {
			continue
		}
		key := ""
		if groupBy != costByOrganization {
			key = costValue(r, groupBy)
		}
		g := groups[key]
		if g == nil {
			id, project := costGroupOf(r, groupBy, orgName)
			g = &costTotals{id: id, project: project, byMonth: make([]*big.Rat, costMonths), total: new(big.Rat)}
			for m := range g.byMonth {
				g.byMonth[m] = new(big.Rat)
			}
			groups[key] = g
		}
		cost, ok := new(big.Rat).SetString(r.Cost)
		if !ok {
			cost = new(big.Rat)
		}
		g.byMonth[i].Add(g.byMonth[i], cost)
		g.total.Add(g.total, cost)
		g.missing += r.MissingPriceRequests
		g.unavailable += r.UsageUnavailableRequests
	}
	ordered := make([]*costTotals, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	// Highest total first; equal totals by value, so the order is stable.
	sort.Slice(ordered, func(i, j int) bool {
		if c := ordered[i].total.Cmp(ordered[j].total); c != 0 {
			return c > 0
		}
		return costGroupName(ordered[i]) < costGroupName(ordered[j])
	})
	out := make([]gwspec.CostByGroup, 0, len(ordered))
	for _, g := range ordered {
		monthly := make([]gwspec.MonthlyCost, costMonths)
		for m, c := range g.byMonth {
			monthly[m] = *gwspec.NewMonthlyCost(months[m], usdString(c))
		}
		entry := gwspec.NewCostByGroup(
			*gwspec.NewNullableString(g.id),
			usdString(g.total),
			*gwspec.NewUnpricedRequests(g.missing, g.unavailable),
			monthly,
		)
		entry.Project = g.project
		out = append(out, *entry)
	}
	return gwspec.NewCostReport(out), nil
}

// costMatches reports whether a row passes every filter.
func costMatches(r gwmodels.CostUsage, filters []costFilter) bool {
	for _, f := range filters {
		if !slices.Contains(f.values, costValue(r, f.typ)) {
			return false
		}
	}
	return true
}

// costGroupOf returns a row's groupBy value (nil when the request carried none) and, for an
// agent, its project; grouped by organization, the value is the organization's handle.
func costGroupOf(r gwmodels.CostUsage, groupBy, orgName string) (id, project *string) {
	copyOf := func(v *string) *string {
		if v == nil {
			return nil
		}
		value := *v
		return &value
	}
	switch groupBy {
	case costByOrganization:
		return &orgName, nil
	case costByProject:
		return copyOf(r.Project), nil
	case costByAgent:
		return copyOf(r.Agent), copyOf(r.Project)
	case costByAPIKey:
		return copyOf(r.APIKey), nil
	case costByUser:
		return copyOf(r.UserID), nil
	case costByLLMProvider:
		return copyOf(&r.LLMProvider), nil
	case costByModel:
		return copyOf(r.Model), nil
	}
	return nil, nil
}

// costGroupName orders equal costs by project then value, so the order is stable.
func costGroupName(g *costTotals) string {
	parts := []string{}
	for _, p := range []*string{g.project, g.id} {
		if p != nil {
			parts = append(parts, *p)
		}
	}
	return strings.Join(parts, "/")
}

// usdString is an amount of USD as the API returns it: exact to six decimals (a request
// often costs a fraction of a cent), at least two, so totals added up by a client match
// the server's however the amounts are grouped. Clients round only for display.
func usdString(r *big.Rat) string {
	s := strings.TrimRight(r.FloatString(6), "0")
	whole, frac, _ := strings.Cut(s, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	return whole + "." + frac
}
