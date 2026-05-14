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
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

func agentEnvAPIArtifactHandle(projectName, agentName, environmentID string) string {
	return fmt.Sprintf("%s/%s/%s", projectName, agentName, environmentID)
}

func ensureAgentEnvAPIArtifact(
	db *gorm.DB,
	artifactRepo repositories.ArtifactRepository,
	orgName, projectName, agentName, environmentID string,
) (*models.Artifact, error) {
	handle := agentEnvAPIArtifactHandle(projectName, agentName, environmentID)
	artifact, err := artifactRepo.GetByHandle(handle, orgName)
	if err == nil {
		if artifact.Kind != models.KindAgent {
			return nil, fmt.Errorf("agent API artifact handle %q exists with kind %q", handle, artifact.Kind)
		}
		return artifact, nil
	}
	if !errors.Is(err, utils.ErrArtifactNotFound) {
		return nil, err
	}

	artifactUUID := uuid.Must(uuid.NewV7())
	artifact = &models.Artifact{
		UUID:             artifactUUID,
		Handle:           handle,
		Name:             fmt.Sprintf("%s-%s-api-%s", agentName, environmentID, artifactUUID.String()[:8]),
		Version:          "v1.0",
		Kind:             models.KindAgent,
		OrganizationName: orgName,
	}
	if err := artifactRepo.Create(db, artifact); err != nil {
		existing, getErr := artifactRepo.GetByHandle(handle, orgName)
		if getErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return artifact, nil
}
