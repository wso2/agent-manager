//go:build integration

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
package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

func TestCoreReadAPIReads(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	api := NewCoreReadAPI(gdb)
	ou := newOrg(t)
	a1 := ensureAgent(t, ou, "zeta", "bravo")
	a2 := ensureAgent(t, ou, "alpha", "alpha-agent")
	envOf(t, ou, "staging")

	org, err := api.GetOrg(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, ou, org.ID)

	projects, err := api.ListProjects(ctx, ou)
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, "alpha", projects[0].Name, "ordered by name")

	p, err := api.GetProject(ctx, ou, projects[1].ID)
	require.NoError(t, err)
	assert.Equal(t, "zeta", p.Name)
	byName, err := api.GetProjectByName(ctx, ou, "zeta")
	require.NoError(t, err)
	assert.Equal(t, p.ID, byName.ID)

	agents, err := api.ListAgents(ctx, ou, "")
	require.NoError(t, err)
	require.Len(t, agents, 2)
	assert.Equal(t, a2.String(), agents[0].ID, "ordered by name")
	assert.Equal(t, "alpha", agents[0].ProjectName, "the project name is joined in")

	inZeta, err := api.ListAgents(ctx, ou, p.ID)
	require.NoError(t, err)
	require.Len(t, inZeta, 1)
	assert.Equal(t, "bravo", inZeta[0].Name)

	got, err := api.GetAgent(ctx, ou, a1.String())
	require.NoError(t, err)
	assert.Equal(t, "zeta", got.ProjectName)
	got, err = api.GetAgentByName(ctx, ou, "zeta", "bravo")
	require.NoError(t, err)
	assert.Equal(t, a1.String(), got.ID)
	_, err = api.GetAgentByName(ctx, ou, "alpha", "bravo")
	require.ErrorIs(t, err, modkit.ErrNotFound, "the agent is not in that project")

	envs, err := api.ListEnvironments(ctx, ou)
	require.NoError(t, err)
	require.Len(t, envs, 2)
	assert.Equal(t, models.DefaultEnvironmentName, envs[0].Name)
	env, err := api.GetEnvironmentByName(ctx, ou, "staging")
	require.NoError(t, err)
	assert.Equal(t, "staging", env.Name)
}

