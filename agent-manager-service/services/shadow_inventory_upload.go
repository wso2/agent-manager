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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ShadowInventoryUpload is what an inventory file upload saved: the finished
// scan it created, and what was read from the file.
type ShadowInventoryUpload struct {
	Scan *models.ShadowScan
	// AgentsRead counts the agent rows or objects in the file.
	AgentsRead int
	// AgentsSaved counts the distinct agents saved.
	AgentsSaved int
	// LeftOut counts the rows or objects left out: invalid, or repeating an id.
	LeftOut int
	// Problems are the first problems found in the file, in file order.
	Problems []sa.InventoryProblem
	// ProblemCount counts every problem found in the file, before the cap.
	ProblemCount int
	// IgnoredColumns are the columns or keys not in the format, sorted.
	IgnoredColumns []string
}

// UploadInventory saves the agents of a Bring your own inventory file for
// the connection, as a finished scan of that connection. The file is the
// complete inventory, so the connection's agents that are not in it are
// marked as no longer present. fileName is used only for its extension and
// is never stored or logged. A file that cannot be used fails with
// utils.ErrInvalidInput wrapping an *sa.InventoryFileError and saves nothing; a connection that is not a Bring
// your own inventory connection is not found.
func (s *shadowAgentService) UploadInventory(
	ctx context.Context, ouID string, id uuid.UUID, fileName string, data []byte,
) (*ShadowInventoryUpload, error) {
	c, err := s.loadConnection(ctx, ouID, id)
	if err != nil {
		return nil, err
	}
	if spec, ok := sa.Provider(c.Provider); !ok || !spec.FileUpload {
		return nil, fmt.Errorf("%w: not a Bring your own inventory connection", utils.ErrShadowConnectionNotFound)
	}
	file, err := sa.ParseInventoryFile(fileName, data)
	if err != nil {
		var bad *sa.InventoryFileError
		if errors.As(err, &bad) {
			// The controller shows bad.Message to the user as it is.
			return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, bad)
		}
		return nil, fmt.Errorf("failed to read the inventory file: %w", err)
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	call := models.ShadowScanCallResult{
		Call: sa.InventoryCallName(file.Format), Outcome: models.ShadowCallOK, Items: len(file.Findings),
	}
	if file.LeftOut > 0 {
		call.Message = fmt.Sprintf("%s left out: no id or name, an id longer than 256 characters, "+
			"or an id listed in an earlier row", sa.Plural(file.LeftOut, "agent", "agents"))
	}
	scan := &models.ShadowScan{
		ID:            uuid.New(),
		OUID:          ouID,
		Status:        models.ShadowScanSucceeded,
		ConnectionIDs: []string{c.ID.String()},
		Options:       models.ShadowScanOptions{InventoryUpload: true},
		Results: []models.ShadowScanConnectionResult{{
			ConnectionID: c.ID.String(), ConnectionName: c.Name, Provider: c.Provider,
			Outcome: shadowConnectionOK, Calls: []models.ShadowScanCallResult{call},
			StartedAt: &now, FinishedAt: &now,
		}},
		RequestedBy: audit.BuildEvent(ctx, audit.ActionShadowInventoryUpload).ActorID,
		StartedAt:   &now,
		FinishedAt:  &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	// The file lists the complete inventory: like a complete scan, the
	// connection's agents it does not list are marked as gone.
	apply := toShadowApply(scan, c, &sa.Result{Findings: file.Findings}, now, now, true, s.logger)

	// The upload changes the organization's agents, so it is recorded
	// fail-closed: nothing is saved unrecorded.
	attempt, err := audit.Begin(ctx, audit.ActionShadowInventoryUpload,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceShadowConnection, c.ID.String(), c.Name),
		audit.Detail("provider", c.Provider),
		audit.Detail("scanId", scan.ID.String()),
		audit.Detail("format", file.Format),
		audit.Detail("agentsRead", file.AgentsRead),
		audit.Detail("agents", len(apply.Findings)),
		audit.Detail("leftOut", file.LeftOut),
		audit.Detail("ignoredColumns", len(file.IgnoredColumns)),
	)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.SaveUploadedScan(ctx, scan, apply)
	attempt.Complete(ctx, err)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return nil, utils.ErrShadowConnectionNotFound
		case errors.Is(err, utils.ErrShadowScanSuperseded):
			return nil, err
		default:
			return nil, fmt.Errorf("failed to save the inventory: %w", err)
		}
	}
	// The agents are saved; possible duplicates across connections are
	// recomputed as after a scan, and a failure there is only logged.
	if err := correlateShadowOrg(context.WithoutCancel(ctx), s.repo, ouID, now); err != nil {
		s.logger.Error("Failed to correlate agents across connections", "ouID", ouID, "scanId", scan.ID, "error", err)
	}
	s.logger.Info("Shadow agent inventory file uploaded", "ouID", ouID, "connectionId", c.ID, "scanId", scan.ID,
		"format", file.Format, "agentsRead", file.AgentsRead, "agents", n, "leftOut", file.LeftOut)
	return &ShadowInventoryUpload{
		Scan: scan, AgentsRead: file.AgentsRead, AgentsSaved: n, LeftOut: file.LeftOut,
		Problems: file.Problems, ProblemCount: file.ProblemCount, IgnoredColumns: file.IgnoredColumns,
	}, nil
}
