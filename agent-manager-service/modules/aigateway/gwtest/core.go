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

package gwtest

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"gorm.io/gorm"
)

// Core is a modkit.CoreAPI for unit tests. Each lookup calls its func, which
// panics when unset; EnsureOrg succeeds unless EnsureOrgFunc is set. The other
// methods panic.
type Core struct {
	GetAgentFunc         func(ctx context.Context, orgID, projectName, agentName string) (*modkit.Agent, error)
	GetProjectFunc       func(ctx context.Context, orgID, projectName string) (*modkit.Project, error)
	GetEnvironmentFunc   func(ctx context.Context, orgID, envName string) (*modkit.Environment, error)
	ListEnvironmentsFunc func(ctx context.Context, orgID string) ([]*modkit.Environment, error)
	EnsureOrgFunc        func(ctx context.Context, orgID string) (modkit.Org, error)
	GetOrgFunc           func(ctx context.Context, orgID string) (modkit.Org, error)
}

var _ modkit.CoreAPI = (*Core)(nil)

func value[T any](p *T, err error) (T, error) {
	var zero T
	if err != nil || p == nil {
		return zero, err
	}
	return *p, nil
}

// GetAgentByName returns GetAgentFunc's agent, in projectName unless the func
// names another project.
func (c *Core) GetAgentByName(ctx context.Context, orgID, projectName, name string) (modkit.Agent, error) {
	a, err := value(c.GetAgentFunc(ctx, orgID, projectName, name))
	if err == nil && a.ProjectName == "" {
		a.ProjectName = projectName
	}
	return a, err
}

func (c *Core) GetProjectByName(ctx context.Context, orgID, name string) (modkit.Project, error) {
	return value(c.GetProjectFunc(ctx, orgID, name))
}

func (c *Core) GetEnvironmentByName(ctx context.Context, orgID, name string) (modkit.Environment, error) {
	return value(c.GetEnvironmentFunc(ctx, orgID, name))
}

func (c *Core) ListEnvironments(ctx context.Context, orgID string) ([]modkit.Environment, error) {
	envs, err := c.ListEnvironmentsFunc(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]modkit.Environment, 0, len(envs))
	for _, e := range envs {
		out = append(out, *e)
	}
	return out, nil
}

func (c *Core) EnsureOrg(ctx context.Context, orgID string) (modkit.Org, error) {
	if c.EnsureOrgFunc == nil {
		return modkit.Org{ID: orgID}, nil
	}
	return c.EnsureOrgFunc(ctx, orgID)
}

func (c *Core) GetOrg(ctx context.Context, orgID string) (modkit.Org, error) {
	if c.GetOrgFunc == nil {
		panic("gwtest.Core: GetOrg")
	}
	return c.GetOrgFunc(ctx, orgID)
}

func (c *Core) ListProjects(context.Context, string) ([]modkit.Project, error) {
	panic("gwtest.Core: ListProjects")
}

func (c *Core) GetProject(context.Context, string, string) (modkit.Project, error) {
	panic("gwtest.Core: GetProject")
}

func (c *Core) ListAgents(context.Context, string, string) ([]modkit.Agent, error) {
	panic("gwtest.Core: ListAgents")
}

func (c *Core) GetAgent(context.Context, string, string) (modkit.Agent, error) {
	panic("gwtest.Core: GetAgent")
}

func (c *Core) LockAgent(context.Context, *gorm.DB, string, string) (modkit.Agent, error) {
	panic("gwtest.Core: LockAgent")
}

// Merge returns next with the lookups it leaves unset taken from cur, when cur
// is a *Core, so a test can replace some of a service's lookups and keep the
// rest.
func Merge(cur modkit.CoreAPI, next *Core) *Core {
	c, ok := cur.(*Core)
	if !ok || c == nil {
		return next
	}
	m := *next
	if m.GetAgentFunc == nil {
		m.GetAgentFunc = c.GetAgentFunc
	}
	if m.GetProjectFunc == nil {
		m.GetProjectFunc = c.GetProjectFunc
	}
	if m.GetEnvironmentFunc == nil {
		m.GetEnvironmentFunc = c.GetEnvironmentFunc
	}
	if m.ListEnvironmentsFunc == nil {
		m.ListEnvironmentsFunc = c.ListEnvironmentsFunc
	}
	if m.EnsureOrgFunc == nil {
		m.EnsureOrgFunc = c.EnsureOrgFunc
	}
	if m.GetOrgFunc == nil {
		m.GetOrgFunc = c.GetOrgFunc
	}
	return &m
}
