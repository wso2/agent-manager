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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type catalogHarness struct {
	catalog *repomocks.A2ACatalogRepositoryMock
	oc      *clientmocks.OpenChoreoClientMock
	svc     *a2aCatalogService
}

func devStagingProdPipeline() *models.DeploymentPipelineResponse {
	return &models.DeploymentPipelineResponse{
		PromotionPaths: []models.PromotionPath{
			{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "prod"}}},
		},
	}
}

func newCatalogHarness(t *testing.T) *catalogHarness {
	t.Helper()
	catalog := &repomocks.A2ACatalogRepositoryMock{}
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return devStagingProdPipeline(), nil
		},
	}
	svc, ok := NewA2ACatalogService(catalog, nil, nil, nil, nil, nil, oc, discardLogger()).(*a2aCatalogService)
	require.True(t, ok)
	return &catalogHarness{catalog: catalog, oc: oc, svc: svc}
}

func (h *catalogHarness) listReturns(rows ...models.A2ACatalogRow) {
	h.catalog.ListFunc = func(context.Context, models.A2ACatalogFilter) ([]models.A2ACatalogRow, int, error) {
		return rows, len(rows), nil
	}
}

func row(project, agent, env string, card string, fingerprint string) models.A2ACatalogRow {
	var r models.A2ACatalogRow
	r.ProjectName = project
	r.AgentName = agent
	r.EnvironmentName = env
	r.Source = models.A2AAgentCardSourcePlatform
	r.Status = models.A2AAgentCardStatusFetched
	if card != "" {
		r.Card = json.RawMessage(card)
	}
	r.CardFingerprint = fingerprint
	return r
}

func entryEnvironments(entry models.A2ACatalogEntry) []string {
	names := make([]string, 0, len(entry.Environments))
	for _, env := range entry.Environments {
		names = append(names, env.Name)
	}
	return names
}

func differingEnvironments(entry models.A2ACatalogEntry) []string {
	names := []string{}
	for _, env := range entry.Environments {
		if env.CardDiffers {
			names = append(names, env.Name)
		}
	}
	return names
}

func listOnlyEntry(t *testing.T, h *catalogHarness) models.A2ACatalogEntry {
	t.Helper()
	page, err := h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	return page.Entries[0]
}

func TestListA2ACatalogOrdersEnvironmentsAndSummarisesTheLead(t *testing.T) {
	h := newCatalogHarness(t)
	devCard := `{"name":"Trip","description":"d","version":"1","skills":[{"name":"plan"}]}`
	h.listReturns(
		row("p1", "trip", "prod", "", ""),
		row("p1", "trip", "dev", devCard, "a"),
		row("p1", "trip", "staging", "", ""),
	)

	entry := listOnlyEntry(t, h)

	assert.Equal(t, []string{"dev", "staging", "prod"}, entryEnvironments(entry))
	assert.Equal(t, "dev", entry.LeadEnvironment)
	assert.Equal(t, "p1", entry.ProjectName)
	assert.Equal(t, "trip", entry.AgentName)
	assert.Equal(t, models.A2AAgentCardSourcePlatform, entry.Kind)
	assert.Equal(t, "Trip", entry.Name)
	assert.Equal(t, "d", entry.Description)
	assert.Equal(t, "1", entry.Version)
	assert.Equal(t, []string{"plan"}, entry.Skills)
}

func TestListA2ACatalogFlagsTheDifferingEnvironment(t *testing.T) {
	h := newCatalogHarness(t)
	card := func(url string, skills string) string {
		return `{"name":"Trip","supportedInterfaces":[{"url":"` + url + `"}],"skills":[` + skills + `]}`
	}
	h.listReturns(
		row("p1", "trip", "dev", card("https://dev", `{"name":"plan"}`), "same"),
		row("p1", "trip", "staging", card("https://staging", `{"name":"plan"}`), "same"),
		row("p1", "trip", "prod", card("https://prod", `{"name":"plan"},{"name":"book"}`), "other"),
	)

	entry := listOnlyEntry(t, h)

	assert.Equal(t, []string{"prod"}, differingEnvironments(entry))
}

func TestListA2ACatalogLeadIsTheFirstEnvironmentWithACard(t *testing.T) {
	h := newCatalogHarness(t)
	h.listReturns(
		row("p1", "trip", "dev", "", ""),
		row("p1", "trip", "staging", `{"skills":[{"name":"one"}]}`, "a"),
		row("p1", "trip", "prod", `{"skills":[{"name":"one"},{"name":"two"}]}`, "b"),
	)

	entry := listOnlyEntry(t, h)

	assert.Equal(t, "staging", entry.LeadEnvironment)
	assert.Equal(t, []string{"one"}, entry.Skills)
	assert.Equal(t, []string{"prod"}, differingEnvironments(entry))
}

