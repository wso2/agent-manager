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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// inventoryService answers UploadInventory only.
type inventoryService struct {
	services.ShadowAgentService
	err    error
	ouID   string
	name   string
	data   []byte
	called bool
}

func (s *inventoryService) UploadInventory(
	_ context.Context, ouID string, id uuid.UUID, fileName string, data []byte,
) (*services.ShadowInventoryUpload, error) {
	s.ouID, s.name, s.data, s.called = ouID, fileName, data, true
	if s.err != nil {
		return nil, s.err
	}
	return &services.ShadowInventoryUpload{
		Scan: &models.ShadowScan{
			ID: uuid.New(), Status: models.ShadowScanSucceeded, ConnectionIDs: []string{id.String()}, FindingsCount: 1,
			Options: models.ShadowScanOptions{InventoryUpload: true},
		},
		AgentsRead: 2, AgentsSaved: 1, LeftOut: 1, ProblemCount: 2,
		Problems: []sa.InventoryProblem{
			{Row: 3, Message: "No id; the agent was left out.", LeftOut: true},
			{Row: 4, Message: "url \"x\" is not an http or https link; the link was left out."},
		},
	}, nil
}

// multipartBody is a form with a part per field name; "file" parts carry
// the file name.
func multipartBody(t *testing.T, parts map[string][]byte, fileName string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for name, data := range parts {
		var w interface{ Write([]byte) (int, error) }
		var err error
		if name == "file" {
			w, err = mw.CreateFormFile(name, fileName)
		} else {
			w, err = mw.CreateFormField(name)
		}
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())
	return &b, mw.FormDataContentType()
}

func postInventory(svc services.ShadowAgentService, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	id := uuid.NewString()
	r := httptest.NewRequest(http.MethodPost, "/orgs/acme/shadow-agents/connections/"+id+"/inventory", body)
	r.Header.Set("Content-Type", contentType)
	r.SetPathValue(PathParamShadowConnectionID, id)
	r = r.WithContext(middleware.WithResolvedOrg(r.Context(), middleware.ResolvedOrg{OUID: "ou-acme"}))
	w := httptest.NewRecorder()
	NewShadowAgentController(svc).UploadInventory(w, r)
	return w
}

func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	msg, _ := body["message"].(string)
	return msg
}

func TestShadowUploadInventory_ReadsTheFilePart(t *testing.T) {
	svc := &inventoryService{}
	body, ct := multipartBody(t, map[string][]byte{"file": []byte("id,name\na,A\n"), "note": []byte("x")}, "agents.csv")
	w := postInventory(svc, body, ct)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, svc.called)
	assert.Equal(t, "ou-acme", svc.ouID)
	assert.Equal(t, "agents.csv", svc.name)
	assert.Equal(t, "id,name\na,A\n", string(svc.data))

	var resp spec.ShadowInventoryUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, models.ShadowScanSucceeded, resp.Scan.Status)
	assert.Equal(t, int32(2), resp.AgentsRead)
	assert.Equal(t, int32(1), resp.AgentsSaved)
	assert.Equal(t, int32(1), resp.LeftOut)
	assert.Equal(t, int32(2), resp.ProblemCount)
	assert.Equal(t, []spec.ShadowInventoryProblem{
		{Row: 3, Message: "No id; the agent was left out.", LeftOut: true},
		{Row: 4, Message: "url \"x\" is not an http or https link; the link was left out.", LeftOut: false},
	}, resp.Problems)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, []any{}, raw["ignoredColumns"], "an empty list, not null")
	assert.Equal(t, true, raw["scan"].(map[string]any)["inventoryUpload"])
	assert.Equal(t, false, raw["problems"].([]any)[1].(map[string]any)["leftOut"], "always present")
}

// A 5 MiB file passes; one byte more is refused before the service is
// called.
func TestShadowUploadInventory_SizeLimit(t *testing.T) {
	svc := &inventoryService{}
	body, ct := multipartBody(t, map[string][]byte{"file": bytes.Repeat([]byte("a"), 5*1024*1024)}, "a.csv")
	w := postInventory(svc, body, ct)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Len(t, svc.data, 5*1024*1024)

	svc = &inventoryService{}
	body, ct = multipartBody(t, map[string][]byte{"file": bytes.Repeat([]byte("a"), 5*1024*1024+1)}, "a.csv")
	w = postInventory(svc, body, ct)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, errorMessage(t, w), "larger than 5 MB")
	assert.False(t, svc.called)

	svc = &inventoryService{}
	body, ct = multipartBody(t, map[string][]byte{"other": bytes.Repeat([]byte("a"), 6*1024*1024)}, "")
	w = postInventory(svc, body, ct)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, errorMessage(t, w), "larger than 5 MB", "the whole request is bounded")
	assert.False(t, svc.called)
}

