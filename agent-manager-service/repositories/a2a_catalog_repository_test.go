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

package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newCatalogOU() string {
	return "ou-" + uuid.New().String()[:8]
}

// seedCatalogCard enqueues one agent-environment row; a fetched row also gets the card and its search text.
func seedCatalogCard(
	t *testing.T, repo A2AAgentCardRepository, ou, project, agent, env string,
	source models.A2AAgentCardSource, status models.A2AAgentCardStatus, searchText, card string,
) *models.A2AAgentCard {
	t.Helper()
	ctx := context.Background()
	row := inEnv(newTestCard(agent, source), env)
	row.OUID, row.ProjectName = ou, project
	cleanupCard(t, repo, row)
	require.NoError(t, repo.Enqueue(ctx, row))
	if status != models.A2AAgentCardStatusFetched {
		return row
	}
	read := claimedCardRow(t, repo, row.ID)
	projection := models.A2ACardProjection{Fingerprint: "fp-" + agent, SearchText: searchText}
	require.NoError(t, repo.MarkFetched(ctx, read, json.RawMessage(card), "hash-"+agent, "", "", projection))
	return row
}

// claimedCardRow claims the row with id, as the reconciler would; agent names repeat across this file's orgs.
func claimedCardRow(t *testing.T, repo A2AAgentCardRepository, id uuid.UUID) models.A2AAgentCard {
	t.Helper()
	due, err := repo.ClaimDue(context.Background(), time.Now(), 100)
	require.NoError(t, err)
	for _, row := range due {
		if row.ID == id {
			return row
		}
	}
	require.FailNow(t, "no due card row", "id %s", id)
	return models.A2AAgentCard{}
}

func seedFetchedCard(t *testing.T, repo A2AAgentCardRepository, ou, project, agent, env, searchText string) *models.A2AAgentCard {
	t.Helper()
	return seedCatalogCard(t, repo, ou, project, agent, env,
		models.A2AAgentCardSourceExternal, models.A2AAgentCardStatusFetched, searchText, sampleCard)
}

func seedPendingCard(t *testing.T, repo A2AAgentCardRepository, ou, project, agent, env string) *models.A2AAgentCard {
	t.Helper()
	return seedCatalogCard(t, repo, ou, project, agent, env,
		models.A2AAgentCardSourceExternal, models.A2AAgentCardStatusPending, "", "")
}

func seedProxyFor(t *testing.T, repo A2AProxyRepository, card *models.A2AAgentCard) {
	t.Helper()
	proxy := newTestProxy(card.AgentName)
	proxy.OUID, proxy.ProjectName = card.OUID, card.ProjectName
	proxy.EnvironmentName, proxy.EnvironmentUUID = card.EnvironmentName, card.EnvironmentUUID
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(context.Background(), proxy))
}

func catalogRepos() (A2ACatalogRepository, A2AAgentCardRepository, A2AProxyRepository) {
	return NewA2ACatalogRepository(db.GetDB()), NewA2AAgentCardRepository(db.GetDB()), NewA2AProxyRepository(db.GetDB())
}

func listCatalog(t *testing.T, repo A2ACatalogRepository, filter models.A2ACatalogFilter) ([]models.A2ACatalogRow, int) {
	t.Helper()
	rows, total, err := repo.List(context.Background(), filter)
	require.NoError(t, err)
	return rows, total
}

// rowKeys names each row "agent/environment".
func rowKeys(rows []models.A2ACatalogRow) []string {
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.AgentName+"/"+r.EnvironmentName)
	}
	return keys
}

func matchedFlags(rows []models.A2ACatalogRow) []bool {
	flags := make([]bool, 0, len(rows))
	for _, r := range rows {
		flags = append(flags, r.Matched)
	}
	return flags
}

func proxiedFlags(rows []models.A2ACatalogRow) []bool {
	flags := make([]bool, 0, len(rows))
	for _, r := range rows {
		flags = append(flags, r.Proxied)
	}
	return flags
}