func TestListA2ACatalogWithoutAnyCard(t *testing.T) {
	h := newCatalogHarness(t)
	h.listReturns(
		row("p1", "trip", "prod", "", ""),
		row("p1", "trip", "dev", "", ""),
	)

	entry := listOnlyEntry(t, h)

	assert.Equal(t, "dev", entry.LeadEnvironment)
	assert.Equal(t, "trip", entry.Name)
	assert.Empty(t, entry.Description)
	assert.NotNil(t, entry.Skills)
	assert.Empty(t, entry.Skills)
	assert.Empty(t, differingEnvironments(entry))
}

func TestListA2ACatalogUsesTheStoredFingerprint(t *testing.T) {
	h := newCatalogHarness(t)
	card := `{"name":"Trip"}`
	h.listReturns(
		row("p1", "trip", "dev", card, "a"),
		row("p1", "trip", "staging", card, "b"),
	)

	entry := listOnlyEntry(t, h)

	assert.Equal(t, []string{"staging"}, differingEnvironments(entry))
}

func TestListA2ACatalogNeverFlagsAnEmptyFingerprint(t *testing.T) {
	cases := map[string][2]string{
		"empty reference": {"", "x"},
		"empty row":       {"x", ""},
	}
	for name, fingerprints := range cases {
		t.Run(name, func(t *testing.T) {
			h := newCatalogHarness(t)
			h.listReturns(
				row("p1", "trip", "dev", `{"name":"A"}`, fingerprints[0]),
				row("p1", "trip", "staging", `{"name":"B"}`, fingerprints[1]),
			)

			entry := listOnlyEntry(t, h)

			assert.Empty(t, differingEnvironments(entry))
		})
	}
}

func TestListA2ACatalogFailsOnPipelineErrors(t *testing.T) {
	h := newCatalogHarness(t)
	cause := errors.New("503")
	h.oc.GetProjectDeploymentPipelineFunc = func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
		return nil, cause
	}
	rows := []models.A2ACatalogRow{row("p1", "trip", "dev", "", "")}
	h.listReturns(rows...)
	h.catalog.ListForAgentFunc = func(context.Context, string, string, string) ([]models.A2ACatalogRow, error) {
		return rows, nil
	}

	_, err := h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	require.ErrorIs(t, err, cause)
	assert.NotErrorIs(t, err, utils.ErrAgentNotInA2ACatalog)

	_, err = h.svc.GetA2ACatalogEntry(context.Background(), "ou", "p1", "trip")
	require.ErrorIs(t, err, cause)
	assert.NotErrorIs(t, err, utils.ErrAgentNotInA2ACatalog)
}

func TestListA2ACatalogHonoursTheTimeout(t *testing.T) {
	const timeout = 50 * time.Millisecond
	h := newCatalogHarness(t)
	h.svc.pipelineTimeout = timeout
	h.oc.GetProjectDeploymentPipelineFunc = func(ctx context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	h.listReturns(row("p1", "trip", "dev", "", ""))

	start := time.Now()
	_, err := h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, timeout+time.Second)
}

func TestListA2ACatalogFetchesThePipelineOncePerProject(t *testing.T) {
	h := newCatalogHarness(t)
	h.listReturns(
		row("p1", "trip", "dev", "", ""),
		row("p1", "trip", "prod", "", ""),
		row("p1", "hotel", "dev", "", ""),
		row("p2", "taxi", "dev", "", ""),
	)

	page, err := h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	require.NoError(t, err)
	assert.Len(t, page.Entries, 3)
	assert.Len(t, h.oc.GetProjectDeploymentPipelineCalls(), 2)

	_, err = h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	require.NoError(t, err)
	assert.Len(t, h.oc.GetProjectDeploymentPipelineCalls(), 4)

	notFound := newCatalogHarness(t)
	notFound.oc.GetProjectDeploymentPipelineFunc = func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
		return nil, utils.ErrDeploymentPipelineNotFound
	}
	notFound.listReturns(
		row("p1", "trip", "prod", "", ""),
		row("p1", "hotel", "dev", "", ""),
	)

	page, err = notFound.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	require.NoError(t, err)
	assert.Len(t, page.Entries, 2)
	assert.Len(t, notFound.oc.GetProjectDeploymentPipelineCalls(), 1)
}

