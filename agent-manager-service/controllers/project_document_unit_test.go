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

package controllers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// projectDoc builds a Project document body with the given name and raw spec.
func projectDoc(name, rawSpec string) string {
	return `{"apiVersion":"` + documentGroupVersion + `","kind":"Project","metadata":{"name":"` + name + `"},"spec":` + rawSpec + `}`
}

func TestProjectDocument_Renders(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	doc := projectDocument(&models.ProjectResponse{UUID: "u1", Name: "shop", DisplayName: "Shop", Description: "d", CreatedAt: created})
	assert.Equal(t, kindProject, doc.Kind)
	assert.Equal(t, "shop", doc.Metadata.Name)
	assert.Equal(t, "u1", doc.Metadata.GetUid())
	assert.Equal(t, created, doc.Metadata.GetCreationTimestamp())
	assert.Equal(t, "d", doc.Spec.GetDescription())

	bare := projectDocument(&models.ProjectResponse{Name: "p"})
	assert.Nil(t, bare.Metadata.Uid)
	assert.Nil(t, bare.Metadata.CreationTimestamp)
	assert.Nil(t, bare.Spec.Description)

	list := projectList([]*models.ProjectResponse{{Name: "a"}, nil}, 10, 0, 1)
	assert.Equal(t, int32(1), list.Count, "a nil project is skipped")
	assert.Equal(t, int32(1), *list.Pagination.Total)
}

func TestProjectDocument_DecodeRejects(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"not json":    {`{`, "malformed body"},
		"wrong type":  {`{"apiVersion":1}`, "invalid document"},
		"api version": {`{"apiVersion":"x/v1","kind":"Project","metadata":{"name":"p"},"spec":{}}`, "apiVersion must be"},
		"kind":        {`{"apiVersion":"` + documentGroupVersion + `","kind":"Agent","metadata":{"name":"p"},"spec":{}}`, "kind must be"},
		"no name":     {projectDoc(" ", `{"displayName":"P"}`), "metadata.name is required"},
		"no spec":     {`{"apiVersion":"` + documentGroupVersion + `","kind":"Project","metadata":{"name":"p"}}`, "spec is required"},
		"null spec":   {projectDoc("p", `null`), "spec is required"},
	} {
		_, err := projectCreateFromDocument([]byte(tc.body))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.want, name)
	}
}

func TestProjectDocument_CreateAndUpdate(t *testing.T) {
	req, err := projectCreateFromDocument([]byte(projectDoc("shop", `{"displayName":"Shop","description":"d"}`)))
	require.NoError(t, err)
	assert.Equal(t, spec.CreateProjectRequest{Name: "shop", DisplayName: "Shop", Description: req.Description}, req)
	assert.Equal(t, "d", *req.Description)

	upd, err := projectUpdateFromDocument([]byte(projectDoc("shop", `{"displayName":"New"}`)), "shop")
	require.NoError(t, err)
	assert.Equal(t, spec.UpdateProjectRequest{DisplayName: "New"}, upd, "an absent description is left for the store to keep")

	upd, err = projectUpdateFromDocument([]byte(projectDoc("shop", `{"displayName":"New","description":""}`)), "shop")
	require.NoError(t, err)
	require.NotNil(t, upd.Description, "a sent description replaces the stored one")
	assert.Empty(t, *upd.Description)

	_, err = projectUpdateFromDocument([]byte(projectDoc("other", `{"displayName":"New"}`)), "shop")
	require.ErrorContains(t, err, "metadata.name does not match")

	_, err = projectUpdateFromDocument([]byte(`{`), "shop")
	require.ErrorIs(t, err, errMalformedBody)
}

// storedProjects echoes creates and updates.
type storedProjects struct {
	stubProjects
}

func (s *storedProjects) CreateProject(_ context.Context, _ string, p spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	return &models.ProjectResponse{Name: p.Name, DisplayName: p.DisplayName}, nil
}

func (s *storedProjects) UpdateProject(_ context.Context, _, name string, p spec.UpdateProjectRequest) (*models.ProjectResponse, error) {
	return &models.ProjectResponse{Name: name, DisplayName: p.DisplayName, Description: utils.StrPointerAsStr(p.Description, "")}, nil
}

func TestInfraResourceController_ProjectDocumentBodies(t *testing.T) {
	ctrl := NewInfraResourceController(&storedProjects{}, nil)
	for name, tc := range map[string]struct {
		h    http.HandlerFunc
		body string
		code int
	}{
		"create bad document": {ctrl.CreateProject, `{`, http.StatusBadRequest},
		"create bad name":     {ctrl.CreateProject, projectDoc("Bad Name", `{"displayName":"P"}`), http.StatusBadRequest},
		"create no label":     {ctrl.CreateProject, projectDoc("p", `{}`), http.StatusBadRequest},
		"create":              {ctrl.CreateProject, projectDoc("p", `{"displayName":"P"}`), http.StatusAccepted},
		"update bad document": {ctrl.UpdateProject, projectDoc("other", `{"displayName":"P"}`), http.StatusBadRequest},
		"update no label":     {ctrl.UpdateProject, projectDoc("proj", `{}`), http.StatusBadRequest},
		"update":              {ctrl.UpdateProject, projectDoc("proj", `{"displayName":"Q"}`), http.StatusOK},
	} {
		w := httptest.NewRecorder()
		tc.h(w, orgRequest(http.MethodPost, "/x", tc.body))
		assert.Equal(t, tc.code, w.Code, "%s: %s", name, w.Body.String())
	}
}

func TestInfraResourceController_ProjectBodyReadFailure(t *testing.T) {
	ctrl := NewInfraResourceController(&storedProjects{}, nil)
	for name, h := range map[string]http.HandlerFunc{"create": ctrl.CreateProject, "update": ctrl.UpdateProject} {
		req := orgRequest(http.MethodPost, "/x", "")
		req.Body = io.NopCloser(failingReader{})
		w := httptest.NewRecorder()
		h(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, name)
	}
}
