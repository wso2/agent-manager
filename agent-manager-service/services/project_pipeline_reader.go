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

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ProjectPipelineReader supplies a project's environment order for agent
// identity provisioning in CP_MANAGED mode.
type ProjectPipelineReader interface {
	ProjectPipeline(ctx context.Context, organizationID, projectName string) (source string, environments []ProjectIdentityEnvironment, err error)
}

type ProjectPipelineReaderSetter interface {
	SetProjectPipelineReader(reader ProjectPipelineReader)
}

var errProjectPipelineReaderUnavailable = errors.New("project pipeline reader is not configured")

// defaultEnvironmentPipelineReader reports every project as having only the
// organization's default environment.
type defaultEnvironmentPipelineReader struct{}

// NewDefaultEnvironmentPipelineReader returns the reader for a single,
// implicit environment.
func NewDefaultEnvironmentPipelineReader() ProjectPipelineReader {
	return defaultEnvironmentPipelineReader{}
}

func (defaultEnvironmentPipelineReader) ProjectPipeline(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
	return models.DefaultEnvironmentName, []ProjectIdentityEnvironment{{Name: models.DefaultEnvironmentName}}, nil
}

func (s *agentThunderProvisioningService) SetProjectPipelineReader(reader ProjectPipelineReader) {
	s.pipelineReader = reader
}

var _ ProjectPipelineReaderSetter = (*agentThunderProvisioningService)(nil)

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
