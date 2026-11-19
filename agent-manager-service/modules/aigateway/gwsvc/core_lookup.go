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

package gwsvc

import (
	"context"
	"errors"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// LookupProject returns the organization's project by name, or
// gwerr.ErrProjectNotFound.
func LookupProject(ctx context.Context, core modkit.CoreAPI, orgID, projectName string) (*modkit.Project, error) {
	p, err := core.GetProjectByName(ctx, orgID, projectName)
	if errors.Is(err, modkit.ErrNotFound) {
		return nil, gwerr.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// LookupAgent returns the project's agent by name, or gwerr.ErrProjectNotFound
// or gwerr.ErrAgentNotFound for whichever of the two does not exist.
func LookupAgent(ctx context.Context, core modkit.CoreAPI, orgID, projectName, agentName string) (*modkit.Agent, error) {
	a, err := core.GetAgentByName(ctx, orgID, projectName, agentName)
	if errors.Is(err, modkit.ErrNotFound) {
		if _, perr := LookupProject(ctx, core, orgID, projectName); perr != nil {
			return nil, perr
		}
		return nil, gwerr.ErrAgentNotFound
	}
	if err != nil {
		return nil, err
	}
	if a.ProjectName != projectName {
		return nil, gwerr.ErrAgentNotFound
	}
	return &a, nil
}

// ensureOrg creates the organization, with its default environment, on its
// first use, as every core lookup of an environment did before the module.
func ensureOrg(ctx context.Context, core modkit.CoreAPI, orgID string) error {
	_, err := core.EnsureOrg(ctx, orgID)
	if errors.Is(err, modkit.ErrNotFound) {
		return gwerr.ErrOrganizationNotFound
	}
	return err
}

// LookupEnvironment returns the organization's environment by name (the
// default one when envName is empty), or gwerr.ErrEnvironmentNotFound.
func LookupEnvironment(ctx context.Context, core modkit.CoreAPI, orgID, envName string) (*modkit.Environment, error) {
	if err := ensureOrg(ctx, core, orgID); err != nil {
		return nil, err
	}
	if envName == "" {
		envName = gwmodels.DefaultEnvironmentName
	}
	e, err := core.GetEnvironmentByName(ctx, orgID, envName)
	if errors.Is(err, modkit.ErrNotFound) {
		return nil, gwerr.ErrEnvironmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListEnvironments returns the organization's environments.
func ListEnvironments(ctx context.Context, core modkit.CoreAPI, orgID string) ([]*modkit.Environment, error) {
	if err := ensureOrg(ctx, core, orgID); err != nil {
		return nil, err
	}
	envs, err := core.ListEnvironments(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]*modkit.Environment, 0, len(envs))
	for i := range envs {
		out = append(out, &envs[i])
	}
	return out, nil
}