func TestA2ACatalogListPagesByAgent(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "a", "dev", "alpha")
	seedFetchedCard(t, cards, ou, "p", "a", "prod", "alpha")
	seedFetchedCard(t, cards, ou, "p", "b", "dev", "beta")
	seedFetchedCard(t, cards, ou, "p", "c", "dev", "gamma")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Limit: 2})
	assert.Equal(t, 3, total)
	assert.Equal(t, []string{"a/dev", "a/prod", "b/dev"}, rowKeys(rows))

	rows, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Limit: 2, Offset: 2})
	assert.Equal(t, 3, total)
	assert.Equal(t, []string{"c/dev"}, rowKeys(rows))

	rows, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou})
	assert.Equal(t, 3, total)
	assert.Len(t, rows, 4, "limit 0 means no limit")
}

func TestA2ACatalogListPastTheEndKeepsTheTotal(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "a", "dev", "alpha")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Limit: 2, Offset: 4})
	assert.Empty(t, rows)
	assert.Equal(t, 1, total)
}

func TestA2ACatalogListPastTheEndCountsOnlyMatches(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "x", "dev", "issue a refund")
	seedFetchedCard(t, cards, ou, "p", "y", "dev", "book a trip")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: "refund", Limit: 2, Offset: 4})
	assert.Empty(t, rows)
	assert.Equal(t, 1, total)
}

func TestA2ACatalogListSeparatesProjects(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p2", "orders", "dev", "orders")
	seedFetchedCard(t, cards, ou, "p1", "orders", "dev", "orders")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Limit: 10})
	assert.Equal(t, 2, total)
	require.Len(t, rows, 2)
	assert.Equal(t, "p1", rows[0].ProjectName, "entries are ordered by project first")
	assert.Equal(t, "p2", rows[1].ProjectName)
}

func TestA2ACatalogListForAgent(t *testing.T) {
	catalog, cards, proxies := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p1", "orders", "dev", "orders")
	seedProxyFor(t, proxies, seedFetchedCard(t, cards, ou, "p1", "orders", "prod", "orders"))
	seedCatalogCard(t, cards, ou, "p2", "orders", "dev",
		models.A2AAgentCardSourcePlatform, models.A2AAgentCardStatusFetched, "orders", sampleCard)

	rows, err := catalog.ListForAgent(context.Background(), ou, "p1", "orders")
	require.NoError(t, err)
	assert.Equal(t, []string{"orders/dev", "orders/prod"}, rowKeys(rows))
	assert.Equal(t, []bool{false, true}, proxiedFlags(rows))
	assert.Equal(t, []bool{false, false}, matchedFlags(rows))

	rows, err = catalog.ListForAgent(context.Background(), ou, "p1", "unknown")
	require.NoError(t, err)
	assert.NotNil(t, rows)
	assert.Empty(t, rows)
}

func TestA2ACatalogListFindsAPendingAgentByName(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedPendingCard(t, cards, ou, "p", "orders-bot", "dev")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: "ORDERS", Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"orders-bot/dev"}, rowKeys(rows))
	assert.Equal(t, []bool{true}, matchedFlags(rows))
}

func TestA2ACatalogListMarksTheMatchingRows(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "a", "dev", "book a trip")
	seedFetchedCard(t, cards, ou, "p", "a", "prod", "issue a Refund")
	seedFetchedCard(t, cards, ou, "p", "b", "dev", "track a parcel")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: "refund", Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"a/dev", "a/prod"}, rowKeys(rows))
	assert.Equal(t, []bool{false, true}, matchedFlags(rows))
}

func TestA2ACatalogListNeverMatchesJSONSyntax(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	// The text a card with tag and example arrays projects to: values only, one per line.
	seedFetchedCard(t, cards, ou, "p", "tagged", "dev", "billing\ninvoices\nsend an invoice")
	seedFetchedCard(t, cards, ou, "p", "greeter", "dev", "s\nsay \"hi\"")

	for _, q := range []string{"[", `", "`, "null", "{"} {
		_, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: q, Limit: 10})
		assert.Equal(t, 0, total, "q=%s", q)
	}

	rows, _ := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: `"`, Limit: 10})
	assert.Equal(t, []string{"greeter/dev"}, rowKeys(rows), "only a literal quote in the text matches")

	rows, _ = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: `say "hi"`, Limit: 10})
	assert.Equal(t, []string{"greeter/dev"}, rowKeys(rows))
}