func TestShadowUploadInventory_Errors(t *testing.T) {
	body, ct := multipartBody(t, map[string][]byte{"note": []byte("x")}, "")
	w := postInventory(&inventoryService{}, body, ct)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, errorMessage(t, w), "Choose a file")

	w = postInventory(&inventoryService{}, bytes.NewBufferString(`{"agents": []}`), "application/json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, errorMessage(t, w), "multipart/form-data")

	for _, tc := range []struct {
		err  error
		code int
		msg  string
	}{
		{
			fmt.Errorf("%w: %w", utils.ErrInvalidInput, &sa.InventoryFileError{Message: "The file has no id column."}),
			http.StatusBadRequest, "The file has no id column.",
		},
		{utils.ErrShadowConnectionNotFound, http.StatusNotFound, "Connection not found"},
		{
			utils.ErrShadowScanSuperseded, http.StatusConflict,
			"A newer file was uploaded to this source while this one was being saved. " +
				"Upload it again if it is the one you want.",
		},
	} {
		body, ct := multipartBody(t, map[string][]byte{"file": []byte("x")}, "a.csv")
		w := postInventory(&inventoryService{err: tc.err}, body, ct)
		assert.Equal(t, tc.code, w.Code, tc.msg)
		assert.Equal(t, tc.msg, errorMessage(t, w), "the message is shown to the user as it is")
	}
}

func TestShadowInventoryTemplate(t *testing.T) {
	ctrl := NewShadowAgentController(&inventoryService{})
	for format, ct := range map[string]string{
		"json": "application/json", "csv": "text/csv; charset=utf-8",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	} {
		w := httptest.NewRecorder()
		ctrl.GetInventoryTemplate(w, httptest.NewRequest(http.MethodGet,
			"/orgs/acme/shadow-agents/inventory/template?format="+format, nil))
		require.Equal(t, http.StatusOK, w.Code, format)
		assert.Equal(t, ct, w.Header().Get("Content-Type"), format)
		assert.Equal(t, `attachment; filename="agent-inventory-template.`+format+`"`,
			w.Header().Get("Content-Disposition"), format)
		out, err := sa.ParseInventoryFile("template."+format, w.Body.Bytes())
		require.NoError(t, err, format)
		assert.Len(t, out.Findings, 2, format)
	}
	for _, q := range []string{"", "?format=xls", "?format=pdf"} {
		w := httptest.NewRecorder()
		ctrl.GetInventoryTemplate(w, httptest.NewRequest(http.MethodGet, "/orgs/acme/shadow-agents/inventory/template"+q, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}

// The catalog describes Bring your own inventory as a file upload with its
// format, and no API contract.
func TestShadowListProviders_InventoryFormat(t *testing.T) {
	w := httptest.NewRecorder()
	NewShadowAgentController(&inventoryService{}).ListProviders(w, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "apiContract")
	var resp spec.ShadowProviderListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, p := range resp.Providers {
		if p.Provider != models.ShadowProviderCustom {
			assert.Nil(t, p.FileUpload, p.Provider)
			assert.Empty(t, p.InventoryFields, p.Provider)
			continue
		}
		require.NotNil(t, p.FileUpload)
		assert.True(t, *p.FileUpload)
		assert.Empty(t, p.Fields)
		assert.Empty(t, p.Permissions)
		require.Len(t, p.InventoryFields, 14)
		assert.Equal(t, spec.ShadowInventoryField{
			Name: "id", Required: true, Example: "support-bot-prod",
			Description: sa.InventoryFields()[0].Description,
		}, p.InventoryFields[0])
		assert.True(t, strings.HasPrefix(p.Description, "Upload a file"))
	}
}

// A scan says whether an upload created it, always; a connection gives its
// last upload time only when it has one.
func TestShadowInventoryResponseFields(t *testing.T) {
	marshal := func(v any) map[string]any {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, json.Unmarshal(b, &out))
		return out
	}
	scan := marshal(toShadowScanResponse(&models.ShadowScan{ID: uuid.New(), Status: models.ShadowScanRunning}, false))
	assert.Equal(t, false, scan["inventoryUpload"], "present on every scan")

	c := models.ShadowConnection{ID: uuid.New(), Provider: models.ShadowProviderOpenAI}
	assert.NotContains(t, marshal(toShadowConnectionResponse(&services.ShadowConnectionView{Connection: c})),
		"lastUploadAt")
	at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	c.Provider, c.LastUploadAt = models.ShadowProviderCustom, &at
	assert.Equal(t, "2026-11-27T10:00:00Z",
		marshal(toShadowConnectionResponse(&services.ShadowConnectionView{Connection: c}))["lastUploadAt"])
}
