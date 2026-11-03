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

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ProjectPipelineReader reports a project's environment order: its source
// (first) environment and every environment with its outgoing promotion edges.
// Today the order comes from the OpenChoreo project deployment pipeline, the
// same source PromoteAgent uses. A deployment whose identity platform maps
// projects itself can supply another implementation without any other change.
type ProjectPipelineReader interface {
	ProjectPipeline(ctx context.Context, organizationID, projectName string) (source string, environments []ProjectIdentityEnvironment, err error)
}

// ProjectPipelineReaderSetter is an optional hook so tests and deployments can
// override the reader DBBackedAgentThunderProvisioning builds.
type ProjectPipelineReaderSetter interface {
	SetProjectPipelineReader(reader ProjectPipelineReader)
}

var errProjectPipelineReaderUnavailable = errors.New("project pipeline reader is not configured")

type openChoreoProjectPipelineReader struct {
	oc client.OpenChoreoClient
}

// NewOpenChoreoProjectPipelineReader reads project pipelines from OpenChoreo.
func NewOpenChoreoProjectPipelineReader(oc client.OpenChoreoClient) ProjectPipelineReader {
	return &openChoreoProjectPipelineReader{oc: oc}
}

func (r *openChoreoProjectPipelineReader) ProjectPipeline(
	ctx context.Context, organizationID, projectName string,
) (string, []ProjectIdentityEnvironment, error) {
	pipeline, err := r.oc.GetProjectDeploymentPipeline(ctx, organizationID, projectName)
	if err != nil {
		return "", nil, fmt.Errorf("get project deployment pipeline: %w", err)
	}
	if pipeline == nil {
		return "", nil, fmt.Errorf("project %s has no deployment pipeline", projectName)
	}
	source, environments := pipelineTopology(pipeline.PromotionPaths)
	if source == "" {
		return "", nil, fmt.Errorf("project %s deployment pipeline has no source environment", projectName)
	}
	return source, environments, nil
}

// pipelineTopology converts promotion paths into the first environment and the
// environment list, in first-seen order, merging duplicate paths of one source.
func pipelineTopology(paths []models.PromotionPath) (string, []ProjectIdentityEnvironment) {
	index := make(map[string]int)
	var environments []ProjectIdentityEnvironment
	add := func(name string) int {
		if i, ok := index[name]; ok {
			return i
		}
		index[name] = len(environments)
		environments = append(environments, ProjectIdentityEnvironment{Name: name})
		return index[name]
	}
	for _, path := range paths {
		from := add(path.SourceEnvironmentRef)
		for _, target := range path.TargetEnvironmentRefs {
			add(target.Name)
			environments[from].PromotesTo = append(environments[from].PromotesTo, target.Name)
		}
	}
	return findLowestEnvironment(paths), environments
}

// SetProjectPipelineReader replaces the project pipeline reader.
func (s *agentThunderProvisioningService) SetProjectPipelineReader(reader ProjectPipelineReader) {
	s.pipelineReader = reader
}

var _ ProjectPipelineReaderSetter = (*agentThunderProvisioningService)(nil)

// projectSourceEnvironment returns the project's authoritative source
// environment from the pipeline reader.
func (s *agentThunderProvisioningService) projectSourceEnvironment(
	ctx context.Context, organizationID, projectName string,
) (string, error) {
	if s.pipelineReader == nil {
		return "", errProjectPipelineReaderUnavailable
	}
	source, _, err := s.pipelineReader.ProjectPipeline(ctx, organizationID, projectName)
	if err != nil {
		return "", fmt.Errorf("resolve project identity source environment: %w", err)
	}
	if source == "" {
		return "", errors.New("resolve project identity source environment: empty source environment")
	}
	return source, nil
}