func TestA2ACatalogListEscapesWildcards(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "uptime", "dev", "100% uptime bot")
	seedFetchedCard(t, cards, ou, "p", "ledger", "dev", "Ledger")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: "%", Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"uptime/dev"}, rowKeys(rows))

	rows, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: "_", Limit: 10})
	assert.Equal(t, 0, total)
	assert.Empty(t, rows)
}

// A backslash is the LIKE escape character, so it is the one a missed escape turns into a broken or wider pattern.
func TestA2ACatalogListMatchesABackslashLiterally(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "greeter", "dev", `say "hi" \ now`)
	seedFetchedCard(t, cards, ou, "p", "clock", "dev", "right now")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: `\`, Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"greeter/dev"}, rowKeys(rows))

	rows, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: `\ now`, Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"greeter/dev"}, rowKeys(rows), "the backslash is not taken as escaping the space")
}

func TestA2ACatalogListLeavesPendingAgentsOutOfSearch(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedPendingCard(t, cards, ou, "p", "waiting", "dev")

	_, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Query: "x", Limit: 10})
	assert.Equal(t, 0, total)

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"waiting/dev"}, rowKeys(rows))
	assert.Equal(t, []bool{false}, matchedFlags(rows))
}

func TestA2ACatalogListFiltersByKindAndEnvironment(t *testing.T) {
	catalog, cards, proxies := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p", "ext", "dev", "ext")
	seedProxyFor(t, proxies, seedFetchedCard(t, cards, ou, "p", "ext", "prod", "ext"))

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Kind: models.A2ACatalogKindProxied, Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"ext/dev", "ext/prod"}, rowKeys(rows))
	assert.Equal(t, []bool{false, true}, proxiedFlags(rows))

	_, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Kind: models.A2ACatalogKindExternal, Limit: 10})
	assert.Equal(t, 1, total)

	rows, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Kind: models.A2ACatalogKindPlatform, Limit: 10})
	assert.Equal(t, 0, total)
	assert.Empty(t, rows)

	rows, total = listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Environment: "prod", Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"ext/dev", "ext/prod"}, rowKeys(rows), "row filters never prune the environments")
}

func TestA2ACatalogListIsScopedToTheOrg(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	mine, theirs := newCatalogOU(), newCatalogOU()
	seedFetchedCard(t, cards, mine, "p", "orders", "dev", "orders")
	seedFetchedCard(t, cards, theirs, "p", "orders", "prod", "orders")
	seedFetchedCard(t, cards, theirs, "p", "billing", "dev", "billing")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: mine, Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"orders/dev"}, rowKeys(rows))
	for _, r := range rows {
		assert.Equal(t, mine, r.OUID)
	}

	rows, err := catalog.ListForAgent(context.Background(), mine, "p", "orders")
	require.NoError(t, err)
	assert.Equal(t, []string{"orders/dev"}, rowKeys(rows))
}

func TestA2ACatalogListEvaluatesFiltersPerRow(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedPendingCard(t, cards, ou, "p", "a", "prod")
	seedFetchedCard(t, cards, ou, "p", "a", "dev", "alpha")

	_, total := listCatalog(t, catalog, models.A2ACatalogFilter{
		OUID: ou, Status: models.A2AAgentCardStatusFetched, Environment: "prod", Limit: 10,
	})
	assert.Equal(t, 0, total)

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{
		OUID: ou, Status: models.A2AAgentCardStatusFetched, Environment: "dev", Limit: 10,
	})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"a/dev", "a/prod"}, rowKeys(rows))
}

func TestA2ACatalogListFiltersByProject(t *testing.T) {
	catalog, cards, _ := catalogRepos()
	ou := newCatalogOU()
	seedFetchedCard(t, cards, ou, "p1", "orders", "dev", "orders")
	seedFetchedCard(t, cards, ou, "p2", "billing", "dev", "billing")

	rows, total := listCatalog(t, catalog, models.A2ACatalogFilter{OUID: ou, Project: "p2", Limit: 10})
	assert.Equal(t, 1, total)
	assert.Equal(t, []string{"billing/dev"}, rowKeys(rows))
}
