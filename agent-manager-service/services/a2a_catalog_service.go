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
	"fmt"
	"log/slog"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A2ACatalogService reads the organisation's A2A agents as catalogue entries, one per agent.
type A2ACatalogService interface {
	ListA2ACatalog(ctx context.Context, filter models.A2ACatalogFilter) (*models.A2ACatalogPage, error)
	// GetA2ACatalogEntry returns utils.ErrAgentNotInA2ACatalog when the agent has no card rows.
	GetA2ACatalogEntry(ctx context.Context, ouID, projectName, agentName string) (*models.A2ACatalogEntry, error)
	// GetA2AConnectDetails returns the card service's and utils.ErrEnvironmentNotFound sentinels unchanged.
	GetA2AConnectDetails(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AConnectDetails, error)
}

type a2aCatalogService struct {
	catalogRepo     repositories.A2ACatalogRepository
	cardService     A2AAgentCardService
	proxyRepo       repositories.A2AProxyRepository
	publisher       *A2APublisher
	agentConfigRepo repositories.AgentConfigRepository
	gatewayRepo     repositories.GatewayRepository
	ocClient        client.OpenChoreoClient
	logger          *slog.Logger
	pipelineTimeout time.Duration
}

// NewA2ACatalogService stores every dependency now; the card, proxy, publisher, config and gateway ones serve connect details.
func NewA2ACatalogService(
	catalogRepo repositories.A2ACatalogRepository,
	cardService A2AAgentCardService,
	proxyRepo repositories.A2AProxyRepository,
	publisher *A2APublisher,
	agentConfigRepo repositories.AgentConfigRepository,
	gatewayRepo repositories.GatewayRepository,
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
) A2ACatalogService {
	return &a2aCatalogService{
		catalogRepo:     catalogRepo,
		cardService:     cardService,
		proxyRepo:       proxyRepo,
		publisher:       publisher,
		agentConfigRepo: agentConfigRepo,
		gatewayRepo:     gatewayRepo,
		ocClient:        ocClient,
		logger:          logger,
		pipelineTimeout: a2aCatalogPipelineTimeout,
	}
}

func (s *a2aCatalogService) ListA2ACatalog(ctx context.Context, filter models.A2ACatalogFilter) (*models.A2ACatalogPage, error) {
	rows, total, err := s.catalogRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list a2a catalogue agents: %w", err)
	}
	groups := groupA2ACatalogRowsByAgent(rows)
	projects := make([]string, 0, len(groups))
	for _, group := range groups {
		projects = append(projects, group[0].ProjectName)
	}
	orders, err := a2aEnvironmentOrders(ctx, s.ocClient, s.logger, filter.OUID, projects, s.pipelineTimeout)
	if err != nil {
		return nil, err
	}
	entries := make([]models.A2ACatalogEntry, 0, len(groups))
	for _, group := range groups {
		entries = append(entries, buildA2ACatalogEntry(group, orders[group[0].ProjectName]))
	}
	return &models.A2ACatalogPage{Entries: entries, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *a2aCatalogService) GetA2ACatalogEntry(ctx context.Context, ouID, projectName, agentName string) (*models.A2ACatalogEntry, error) {
	rows, err := s.catalogRepo.ListForAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("get a2a catalogue agent: %w", err)
	}
	if len(rows) == 0 {
		return nil, utils.ErrAgentNotInA2ACatalog
	}
	orders, err := a2aEnvironmentOrders(ctx, s.ocClient, s.logger, ouID, []string{projectName}, s.pipelineTimeout)
	if err != nil {
		return nil, err
	}
	entry := buildA2ACatalogEntry(rows, orders[projectName])
	return &entry, nil
}

// groupA2ACatalogRowsByAgent splits rows into one group per (project, agent), in first-seen order.
func groupA2ACatalogRowsByAgent(rows []models.A2ACatalogRow) [][]models.A2ACatalogRow {
	type agentKey struct{ project, agent string }
	index := make(map[agentKey]int)
	var groups [][]models.A2ACatalogRow
	for _, row := range rows {
		key := agentKey{project: row.ProjectName, agent: row.AgentName}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], row)
	}
	return groups
}

// buildA2ACatalogEntry summarises one agent's rows (non-empty) from its lead environment: the first, in pipeline order, with a card.
func buildA2ACatalogEntry(rows []models.A2ACatalogRow, order []string) models.A2ACatalogEntry {
	sortA2ACatalogRows(rows, order)
	lead := leadA2ACatalogRow(rows)
	summary := a2aCardSummaryOf(lead.Card)
	name := summary.Name
	if name == "" {
		name = lead.AgentName
	}

	environments := make([]models.A2ACatalogEnvironment, 0, len(rows))
	for _, row := range rows {
		environments = append(environments, models.A2ACatalogEnvironment{
			Name:        row.EnvironmentName,
			Status:      row.Status,
			FetchedAt:   row.FetchedAt,
			Proxied:     row.Proxied,
			CardDiffers: fingerprintsDiffer(lead.CardFingerprint, row.CardFingerprint),
			Matched:     row.Matched,
		})
	}
	return models.A2ACatalogEntry{
		ProjectName:     lead.ProjectName,
		AgentName:       lead.AgentName,
		Kind:            rows[0].Source,
		Name:            name,
		Description:     summary.Description,
		Version:         summary.Version,
		Skills:          summary.Skills,
		LeadEnvironment: lead.EnvironmentName,
		Environments:    environments,
	}
}

func leadA2ACatalogRow(rows []models.A2ACatalogRow) models.A2ACatalogRow {
	for _, row := range rows {
		if len(row.Card) > 0 {
			return row
		}
	}
	return rows[0]
}

// fingerprintsDiffer treats an empty fingerprint (a card not yet re-fetched since projection was added) as unknown, never different.
func fingerprintsDiffer(lead, other string) bool {
	return lead != "" && other != "" && lead != other
}