// Every read is scoped: another org's ids, a context bound to another org, a malformed id, or a missing row all read
// as not found (or an empty list), never as an error a module could tell apart.
func TestCoreReadAPIScoping(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	api := NewCoreReadAPI(gdb)
	ou, other := newOrg(t), newOrg(t)
	agent := ensureAgent(t, ou, "p", "a")
	var project models.Project
	require.NoError(t, gdb.Where("org_id = ? AND name = ?", ou, "p").First(&project).Error)
	missing := uuid.New().String()

	notFound := map[string]func(context.Context) error{
		"GetOrg":           func(c context.Context) error { _, err := api.GetOrg(c, ou); return err },
		"GetProject":       func(c context.Context) error { _, err := api.GetProject(c, ou, project.ID.String()); return err },
		"GetAgent":         func(c context.Context) error { _, err := api.GetAgent(c, ou, agent.String()); return err },
		"GetProjectByName": func(c context.Context) error { _, err := api.GetProjectByName(c, ou, "p"); return err },
		"GetAgentByName":   func(c context.Context) error { _, err := api.GetAgentByName(c, ou, "p", "a"); return err },
		"GetEnvironmentByName": func(c context.Context) error {
			_, err := api.GetEnvironmentByName(c, ou, models.DefaultEnvironmentName)
			return err
		},
		"EnsureOrg": func(c context.Context) error { _, err := api.EnsureOrg(c, ou); return err },
	}
	foreignCtx := modkit.WithOrgID(ctx, other)
	for name, call := range notFound {
		require.NoError(t, call(modkit.WithOrgID(ctx, ou)), "%s in its own org", name)
		require.ErrorIs(t, call(foreignCtx), modkit.ErrNotFound, "%s from another org's context", name)
	}

	// Ids of another org, malformed ids and missing rows.
	_, err := api.GetProject(ctx, other, project.ID.String())
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetAgent(ctx, other, agent.String())
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetAgent(ctx, ou, "not-a-uuid")
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetOrg(ctx, missing)
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetOrg(ctx, "bad")
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetProject(ctx, ou, missing)
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetProjectByName(ctx, "bad", "p")
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetAgentByName(ctx, "bad", "p", "a")
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetEnvironmentByName(ctx, ou, "nope")
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.GetEnvironmentByName(ctx, "bad", "x")
	require.ErrorIs(t, err, modkit.ErrNotFound)
	_, err = api.EnsureOrg(ctx, "bad")
	require.ErrorIs(t, err, modkit.ErrNotFound)

	empty := map[string]func(context.Context, string) (int, error){
		"ListProjects": func(c context.Context, o string) (int, error) { r, err := api.ListProjects(c, o); return len(r), err },
		"ListAgents":   func(c context.Context, o string) (int, error) { r, err := api.ListAgents(c, o, ""); return len(r), err },
		"ListEnvironments": func(c context.Context, o string) (int, error) {
			r, err := api.ListEnvironments(c, o)
			return len(r), err
		},
	}
	for name, list := range empty {
		n, err := list(ctx, ou)
		require.NoError(t, err)
		assert.Positive(t, n, name)
		for _, c := range []struct {
			ctx context.Context
			org string
		}{{foreignCtx, ou}, {ctx, "bad"}, {ctx, other}} {
			n, err := list(c.ctx, c.org)
			require.NoError(t, err, name)
			if c.org != other || name != "ListEnvironments" {
				assert.Zero(t, n, "%s(%s)", name, c.org)
			}
		}
	}
	agents, err := api.ListAgents(ctx, ou, "bad")
	require.NoError(t, err)
	assert.Empty(t, agents, "a malformed project id lists nothing")
}

func TestCoreReadAPIEnsureOrg(t *testing.T) {
	ctx := context.Background()
	api := NewCoreReadAPI(db.GetDB())
	ou := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteOrg(t, ou) })

	org, err := api.EnsureOrg(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, ou, org.ID)
	envs, err := api.ListEnvironments(ctx, ou)
	require.NoError(t, err)
	assert.Len(t, envs, 1, "the default environment comes with it")
	again, err := api.EnsureOrg(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, org.CreatedAt.Unix(), again.CreatedAt.Unix(), "the second call finds the first's row")
}

func TestCoreReadAPILockAgent(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	api := NewCoreReadAPI(gdb)
	ou, other := newOrg(t), newOrg(t)
	agent := ensureAgent(t, ou, "locks", "locked")

	_, err := api.LockAgent(ctx, nil, ou, agent.String())
	require.ErrorContains(t, err, "needs a transaction")

	err = gdb.Transaction(func(tx *gorm.DB) error {
		got, err := api.LockAgent(ctx, tx, ou, agent.String())
		require.NoError(t, err)
		assert.Equal(t, "locks", got.ProjectName)

		_, err = api.LockAgent(ctx, tx, other, agent.String())
		require.ErrorIs(t, err, modkit.ErrNotFound, "another org's agent")
		_, err = api.LockAgent(ctx, tx, ou, uuid.New().String())
		require.ErrorIs(t, err, modkit.ErrNotFound)
		_, err = api.LockAgent(ctx, tx, ou, "bad")
		require.ErrorIs(t, err, modkit.ErrNotFound)
		_, err = api.LockAgent(modkit.WithOrgID(ctx, other), tx, ou, agent.String())
		require.ErrorIs(t, err, modkit.ErrNotFound)
		return errors.New("rollback")
	})
	require.EqualError(t, err, "rollback")
}
