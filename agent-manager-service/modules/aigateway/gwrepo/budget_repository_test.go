//go:build integration

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

package gwrepo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// gatewayBudget builds a budget a gateway pushes.
func gatewayBudget(ouID string, gatewayID uuid.UUID, name, amount string) gwmodels.Budget {
	return gwmodels.Budget{
		OrgID: ouID, Origin: gwmodels.BudgetOriginGateway, GatewayID: &gatewayID,
		Configuration: budgetConfig(name, amount),
	}
}

func budgetConfig(displayName, amount string) gwmodels.BudgetConfig {
	return gwmodels.BudgetConfig{
		DisplayName: displayName, Status: "active",
		Limit:   gwmodels.BudgetLimit{Amount: amount, Currency: "USD", Period: "month", Action: "allow"},
		Filters: []gwmodels.BudgetFilter{}, Exemptions: []gwmodels.BudgetExemption{},
	}
}

// coreOwners reads the core's organizations and projects, as the core API does.
type coreOwners struct{ db *gorm.DB }

func (o coreOwners) GetOrg(ctx context.Context, orgID string) (modkit.Org, error) {
	var org modkit.Org
	err := o.db.WithContext(ctx).Table("organizations").Select("id, handle").Where("id = ?", orgID).Take(&org).Error
	return org, err
}

func (o coreOwners) ListProjects(ctx context.Context, orgID string) ([]modkit.Project, error) {
	var out []modkit.Project
	err := o.db.WithContext(ctx).Table("projects").Select("id, name").Where("org_id = ?", orgID).Scan(&out).Error
	return out, err
}

func newBudgetRepo(gdb *gorm.DB) BudgetRepository { return NewBudgetRepository(gdb, coreOwners{gdb}) }

// createBudget stores a control-plane budget named name and returns its ID.
func createBudget(t *testing.T, gdb *gorm.DB, ouID, name string) uuid.UUID {
	t.Helper()
	b := gwmodels.Budget{OrgID: ouID, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig(name, "1000")}
	require.NoError(t, newBudgetRepo(gdb).CreateForControlPlane(context.Background(), &b, name))
	return b.ID
}

func budgetsByName(t *testing.T, repo BudgetRepository, ouID string) map[string]gwmodels.Budget {
	t.Helper()
	rows, err := repo.List(context.Background(), ouID)
	require.NoError(t, err)
	out := map[string]gwmodels.Budget{}
	for _, r := range rows {
		out[r.Name] = r
	}
	return out
}

func keys(m map[string]gwmodels.Budget) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestBudgetRepository_GatewayBudgets(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	gw := newGateway(t, ou, gwmodels.GatewayRoleEgress)
	other := newGateway(t, ou, gwmodels.GatewayRoleEgress)
	repo := newBudgetRepo(gdb)
	save := func(b gwmodels.Budget, name string) (gwmodels.Budget, bool) {
		t.Helper()
		stored, err := repo.SaveForGateway(ctx, &b, name)
		require.NoError(t, err)
		return b, stored
	}

	// A control-plane budget and another gateway's budget.
	createBudget(t, gdb, ou, "org-wide")
	_, stored := save(gatewayBudget(ou, other, "team-z", "5"), "team-z")
	require.True(t, stored)

	a, stored := save(gatewayBudget(ou, gw, "team-a", "100"), "team-a")
	require.True(t, stored)
	_, stored = save(gatewayBudget(ou, gw, "team-b", "200"), "team-b")
	require.True(t, stored)
	for _, taken := range []string{"org-wide", "team-z"} {
		_, stored = save(gatewayBudget(ou, gw, taken, "1"), taken)
		assert.False(t, stored, "%s is another budget's name", taken)
	}
	all := budgetsByName(t, repo, ou)
	assert.ElementsMatch(t, []string{"org-wide", "team-z", "team-a", "team-b"}, keys(all))
	assert.Equal(t, gwmodels.BudgetOriginControlPlane, all["org-wide"].Origin)
	assert.Equal(t, "1000", all["org-wide"].Configuration.Limit.Amount)
	assert.Equal(t, other, *all["team-z"].GatewayID)

	// Agent Manager's fields of a gateway budget survive the gateway's next push.
	named, err := repo.Get(ctx, ou, "team-a")
	require.NoError(t, err)
	named.Configuration.DisplayName = "Team A, monthly"
	named.Configuration.Warnings = &gwmodels.BudgetWarnings{Thresholds: []int32{80}, Channels: []string{"finance-email"}}
	require.NoError(t, repo.Update(ctx, named))

	// A later push of team-a updates the stored budget and keeps its ID.
	again, stored := save(gatewayBudget(ou, gw, "team-a", "150"), "team-a")
	require.True(t, stored)
	assert.Equal(t, a.ID, again.ID, "the control plane's ID is kept across pushes")
	pushed := budgetsByName(t, repo, ou)["team-a"]
	assert.Equal(t, "150", pushed.Configuration.Limit.Amount)
	assert.Equal(t, "Team A, monthly", pushed.Configuration.DisplayName, "Agent Manager's display name is kept")
	require.NotNil(t, pushed.Configuration.Warnings, "Agent Manager's warnings are kept")

	// A deletion removes only that gateway's budget of the name.
	require.NoError(t, repo.DeleteForGateway(ctx, ou, other, []string{"team-a"}))
	require.NoError(t, repo.DeleteForGateway(ctx, ou, gw, []string{"team-b", "unknown", "org-wide"}))
	assert.ElementsMatch(t, []string{"org-wide", "team-z", "team-a"}, keys(budgetsByName(t, repo, ou)))

	// A complete list removes the gateway's budgets it does not name, and nothing else.
	_, _ = save(gatewayBudget(ou, gw, "team-c", "300"), "team-c")
	require.NoError(t, repo.DeleteForGatewayExcept(ctx, ou, gw, []string{"team-c"}))
	assert.ElementsMatch(t, []string{"org-wide", "team-z", "team-c"}, keys(budgetsByName(t, repo, ou)))

	// Deleting a gateway deletes the budgets it pushed.
	require.NoError(t, NewGatewayRepo(gdb).Delete(gw.String(), ou))
	assert.ElementsMatch(t, []string{"org-wide", "team-z"}, keys(budgetsByName(t, repo, ou)))
}

