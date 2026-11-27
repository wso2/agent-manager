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

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The project routes over the real database: create, list with paging, get,
// update and delete as Project documents, each refusal with its status and
// code, and the organization boundary - the token decides the organization.
func TestCoreProjectsAPI(t *testing.T) {
	w := newCoreWorld(t)

	create := func(name, display string) spec.ProjectDocument {
		t.Helper()
		return decodeAs[spec.ProjectDocument](t, mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, projectsPath(""),
			projectDocument(name, display, stringPtr("about "+name))))
	}

	t.Run("create", func(t *testing.T) {
		p := create("alpha", "Alpha")
		assert.Equal(t, "Project", p.Kind)
		assert.Equal(t, "alpha", p.Metadata.Name)
		assert.Equal(t, "Alpha", p.Spec.DisplayName)
		assert.Equal(t, "about alpha", p.Spec.GetDescription())
		assert.NotEmpty(t, p.Metadata.GetUid())
		assert.False(t, p.Metadata.GetCreationTimestamp().IsZero())
		create("beta", "Beta")
		create("gamma", "Gamma")
	})

	t.Run("create refusals", func(t *testing.T) {
		wrongKind := projectDocument("delta", "Delta", nil)
		wrongKind.Kind = "Agent"
		for _, tc := range []struct {
			name   string
			body   any
			status int
			msg    string
			code   string
		}{
			{"malformed body", "{", http.StatusBadRequest, "Invalid request body", ""},
			{"wrong kind", wrongKind, http.StatusBadRequest, `kind must be "Project" on this route, got "Agent"`, ""},
			{
				"invalid name",
				projectDocument("Not Valid", "x", nil),
				http.StatusBadRequest,
				"Project name must contain only lowercase alphanumeric characters or '-'", "VALIDATION_ERROR",
			},
			{
				"empty display name",
				projectDocument("delta", "", nil),
				http.StatusBadRequest,
				"Project display name cannot be empty", "VALIDATION_ERROR",
			},
			{
				"name taken",
				projectDocument("alpha", "Again", nil),
				http.StatusConflict,
				"Project already exists", utils.ErrCodeProjectAlreadyExists,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := call(t, w.admin, http.MethodPost, projectsPath(""), tc.body)
				require.Equal(t, tc.status, status, string(body))
				e := errorOf(t, body)
				assert.Equal(t, tc.msg, e.Message)
				if tc.code != "" {
					assert.Equal(t, tc.code, e.Code)
				}
			})
		}
	})

	t.Run("list pages", func(t *testing.T) {
		all := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, projectsPath(""), nil))
		assert.Equal(t, int32(3), *all.Pagination.Total)
		assert.Equal(t, int32(utils.DefaultLimit), *all.Pagination.Limit)
		assert.Equal(t, int32(3), all.Count)
		names := make([]string, 0, len(all.List))
		for _, p := range all.List {
			names = append(names, p.Metadata.Name)
		}
		assert.ElementsMatch(t, []string{"alpha", "beta", "gamma"}, names)

		page := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, projectsPath("?limit=2&offset=1"), nil))
		assert.Equal(t, int32(3), *page.Pagination.Total, "the total counts every project, not the page")
		assert.Equal(t, int32(2), *page.Pagination.Limit)
		assert.Equal(t, int32(1), *page.Pagination.Offset)
		assert.Len(t, page.List, 2)

		past := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, projectsPath("?offset=10"), nil))
		assert.Empty(t, past.List)
		assert.Equal(t, int32(3), *past.Pagination.Total)

		for _, q := range []string{"limit=0", "limit=101", "limit=x", "offset=-1"} {
			status, body := call(t, w.admin, http.MethodGet, projectsPath("?%s", q), nil)
			assert.Equal(t, http.StatusBadRequest, status, q)
			assert.Contains(t, errorOf(t, body).Message, "Invalid", q)
		}
	})

	t.Run("get", func(t *testing.T) {
		p := decodeAs[spec.ProjectDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, projectsPath("/alpha"), nil))
		assert.Equal(t, "Alpha", p.Spec.DisplayName)
		assert.Equal(t, "about alpha", p.Spec.GetDescription())

		status, body := call(t, w.reader, http.MethodGet, projectsPath("/missing"), nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, "Project not found", errorOf(t, body).Message)
	})

	t.Run("update", func(t *testing.T) {
		p := decodeAs[spec.ProjectDocument](t, mustCall(t, w.admin, http.StatusOK, http.MethodPut, projectsPath("/beta"),
			projectDocument("beta", "Beta Renamed", stringPtr("new words"))))
		assert.Equal(t, "Beta Renamed", p.Spec.DisplayName)

		got := decodeAs[spec.ProjectDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, projectsPath("/beta"), nil))
		assert.Equal(t, "Beta Renamed", got.Spec.DisplayName, "the change is stored")
		assert.Equal(t, "new words", got.Spec.GetDescription())

		mustCall(t, w.admin, http.StatusOK, http.MethodPut, projectsPath("/beta"), projectDocument("beta", "Beta Again", nil))
		got = decodeAs[spec.ProjectDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, projectsPath("/beta"), nil))
		assert.Equal(t, "new words", got.Spec.GetDescription(), "an absent description is kept")

		status, _ := call(t, w.admin, http.MethodPut, projectsPath("/beta"), "{")
		assert.Equal(t, http.StatusBadRequest, status)
		status, body := call(t, w.admin, http.MethodPut, projectsPath("/beta"), projectDocument("beta", "", nil))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, "Project display name cannot be empty", errorOf(t, body).Message)
		status, body = call(t, w.admin, http.MethodPut, projectsPath("/beta"), projectDocument("alpha", "x", nil))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, "metadata.name does not match the project in the path", errorOf(t, body).Message)
		status, body = call(t, w.admin, http.MethodPut, projectsPath("/missing"), projectDocument("missing", "x", nil))
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorOf(t, body).Code)
	})

	t.Run("a reader cannot change projects", func(t *testing.T) {
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, projectsPath(""), projectDocument("sneaky", "Sneaky", nil)},
			{http.MethodPut, projectsPath("/alpha"), projectDocument("alpha", "Hacked", nil)},
			{http.MethodDelete, projectsPath("/alpha"), nil},
		} {
			status, _ := call(t, w.reader, tc.method, tc.path, tc.body)
			assert.Equal(t, http.StatusForbidden, status, "%s %s", tc.method, tc.path)
		}
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, projectsPath("/sneaky"), nil)
		p := decodeAs[spec.ProjectDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, projectsPath("/alpha"), nil))
		assert.Equal(t, "Alpha", p.Spec.DisplayName, "nothing changed")
	})

	t.Run("another organization's token sees only its own projects", func(t *testing.T) {
		list := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet, projectsPath(""), nil))
		assert.Zero(t, *list.Pagination.Total)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, projectsPath("/alpha"), nil)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodPut, projectsPath("/alpha"), projectDocument("alpha", "Theirs", nil))
		// Deleting is idempotent, so the stranger's delete of a project its own
		// org does not have succeeds without touching this org's.
		mustCall(t, w.stranger, http.StatusNoContent, http.MethodDelete, projectsPath("/alpha"), nil)
		mustCall(t, w.admin, http.StatusOK, http.MethodGet, projectsPath("/alpha"), nil)
	})

	t.Run("delete", func(t *testing.T) {
		mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, projectsPath("/gamma"), nil)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodGet, projectsPath("/gamma"), nil)
		list := decodeAs[spec.ProjectListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, projectsPath(""), nil))
		assert.Equal(t, int32(2), *list.Pagination.Total)
	})
}

