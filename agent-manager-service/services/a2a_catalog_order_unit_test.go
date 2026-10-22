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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func catalogRowsIn(envs ...string) []models.A2ACatalogRow {
	rows := make([]models.A2ACatalogRow, 0, len(envs))
	for _, env := range envs {
		var row models.A2ACatalogRow
		row.EnvironmentName = env
		rows = append(rows, row)
	}
	return rows
}

func catalogRowEnvironments(rows []models.A2ACatalogRow) []string {
	envs := make([]string, 0, len(rows))
	for _, row := range rows {
		envs = append(envs, row.EnvironmentName)
	}
	return envs
}

func pipelineLookup(pipeline *models.DeploymentPipelineResponse, err error) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return pipeline, err
		},
	}
}

func TestSortA2ACatalogRowsUsesPipelineThenName(t *testing.T) {
	rows := catalogRowsIn("zeta", "alpha", "prod", "dev")
	sortA2ACatalogRows(rows, []string{"dev", "prod"})
	assert.Equal(t, []string{"dev", "prod", "alpha", "zeta"}, catalogRowEnvironments(rows))

	rows = catalogRowsIn("zeta", "alpha", "prod", "dev")
	sortA2ACatalogRows(rows, nil)
	assert.Equal(t, []string{"alpha", "dev", "prod", "zeta"}, catalogRowEnvironments(rows))
}

func TestSortA2ACatalogRowsSkipsPipelineNamesWithoutRows(t *testing.T) {
	rows := catalogRowsIn("prod", "dev")
	sortA2ACatalogRows(rows, []string{"dev", "staging", "prod"})
	assert.Equal(t, []string{"dev", "prod"}, catalogRowEnvironments(rows))

	rows = catalogRowsIn("b", "a")
	sortA2ACatalogRows(rows, []string{"x", "y"})
	assert.Equal(t, []string{"a", "b"}, catalogRowEnvironments(rows))
}

func TestA2AEnvironmentOrdersFallBackOnNotFound(t *testing.T) {
	cases := map[string]error{
		"project not found":             fmt.Errorf("get project: %w", utils.ErrProjectNotFound),
		"deployment pipeline not found": fmt.Errorf("get project: %w", utils.ErrDeploymentPipelineNotFound),
		"generic not found":             fmt.Errorf("get pipeline: %w", utils.ErrNotFound),
		"nil pipeline without error":    nil,
	}
	for name, lookupErr := range cases {
		t.Run(name, func(t *testing.T) {
			oc := pipelineLookup(nil, lookupErr)

			orders, err := a2aEnvironmentOrders(context.Background(), oc, discardLogger(), "ou", []string{"p1"}, time.Second)

			require.NoError(t, err)
			assert.Contains(t, orders, "p1")
			assert.Nil(t, orders["p1"])
		})
	}
}

func TestA2AEnvironmentOrdersFailOnOtherErrors(t *testing.T) {
	cause := errors.New("503")
	oc := pipelineLookup(nil, cause)

	_, err := a2aEnvironmentOrders(context.Background(), oc, discardLogger(), "ou", []string{"p1"}, time.Second)

	require.ErrorIs(t, err, cause)
	assert.NotErrorIs(t, err, utils.ErrNotFound)
	assert.Contains(t, err.Error(), "order a2a catalogue environments: ")
}

func TestA2AEnvironmentOrdersTimeOut(t *testing.T) {
	const timeout = 50 * time.Millisecond
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(ctx context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	start := time.Now()
	_, err := a2aEnvironmentOrders(context.Background(), oc, discardLogger(), "ou", []string{"p1", "p2"}, timeout)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, timeout+time.Second)
}

func TestA2AEnvironmentOrdersFetchEachProjectOnce(t *testing.T) {
	oc := pipelineLookup(&models.DeploymentPipelineResponse{}, nil)

	orders, err := a2aEnvironmentOrders(context.Background(), oc, discardLogger(), "ou", []string{"p1", "p1", "p2"}, time.Second)
	require.NoError(t, err)
	assert.Len(t, orders, 2)
	assert.Len(t, oc.GetProjectDeploymentPipelineCalls(), 2)

	_, err = a2aEnvironmentOrders(context.Background(), oc, discardLogger(), "ou", []string{"p1", "p1", "p2"}, time.Second)
	require.NoError(t, err)
	assert.Len(t, oc.GetProjectDeploymentPipelineCalls(), 4)
}

func TestA2AEnvironmentOrdersReadPromotionPaths(t *testing.T) {
	pipeline := &models.DeploymentPipelineResponse{
		PromotionPaths: []models.PromotionPath{
			{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "prod"}}},
		},
	}
	oc := pipelineLookup(pipeline, nil)

	orders, err := a2aEnvironmentOrders(context.Background(), oc, discardLogger(), "ou", []string{"p1"}, time.Second)

	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging", "prod"}, orders["p1"])
}
