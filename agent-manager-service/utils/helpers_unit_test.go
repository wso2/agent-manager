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

package utils

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/aesgcm"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestConvertAgents(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	agent := &models.AgentResponse{
		UUID: "u1", Name: "a1", DisplayName: "A1", Description: "d", ProjectName: "p", CreatedAt: created, Status: "Active",
		Provisioning: models.Provisioning{Type: "external"}, Type: models.AgentType{Type: "external-agent-api"},
		Labels:    map[string]string{"team": "x"},
		CreatedBy: &models.AgentCreatedBy{ID: "sub-1", Display: "alice"},
	}
	got := ConvertToAgentResponse(agent)
	assert.Equal(t, "u1", got.Uuid)
	assert.Equal(t, "A1", got.DisplayName)
	assert.Equal(t, created, got.CreatedAt)
	require.NotNil(t, got.Status)
	assert.Equal(t, "Active", *got.Status)
	assert.Equal(t, "external-agent-api", got.AgentType.Type)
	require.NotNil(t, got.Labels)
	assert.Equal(t, map[string]string{"team": "x"}, *got.Labels)
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, "sub-1", got.CreatedBy.Id)
	require.NotNil(t, got.CreatedBy.Display)
	assert.Equal(t, "alice", *got.CreatedBy.Display)

	bare := ConvertToAgentResponse(&models.AgentResponse{Name: "b", CreatedBy: &models.AgentCreatedBy{ID: "sub-2"}})
	assert.Nil(t, bare.Labels, "no labels are omitted, not sent empty")
	assert.Nil(t, bare.CreatedBy.Display, "an unresolved creator has only an id")
	assert.Nil(t, ConvertToAgentResponse(&models.AgentResponse{}).CreatedBy)
	assert.Equal(t, spec.AgentResponse{}, ConvertToAgentResponse(nil))
}

func TestConvertEnvironments(t *testing.T) {
	env := &models.EnvironmentResponse{UUID: "e1", Name: "default", DisplayName: "Default", DNSPrefix: "dev", IsProduction: true, DataplaneRef: "dp"}
	got := ConvertToEnvironmentResponse(env)
	assert.Equal(t, "e1", got.Uuid)
	assert.True(t, got.IsProduction)
	require.NotNil(t, got.DisplayName)
	assert.Equal(t, "Default", *got.DisplayName)
	require.NotNil(t, got.DnsPrefix)
	assert.Equal(t, "dev", *got.DnsPrefix)
	assert.Equal(t, spec.Environment{}, ConvertToEnvironmentResponse(nil))

	assert.Len(t, ConvertToEnvironmentListResponse([]*models.EnvironmentResponse{env, env}), 2)
	assert.Equal(t, []spec.Environment{}, ConvertToEnvironmentListResponse(nil))
}

func TestConvertOrganizations(t *testing.T) {
	org := &models.OrganizationResponse{Name: "o", DisplayName: "O", Description: "d", Handle: "acme"}
	got := ConvertToOrganizationResponse(org)
	assert.Equal(t, "d", got.Description)
	require.NotNil(t, got.Handle)
	assert.Equal(t, "acme", *got.Handle)
	assert.Nil(t, ConvertToOrganizationResponse(&models.OrganizationResponse{Name: "o"}).Handle, "no handle is omitted")
	assert.Equal(t, spec.OrganizationResponse{}, ConvertToOrganizationResponse(nil))

	items := ConvertToOrganizationListResponse([]*models.OrganizationResponse{org, nil})
	require.Len(t, items, 2)
	assert.Equal(t, "acme", *items[0].Handle)
	assert.Equal(t, spec.OrganizationListItem{}, items[1])
	assert.Equal(t, []spec.OrganizationListItem{}, ConvertToOrganizationListResponse(nil))
}