func TestBudgetRepository_CreateForControlPlane(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	repo := newBudgetRepo(gdb)

	b := gwmodels.Budget{
		OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, CreatedBy: "alice",
		Configuration: budgetConfig("Org monthly", "5000"),
	}
	require.NoError(t, repo.CreateForControlPlane(ctx, &b, "org-monthly"))
	assert.NotEqual(t, uuid.Nil, b.ID)
	assert.Equal(t, "org-monthly", b.Name)
	assert.False(t, b.CreatedAt.IsZero())

	stored := budgetsByName(t, repo, ou)["org-monthly"]
	assert.Equal(t, b.ID, stored.ID)
	assert.Equal(t, "alice", stored.CreatedBy)
	assert.Nil(t, stored.GatewayID)
	assert.False(t, stored.CreatedAt.IsZero())
	assert.Equal(t, b.Configuration, stored.Configuration)

	again := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig("x", "1")}
	assert.ErrorIs(t, repo.CreateForControlPlane(ctx, &again, "org-monthly"), gwerr.ErrBudgetAlreadyExists)
	assert.Len(t, budgetsByName(t, repo, ou), 1)
}

func TestBudgetRepository_GetUpdateDelete(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	otherOrg := dbtest.Org(t, gdb)
	repo := newBudgetRepo(gdb)

	b := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig("Team A", "100")}
	require.NoError(t, repo.CreateForControlPlane(ctx, &b, "team-a"))

	got, err := repo.Get(ctx, ou, "team-a")
	require.NoError(t, err)
	assert.Equal(t, b.ID, got.ID)
	assert.Equal(t, "team-a", got.Name)
	assert.Equal(t, "100", got.Configuration.Limit.Amount)
	for _, missing := range []struct{ org, name string }{{ou, "nope"}, {otherOrg, "team-a"}} {
		_, err = repo.Get(ctx, missing.org, missing.name)
		assert.ErrorIs(t, err, gwerr.ErrBudgetNotFound, missing.name)
	}

	got.Configuration.Limit.Amount = "150"
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.Update(ctx, got))
	again, err := repo.Get(ctx, ou, "team-a")
	require.NoError(t, err)
	assert.Equal(t, "150", again.Configuration.Limit.Amount)
	assert.True(t, again.UpdatedAt.After(again.CreatedAt), "updated_at moves")

	assert.ErrorIs(t, repo.Delete(ctx, otherOrg, "team-a"), gwerr.ErrBudgetNotFound)
	require.NoError(t, repo.Delete(ctx, ou, "team-a"))
	_, err = repo.Get(ctx, ou, "team-a")
	assert.ErrorIs(t, err, gwerr.ErrBudgetNotFound)
}

