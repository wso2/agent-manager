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

// Package dbtest seeds the rows integration tests build on. Every row hangs off an organization, so each test
// creates its own and the foreign keys keep tests apart. It imports only models and GORM, so any package's tests
// can use it.
package dbtest

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// Org creates an organization with its default environment and returns the organization's id, which is also its
// Thunder OU id.
func Org(t *testing.T, db *gorm.DB) string {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	ou := id.String()
	ts := now()
	require.NoError(t, db.Create(&models.Organization{
		ID: id, Handle: "org-" + ou[len(ou)-12:], DisplayName: "Test org", ThunderOUID: &ou, CreatedAt: ts, UpdatedAt: ts,
	}).Error)
	require.NoError(t, db.Create(&models.Environment{
		ID: uuid.Must(uuid.NewV7()), OrgID: id, Name: models.DefaultEnvironmentName, DisplayName: "Default",
		CreatedAt: ts, UpdatedAt: ts,
	}).Error)
	return ou
}

// Environment returns the id of the organization's default environment.
func Environment(t *testing.T, db *gorm.DB, orgID string) uuid.UUID {
	t.Helper()
	var env models.Environment
	require.NoError(t, db.Where("org_id = ? AND name = ?", orgID, models.DefaultEnvironmentName).Take(&env).Error)
	return env.ID
}

// Project creates a project in the organization and returns its id.
func Project(t *testing.T, db *gorm.DB, orgID, name string) uuid.UUID {
	t.Helper()
	ts := now()
	p := models.Project{
		ID: uuid.Must(uuid.NewV7()), OrgID: uuid.MustParse(orgID), Name: name, DisplayName: name,
		CreatedAt: ts, UpdatedAt: ts,
	}
	require.NoError(t, db.Create(&p).Error)
	return p.ID
}

// Agent creates an external agent in the project and returns its id.
func Agent(t *testing.T, db *gorm.DB, orgID string, projectID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	ts := now()
	a := models.Agent{
		ID: uuid.Must(uuid.NewV7()), OrgID: uuid.MustParse(orgID), ProjectID: projectID, Name: name, DisplayName: name,
		ProvisioningType: "external", CreatedAt: ts, UpdatedAt: ts,
	}
	require.NoError(t, db.Omit("Labels").Create(&a).Error)
	return a.ID
}

// EnvironmentWithID makes sure the organization has an environment with id, creating one named after it if not.
func EnvironmentWithID(t *testing.T, db *gorm.DB, orgID string, id uuid.UUID) {
	t.Helper()
	EnvironmentNamed(t, db, orgID, id, "env-"+id.String()[:8])
}

// EnvironmentNamed makes sure the organization has an environment with id, creating one named name if not.
func EnvironmentNamed(t *testing.T, db *gorm.DB, orgID string, id uuid.UUID, name string) {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Environment{}).Where("id = ?", id).Count(&n).Error)
	if n > 0 {
		return
	}
	ts := now()
	require.NoError(t, db.Create(&models.Environment{
		ID: id, OrgID: uuid.MustParse(orgID), Name: name, DisplayName: name, CreatedAt: ts, UpdatedAt: ts,
	}).Error)
}

// EnsureProject is the id of the organization's project named name, created on first use.
func EnsureProject(t *testing.T, db *gorm.DB, orgID, name string) uuid.UUID {
	t.Helper()
	var p models.Project
	require.NoError(t, db.Where("org_id = ? AND name = ?", orgID, name).Limit(1).Find(&p).Error)
	if p.ID != uuid.Nil {
		return p.ID
	}
	return Project(t, db, orgID, name)
}

// EnsureAgent is the id of the agent named agent in the project, both created on first use.
func EnsureAgent(t *testing.T, db *gorm.DB, orgID, project, agent string) uuid.UUID {
	t.Helper()
	projectID := EnsureProject(t, db, orgID, project)
	var a models.Agent
	require.NoError(t, db.Where("org_id = ? AND name = ?", orgID, agent).Limit(1).Find(&a).Error)
	if a.ID != uuid.Nil {
		return a.ID
	}
	return Agent(t, db, orgID, projectID, agent)
}