func TestListA2ACatalogEchoesTheRepositoryFlagsAndPaging(t *testing.T) {
	h := newCatalogHarness(t)
	flagged := row("p1", "trip", "dev", "", "")
	flagged.Proxied = true
	flagged.Matched = true
	fetchedAt := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	flagged.FetchedAt = &fetchedAt
	h.catalog.ListFunc = func(context.Context, models.A2ACatalogFilter) ([]models.A2ACatalogRow, int, error) {
		return []models.A2ACatalogRow{flagged}, 42, nil
	}
	filter := models.A2ACatalogFilter{OUID: "ou", Limit: 10, Offset: 20}

	page, err := h.svc.ListA2ACatalog(context.Background(), filter)

	require.NoError(t, err)
	assert.Equal(t, 42, page.Total)
	assert.Equal(t, 10, page.Limit)
	assert.Equal(t, 20, page.Offset)
	require.Len(t, h.catalog.ListCalls(), 1)
	assert.Equal(t, filter, h.catalog.ListCalls()[0].Filter)
	require.Len(t, page.Entries, 1)
	env := page.Entries[0].Environments[0]
	assert.True(t, env.Proxied)
	assert.True(t, env.Matched)
	assert.Equal(t, flagged.Status, env.Status)
	assert.Equal(t, &fetchedAt, env.FetchedAt)
}

func TestListA2ACatalogReturnsAnEmptyPage(t *testing.T) {
	h := newCatalogHarness(t)
	h.listReturns()

	page, err := h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})

	require.NoError(t, err)
	assert.NotNil(t, page.Entries)
	assert.Empty(t, page.Entries)
}

func TestListA2ACatalogWrapsRepositoryErrors(t *testing.T) {
	h := newCatalogHarness(t)
	h.catalog.ListFunc = func(context.Context, models.A2ACatalogFilter) ([]models.A2ACatalogRow, int, error) {
		return nil, 0, assert.AnError
	}
	h.catalog.ListForAgentFunc = func(context.Context, string, string, string) ([]models.A2ACatalogRow, error) {
		return nil, assert.AnError
	}

	_, err := h.svc.ListA2ACatalog(context.Background(), models.A2ACatalogFilter{OUID: "ou"})
	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "list a2a catalogue agents: ")

	_, err = h.svc.GetA2ACatalogEntry(context.Background(), "ou", "p1", "trip")
	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "get a2a catalogue agent: ")
	assert.NotErrorIs(t, err, utils.ErrAgentNotInA2ACatalog)
}

func TestGetA2ACatalogEntryNotInCatalogue(t *testing.T) {
	h := newCatalogHarness(t)
	h.catalog.ListForAgentFunc = func(context.Context, string, string, string) ([]models.A2ACatalogRow, error) {
		return []models.A2ACatalogRow{}, nil
	}

	_, err := h.svc.GetA2ACatalogEntry(context.Background(), "ou", "p1", "trip")

	require.ErrorIs(t, err, utils.ErrAgentNotInA2ACatalog)
	assert.Empty(t, h.oc.GetProjectDeploymentPipelineCalls())
}

func TestGetA2ACatalogEntryUsesOnlyThatProject(t *testing.T) {
	h := newCatalogHarness(t)
	h.catalog.ListForAgentFunc = func(context.Context, string, string, string) ([]models.A2ACatalogRow, error) {
		return []models.A2ACatalogRow{
			row("p1", "trip", "dev", "", ""),
			row("p1", "trip", "prod", "", ""),
			row("p1", "trip", "staging", `{"name":"Trip"}`, "a"),
		}, nil
	}

	entry, err := h.svc.GetA2ACatalogEntry(context.Background(), "ou", "p1", "trip")

	require.NoError(t, err)
	require.Len(t, h.catalog.ListForAgentCalls(), 1)
	call := h.catalog.ListForAgentCalls()[0]
	assert.Equal(t, []string{"ou", "p1", "trip"}, []string{call.OuID, call.ProjectName, call.AgentName})
	calls := h.oc.GetProjectDeploymentPipelineCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "p1", calls[0].ProjectName)
	assert.Equal(t, []string{"dev", "staging", "prod"}, entryEnvironments(*entry))
	assert.Equal(t, "staging", entry.LeadEnvironment)
	assert.Equal(t, "Trip", entry.Name)
}
