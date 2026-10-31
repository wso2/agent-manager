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
	"slices"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

func agentEnvAPIArtifactHandle(projectName, agentName, environmentID string) string {
	return agentAPIArtifactHandlePrefix(projectName, agentName) + environmentID
}

func agentAPIArtifactHandlePrefix(projectName, agentName string) string {
	return fmt.Sprintf("%s/%s/", projectName, agentName)
}

// agentEnvAPIArtifact reads the agent's API artifact for the environment; a missing row, or one whose kind is not
// in kinds, is utils.ErrArtifactNotFound.
func agentEnvAPIArtifact(
	artifactRepo repositories.ArtifactRepository, ouID, projectName, agentName, environmentID string, kinds ...string,
) (*models.Artifact, error) {
	artifact, err := artifactRepo.GetByHandle(agentEnvAPIArtifactHandle(projectName, agentName, environmentID), ouID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(kinds, artifact.Kind) {
		return nil, utils.ErrArtifactNotFound
	}
	return artifact, nil
}

// ensureAgentEnvAPIArtifact reads or creates the agent's API artifact of the given kind for the environment through
// db, which may be the caller's transaction. An existing artifact of another kind is an error.
func ensureAgentEnvAPIArtifact(
	db *gorm.DB,
	artifactRepo repositories.ArtifactRepository,
	ouID, projectName, agentName, environmentID, kind string,
) (*models.Artifact, error) {
	artifactRepo = artifactRepo.WithTx(db)
	handle := agentEnvAPIArtifactHandle(projectName, agentName, environmentID)
	artifact, err := artifactRepo.GetByHandle(handle, ouID)
	if err == nil {
		return requireArtifactKind(artifact, kind)
	}
	if !errors.Is(err, utils.ErrArtifactNotFound) {
		return nil, err
	}

	artifactUUID := uuid.Must(uuid.NewV7())
	artifact = &models.Artifact{
		UUID:    artifactUUID,
		Handle:  handle,
		Name:    fmt.Sprintf("%s-%s-api-%s", agentName, environmentID, artifactUUID.String()[:8]),
		Version: "v1.0",
		Kind:    kind,
		OUID:    ouID,
	}
	// A savepoint inside a caller's transaction, so losing the insert race does not abort it.
	err = db.Transaction(func(tx *gorm.DB) error {
		return artifactRepo.Create(tx, artifact)
	})
	if err != nil {
		existing, getErr := artifactRepo.GetByHandle(handle, ouID)
		if getErr == nil {
			return requireArtifactKind(existing, kind)
		}
		return nil, err
	}
	return artifact, nil
}

func requireArtifactKind(artifact *models.Artifact, kind string) (*models.Artifact, error) {
	if artifact.Kind != kind {
		return nil, fmt.Errorf("agent API artifact handle %q exists with kind %q, not %q", artifact.Handle, artifact.Kind, kind)
	}
	return artifact, nil
}