func TestAPIKeyHashing(t *testing.T) {
	hash, err := HashAPIKey("key-123")
	require.NoError(t, err)
	assert.NotContains(t, string(hash), "key-123", "only the hash is stored")
	assert.NoError(t, VerifyAPIKey("key-123", hash))
	assert.ErrorIs(t, VerifyAPIKey("key-124", hash), ErrInvalidAPIKey)
	assert.ErrorIs(t, VerifyAPIKey("key-123", []byte("not a hash")), ErrInvalidAPIKey)

	_, err = HashAPIKey(string(make([]byte, 100)))
	assert.Error(t, err, "bcrypt refuses keys longer than 72 bytes")

	token := GenerateAPIKeyToken("key-123")
	back, err := ParseAPIKeyToken(token)
	require.NoError(t, err)
	assert.Equal(t, "key-123", back)
	_, err = ParseAPIKeyToken("%%% not base64")
	assert.Error(t, err)
}

func TestGenerateEncryptionKey(t *testing.T) {
	a, err := GenerateEncryptionKey()
	require.NoError(t, err)
	b, err := GenerateEncryptionKey()
	require.NoError(t, err)
	assert.Len(t, a, aesgcm.KeySize)
	assert.NotEqual(t, a, b)
}

func TestSanitizeForLog(t *testing.T) {
	assert.Equal(t, "line1line2 fake=entry", SanitizeForLog("line1\r\nline2 fake=entry"))
	assert.Equal(t, "nul and [31mred", SanitizeForLog("nul\x00 and \x1b[31mred"))
	assert.Equal(t, "tab\tkept", SanitizeForLog("tab\tkept"))
	assert.Equal(t, "bell", SanitizeForLog("be\x07ll"))
	assert.Equal(t, "naïve ✓", SanitizeForLog("naïve ✓"), "printable unicode is kept")
}

func TestTruncateForLog(t *testing.T) {
	assert.Equal(t, "short", TruncateForLog("short", 10))
	assert.Equal(t, "abc…", TruncateForLog("abcdef", 3))
	assert.Equal(t, "日本…", TruncateForLog("日本語です", 2), "cut on runes, not bytes")
	assert.Empty(t, TruncateForLog("anything", 0))
	assert.Empty(t, TruncateForLog("anything", -1))
}

func TestCalculateAdaptiveGranularity(t *testing.T) {
	for _, tc := range []struct {
		span  time.Duration
		count int64
		want  string
	}{
		{90 * 24 * time.Hour, RawThreshold, "trace"},
		{time.Minute, 0, "trace"},
		{3 * time.Hour, RawThreshold + 1, "minute"},
		{3*time.Hour + time.Second, 100, "hour"},
		{7 * 24 * time.Hour, 100, "hour"},
		{8 * 24 * time.Hour, 100, "day"},
		{28 * 24 * time.Hour, 100, "day"},
		{29 * 24 * time.Hour, 100, "week"},
	} {
		assert.Equal(t, tc.want, CalculateAdaptiveGranularity(tc.span, tc.count), "%s/%d", tc.span, tc.count)
	}
}

func TestTypeHelpers(t *testing.T) {
	s := "v"
	assert.Equal(t, "v", StrPointerAsStr(&s, "d"))
	assert.Equal(t, "d", StrPointerAsStr(nil, "d"))
	assert.Equal(t, "x", *StrAsStrPointer("x"))

	b := false
	assert.False(t, BoolPointerAsBool(&b, true))
	assert.True(t, BoolPointerAsBool(nil, true))
	assert.Equal(t, "true", BoolAsString(true))

	n := int32(7)
	assert.Equal(t, int32(7), IntPointerAsInt(&n, 1))
	assert.Equal(t, int32(1), IntPointerAsInt(nil, 1))
	assert.Equal(t, 3, *IntAsIntPointer(3))

	id := uuid.New()
	got, err := ParseUUID(id.String())
	require.NoError(t, err)
	assert.Equal(t, id, got)
	got, err = ParseUUID("nope")
	assert.EqualError(t, err, `invalid UUID "nope": invalid UUID length: 4`)
	assert.Equal(t, uuid.Nil, got)
}