// A budget is owned by its organization or one of its projects; reads carry both handles,
// a gateway's push may move its budget between them, and a project with control-plane
// budgets cannot be deleted until they are, while its gateway budgets become the
// organization's.
func TestBudgetRepository_ProjectBudgets(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	orgHandle := "org-" + ou[len(ou)-12:]
	project := dbtest.Project(t, gdb, ou, "api-platform")
	repo := newBudgetRepo(gdb)

	orgBudget := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig("Org", "5000")}
	require.NoError(t, repo.CreateForControlPlane(ctx, &orgBudget, "org-monthly"))
	projectBudget := gwmodels.Budget{
		OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, ProjectID: &project, ProjectName: "api-platform",
		Configuration: budgetConfig("API Platform", "1000"),
	}
	require.NoError(t, repo.CreateForControlPlane(ctx, &projectBudget, "api-platform-monthly"))

	all := budgetsByName(t, repo, ou)
	assert.Equal(t, orgHandle, all["org-monthly"].OrgName)
	assert.Empty(t, all["org-monthly"].ProjectName)
	assert.Nil(t, all["org-monthly"].ProjectID)
	assert.Equal(t, orgHandle, all["api-platform-monthly"].OrgName)
	assert.Equal(t, "api-platform", all["api-platform-monthly"].ProjectName)
	require.NotNil(t, all["api-platform-monthly"].ProjectID)
	assert.Equal(t, project, *all["api-platform-monthly"].ProjectID)

	got, err := repo.Get(ctx, ou, "api-platform-monthly")
	require.NoError(t, err)
	assert.Equal(t, "api-platform", got.ProjectName)

	gw := newGateway(t, ou, gwmodels.GatewayRoleEgress)
	pushed := gatewayBudget(ou, gw, "team-daily", "50")
	pushed.ProjectID = &project
	stored, err := repo.SaveForGateway(ctx, &pushed, "team-daily")
	require.NoError(t, err)
	require.True(t, stored)
	assert.Equal(t, "api-platform", budgetsByName(t, repo, ou)["team-daily"].ProjectName)
	again := gatewayBudget(ou, gw, "team-daily", "60")
	stored, err = repo.SaveForGateway(ctx, &again, "team-daily")
	require.NoError(t, err)
	require.True(t, stored)
	assert.Empty(t, budgetsByName(t, repo, ou)["team-daily"].ProjectName, "a push without the annotation makes it the organization's")
	again.ProjectID = &project
	_, err = repo.SaveForGateway(ctx, &again, "team-daily")
	require.NoError(t, err)

	// The project finalize check: its control-plane budget refuses the delete.
	names, err := ProjectBudgets(gdb, ou, project.String())
	require.NoError(t, err)
	assert.Equal(t, []string{"api-platform-monthly"}, names)
	assert.NotNil(t, budgetsByName(t, repo, ou)["team-daily"].ProjectID, "nothing changes while it refuses")

	require.NoError(t, repo.Delete(ctx, ou, "api-platform-monthly"))
	names, err = ProjectBudgets(gdb, ou, project.String())
	require.NoError(t, err)
	assert.Empty(t, names)
	assert.Nil(t, budgetsByName(t, repo, ou)["team-daily"].ProjectID, "the gateway's budget becomes the organization's")
}

