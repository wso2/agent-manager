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
	"log/slog"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	a2aCatalogPipelineTimeout     = 5 * time.Second
	a2aCatalogPipelineConcurrency = 8
)

// a2aEnvironmentOrders fetches each project's pipeline order at most once, concurrently, under one deadline.
// Not-found pipelines (ErrProjectNotFound, ErrDeploymentPipelineNotFound, ErrNotFound) and nil pipelines give a nil order; any other error fails as "order a2a catalogue environments: %w".
func a2aEnvironmentOrders(
	ctx context.Context,
	oc client.OpenChoreoClient,
	logger *slog.Logger,
	ouID string,
	projects []string,
	timeout time.Duration,
) (map[string][]string, error) {
	distinct := distinctProjects(projects)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(a2aCatalogPipelineConcurrency)

	ordersByPosition := make([][]string, len(distinct))
	for i, project := range distinct {
		g.Go(func() error {
			order, err := projectEnvironmentOrder(gctx, oc, logger, ouID, project)
			if err != nil {
				return fmt.Errorf("order a2a catalogue environments: %w", err)
			}
			ordersByPosition[i] = order
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	orders := make(map[string][]string, len(distinct))
	for i, project := range distinct {
		orders[project] = ordersByPosition[i]
	}
	return orders, nil
}

func projectEnvironmentOrder(
	ctx context.Context,
	oc client.OpenChoreoClient,
	logger *slog.Logger,
	ouID, project string,
) ([]string, error) {
	pipeline, err := oc.GetProjectDeploymentPipeline(ctx, ouID, project)
	if isMissingPipeline(err) {
		logger.Info("A2A catalogue: project has no deployment pipeline; using name order",
			"ouID", ouID, "project", project)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		return nil, nil
	}
	return client.BuildEnvironmentOrder(pipeline.PromotionPaths), nil
}

func isMissingPipeline(err error) bool {
	return errors.Is(err, utils.ErrProjectNotFound) ||
		errors.Is(err, utils.ErrDeploymentPipelineNotFound) ||
		errors.Is(err, utils.ErrNotFound)
}

func distinctProjects(projects []string) []string {
	seen := make(map[string]bool, len(projects))
	distinct := make([]string, 0, len(projects))
	for _, project := range projects {
		if !seen[project] {
			seen[project] = true
			distinct = append(distinct, project)
		}
	}
	return distinct
}

// sortA2ACatalogRows orders rows by pipeline position, then unknown environments by name; nil order means name order. Stable.
func sortA2ACatalogRows(rows []models.A2ACatalogRow, order []string) {
	position := make(map[string]int, len(order))
	for i, env := range order {
		position[env] = i
	}
	rank := func(env string) int {
		if i, ok := position[env]; ok {
			return i
		}
		return len(order)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := rank(rows[i].EnvironmentName), rank(rows[j].EnvironmentName)
		if ri != rj {
			return ri < rj
		}
		return rows[i].EnvironmentName < rows[j].EnvironmentName
	})
}