// The organization routes: the caller's org under the handle its token
// carries, and the org listing's paging.
func TestCoreOrganizationsAPI(t *testing.T) {
	w := newCoreWorld(t)
	// The organization exists once something has been written to it.
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, projectsPath(""), projectDocument("seed", "Seed", nil))

	org := decodeAs[spec.OrganizationResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/organizations/current"), nil))
	require.NotNil(t, org.Handle)
	assert.Equal(t, w.handle, *org.Handle)

	page := decodeAs[spec.OrganizationListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, "/api/v1/organizations?limit=1", nil))
	assert.Len(t, page.Organizations, 1)
	assert.GreaterOrEqual(t, page.Total, int32(1))
	assert.Equal(t, int32(1), page.Limit)

	for _, q := range []string{"limit=0", "limit=500", "offset=-2", "offset=abc"} {
		status, _ := call(t, w.reader, http.MethodGet, "/api/v1/organizations?"+q, nil)
		assert.Equal(t, http.StatusBadRequest, status, q)
	}

	for _, old := range []string{"/api/v1/orgs", w.orgPath(""), w.orgPath("/identities/roles"), w.orgPath("/events/event-types"), w.orgPath("/thunder-instances"), w.orgPath("/environments")} {
		status, _ := call(t, w.reader, http.MethodGet, old, nil)
		assert.Equal(t, http.StatusNotFound, status, "%s is no longer served", old)
	}
}

// The environment routes list the organization's default environment, get it
// by id, and answer 404 for an id it does not have or another org's.
func TestCoreEnvironmentsAPI(t *testing.T) {
	w := newCoreWorld(t)
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, projectsPath(""), projectDocument("seed", "Seed", nil))

	envs := decodeAs[[]spec.GatewayEnvironmentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/environments"), nil))
	require.Len(t, envs, 1, "a new organization has its default environment")
	env := envs[0]
	assert.Equal(t, "default", env.Name)
	assert.NotEmpty(t, env.Id)

	got := decodeAs[spec.GatewayEnvironmentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/environments/%s", env.Id), nil))
	assert.Equal(t, env.Id, got.Id)
	assert.Equal(t, env.DisplayName, got.DisplayName)

	// AMP Lite has one environment per organization, so the id in the path
	// is not looked up: every id answers the caller's own default
	// environment. Another org's token on the same path gets its own, never
	// this org's.
	byOther := decodeAs[spec.GatewayEnvironmentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet,
		w.path("/environments/00000000-0000-0000-0000-000000000000"), nil))
	assert.Equal(t, env.Id, byOther.Id)
	theirs := decodeAs[spec.GatewayEnvironmentResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet,
		w.path("/environments/%s", env.Id), nil))
	assert.NotEqual(t, env.Id, theirs.Id, "another org never reads this org's environment")

	for _, q := range []string{"limit=0", "limit=101", "offset=-1"} {
		status, _ := call(t, w.reader, http.MethodGet, w.path("/environments?%s", q), nil)
		assert.Equal(t, http.StatusBadRequest, status, q)
	}

	instances := decodeAs[map[string]any](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/thunder-instances"), nil))
	assert.Contains(t, instances, "thunderInstances")
}