// An import saves every budget or none, and finds names budgets already hold.
func TestBudgetRepository_CreateManyIsAllOrNothing(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	repo := newBudgetRepo(gdb)
	createBudget(t, gdb, ou, "existing")

	taken, err := repo.TakenNames(ctx, ou, []string{"team-a", "existing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"existing"}, taken)

	newBudget := func(amount string) *gwmodels.Budget {
		return &gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig("x", amount)}
	}
	err = repo.CreateManyForControlPlane(ctx, []*gwmodels.Budget{newBudget("1"), newBudget("2")}, []string{"team-a", "existing"}, nil)
	var nameTaken *BudgetNameTakenError
	require.ErrorAs(t, err, &nameTaken)
	assert.Equal(t, "existing", nameTaken.Name)
	assert.ErrorIs(t, err, gwerr.ErrBudgetAlreadyExists)
	assert.ElementsMatch(t, []string{"existing"}, keys(budgetsByName(t, repo, ou)), "team-a was rolled back")

	require.NoError(t, repo.CreateManyForControlPlane(ctx, []*gwmodels.Budget{newBudget("1"), newBudget("2")}, []string{"team-a", "team-b"}, nil))
	assert.ElementsMatch(t, []string{"existing", "team-a", "team-b"}, keys(budgetsByName(t, repo, ou)))

	// Updates go in the same transaction: a failed create leaves an update undone.
	teamA := budgetsByName(t, repo, ou)["team-a"]
	teamA.Configuration.Limit.Amount = "99"
	err = repo.CreateManyForControlPlane(ctx, []*gwmodels.Budget{newBudget("3")}, []string{"existing"}, []*gwmodels.Budget{&teamA})
	require.ErrorAs(t, err, &nameTaken)
	assert.Equal(t, "1", budgetsByName(t, repo, ou)["team-a"].Configuration.Limit.Amount, "rolled back")
	require.NoError(t, repo.CreateManyForControlPlane(ctx, []*gwmodels.Budget{newBudget("3")}, []string{"team-c"}, []*gwmodels.Budget{&teamA}))
	assert.Equal(t, "99", budgetsByName(t, repo, ou)["team-a"].Configuration.Limit.Amount)
	assert.Len(t, budgetsByName(t, repo, ou), 4)
}

// The row holds the Budget kind as received; a provider a budget uses cannot be deleted.
func TestBudgetRepository_StoredKindAndBudgetsUsing(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	repo := newBudgetRepo(gdb)

	openai := "openai"
	cfg := budgetConfig("OpenAI monthly", "1000")
	cfg.Filters = []gwmodels.BudgetFilter{{Type: "llmProvider", ID: &openai}}
	cfg.Warnings = &gwmodels.BudgetWarnings{Thresholds: []int32{80}, Channels: []string{"finance-email"}}
	b := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: cfg}
	require.NoError(t, repo.CreateForControlPlane(ctx, &b, "openai-monthly"))

	var raw string
	require.NoError(t, gdb.Table("mod_ai_gateway_budgets").Select("configuration").Where("id = ?", b.ID).Scan(&raw).Error)
	var doc gwmodels.BudgetDocument
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	assert.Equal(t, gwmodels.BudgetAPIVersion, doc.APIVersion)
	assert.Equal(t, gwmodels.KindBudget, doc.Kind)
	assert.Equal(t, "openai-monthly", doc.Metadata.Name)
	assert.Equal(t, []string{"finance-email"}, doc.Spec.Warnings.Channels, "channels by name")
	read, err := repo.Get(ctx, ou, "openai-monthly")
	require.NoError(t, err)
	assert.Equal(t, cfg, read.Configuration, "the spec reads back")
	other := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig("All", "5")}
	require.NoError(t, repo.CreateForControlPlane(ctx, &other, "all-providers"))

	names, err := NewLLMProviderRepo(gdb, nil).BudgetsUsing(ctx, ou, "openai")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai-monthly"}, names)
	names, err = NewLLMProviderRepo(gdb, nil).BudgetsUsing(ctx, ou, "anthropic")
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestBudgetRepository_ChannelBudgetsNamesTheBudgetsThatUseAChannel(t *testing.T) {
	gdb := db.GetDB()
	ou := dbtest.Org(t, gdb)
	repo := newBudgetRepo(gdb)
	add := func(name string, channels ...string) {
		cfg := budgetConfig(name, "50")
		if channels != nil {
			cfg.Warnings = &gwmodels.BudgetWarnings{Thresholds: []int32{80}, Channels: channels}
		}
		b := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: cfg}
		require.NoError(t, repo.CreateForControlPlane(context.Background(), &b, name))
	}
	add("org-daily", "finance-webhook", "pasan-email")
	add("openai-monthly", "finance-webhook")
	add("returns-bot-weekly", "support-email")
	add("no-warnings")

	names, err := ChannelBudgets(gdb, ou, "finance-webhook")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai-monthly", "org-daily"}, names, "sorted by name")

	names, err = ChannelBudgets(gdb, ou, "unused-channel")
	require.NoError(t, err)
	assert.Empty(t, names)

	names, err = ChannelBudgets(gdb, dbtest.Org(t, gdb), "finance-webhook")
	require.NoError(t, err)
	assert.Empty(t, names, "another organization's budgets are not counted")
}

func TestBudgetRepository_ATakenNameIsRefused(t *testing.T) {
	gdb := db.GetDB()
	ou := dbtest.Org(t, gdb)
	createBudget(t, gdb, ou, "org-daily")
	b := gwmodels.Budget{OrgID: ou, Origin: gwmodels.BudgetOriginControlPlane, Configuration: budgetConfig("org-daily", "10")}
	err := newBudgetRepo(gdb).CreateForControlPlane(context.Background(), &b, "org-daily")
	require.ErrorIs(t, err, gwerr.ErrBudgetAlreadyExists)
	var taken *BudgetNameTakenError
	require.ErrorAs(t, err, &taken)
	assert.Equal(t, `budget name "org-daily" is taken`, taken.Error())
}
