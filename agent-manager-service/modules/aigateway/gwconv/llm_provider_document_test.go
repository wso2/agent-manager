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

package gwconv_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

func provider() *gwmodels.LLMProvider {
	id := uuid.MustParse("9c1b7a44-3e2f-4d8a-b1c5-6a9f0e2d7c81")
	created := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	return &gwmodels.LLMProvider{
		UUID:           id,
		OrgID:          "ou-a",
		Description:    "OpenAI GPT-4 provider for production",
		CreatedBy:      "chamila",
		TemplateHandle: "openai",
		Status:         "CREATED",
		Configuration: gwmodels.LLMProviderConfig{
			Name:     "OpenAI Provider",
			Handle:   "openai-provider",
			Version:  "v1.0",
			Context:  ptr("/openai"),
			Template: "openai",
			Upstream: &gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{URL: "https://api.openai.com/v1"}},
		},
		Artifact: &gwmodels.Artifact{
			UUID:      id,
			Handle:    "openai-provider",
			Name:      "OpenAI Provider",
			Version:   "v1.0",
			CreatedAt: created,
			UpdatedAt: created.Add(time.Hour),
		},
	}
}

func ptr(s string) *string { return &s }

// TestLLMProviderDocumentEnvelope pins the document envelope: the four things
// that make this a v2 payload rather than the v1 one with extra keys.
func TestLLMProviderDocumentEnvelope(t *testing.T) {
	doc, err := gwconv.LLMProviderDocument(provider(), nil)
	require.NoError(t, err)

	assert.Equal(t, "ai-control-plane.choreolabs.com/v1", doc.ApiVersion,
		"the group version tracks the document schema")
	assert.Equal(t, "LlmProvider", doc.Kind)
	assert.Equal(t, "openai-provider", doc.Metadata.Name, "metadata.name is the handle")
	assert.Equal(t, "OpenAI Provider", doc.Spec.DisplayName, "spec.displayName is the label")
}

// TestLLMProviderDocumentRename is the §5.2 rename, which is the whole reason
// the spec body could not simply be reused: v1 puts the handle in `id` and the
// label in `name`, v2 puts the handle in metadata.name and the label in
// spec.displayName. A JSON bridge alone silently loses displayName, so this
// checks the field that the bridge does NOT carry.
func TestLLMProviderDocumentRename(t *testing.T) {
	doc, err := gwconv.LLMProviderDocument(provider(), nil)
	require.NoError(t, err)

	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	var seen map[string]any
	require.NoError(t, json.Unmarshal(raw, &seen))

	spec, ok := seen["spec"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "OpenAI Provider", spec["displayName"])
	assert.NotContains(t, spec, "id", "the handle belongs to metadata.name, not spec")
	assert.NotContains(t, spec, "name", "v1's `name` was renamed to displayName")
	assert.NotContains(t, spec, "uuid", "record fields belong to metadata")
	assert.NotContains(t, spec, "createdAt", "record fields belong to metadata")
}

// TestLLMProviderDocumentMetadata checks the record fields land in metadata.
func TestLLMProviderDocumentMetadata(t *testing.T) {
	doc, err := gwconv.LLMProviderDocument(provider(), nil)
	require.NoError(t, err)

	require.NotNil(t, doc.Metadata.Uid)
	assert.Equal(t, "9c1b7a44-3e2f-4d8a-b1c5-6a9f0e2d7c81", *doc.Metadata.Uid)
	require.NotNil(t, doc.Metadata.CreatedBy)
	assert.Equal(t, "chamila", *doc.Metadata.CreatedBy)
	require.NotNil(t, doc.Metadata.CreationTimestamp)
	require.NotNil(t, doc.Metadata.UpdateTimestamp)
	assert.True(t, doc.Metadata.UpdateTimestamp.After(*doc.Metadata.CreationTimestamp),
		"updateTimestamp tracks spec edits and is distinct from creationTimestamp")
}

// TestLLMProviderDocumentCarriesTheSpecBody checks the JSON bridge actually
// moves the shared fields. Without this the bridge could silently produce an
// empty spec and the envelope tests above would still pass.
func TestLLMProviderDocumentCarriesTheSpecBody(t *testing.T) {
	doc, err := gwconv.LLMProviderDocument(provider(), nil)
	require.NoError(t, err)

	assert.Equal(t, "v1.0", doc.Spec.Version)
	assert.Equal(t, "/openai", doc.Spec.Context)
	assert.Equal(t, "openai", doc.Spec.Template)
	require.NotNil(t, doc.Spec.Upstream.Main)
	require.NotNil(t, doc.Spec.Upstream.Main.Url)
	assert.Equal(t, "https://api.openai.com/v1", *doc.Spec.Upstream.Main.Url)
	require.NotNil(t, doc.Spec.Description)
	assert.Equal(t, "OpenAI GPT-4 provider for production", *doc.Spec.Description)
}

// TestLLMProviderDocumentStatus covers the three states status can be in, and
// the distinction between the first two is the point: nil placement means "not
// reported" and leaves status absent; an empty slice means "reported, placed
// nowhere". A caller that cannot tell those apart shows "not deployed" for a
// provider whose placement simply failed to load.
func TestLLMProviderDocumentStatus(t *testing.T) {
	t.Run("not reported", func(t *testing.T) {
		doc, err := gwconv.LLMProviderDocument(provider(), nil)
		require.NoError(t, err)
		assert.Nil(t, doc.Status, "nil placement leaves status absent")
	})

	t.Run("placed nowhere", func(t *testing.T) {
		doc, err := gwconv.LLMProviderDocument(provider(), []gwmodels.DeploymentSummary{})
		require.NoError(t, err)
		require.NotNil(t, doc.Status, "an empty report is still a report")
		assert.Empty(t, doc.Status.Gateways)
	})

	t.Run("placed, one gateway failed", func(t *testing.T) {
		gwA, gwB := uuid.New(), uuid.New()
		doc, err := gwconv.LLMProviderDocument(provider(), []gwmodels.DeploymentSummary{
			{GatewayID: gwA, GatewayName: "prod-gw", Status: gwmodels.DeploymentStatusDeployed, VHost: "api.example.com"},
			{GatewayID: gwB, GatewayName: "edge-gw", Status: gwmodels.DeploymentStatusFailed},
		})
		require.NoError(t, err)
		require.NotNil(t, doc.Status)
		require.Len(t, doc.Status.Gateways, 2)

		// The thing v1 could not say: WHICH gateway failed. v1 carried one
		// status string for the whole provider plus a list of gateway IDs.
		assert.Equal(t, gwA.String(), doc.Status.Gateways[0].GatewayId)
		assert.Equal(t, "prod-gw", doc.Status.Gateways[0].GatewayName)
		assert.Equal(t, "DEPLOYED", doc.Status.Gateways[0].Status)
		require.NotNil(t, doc.Status.Gateways[0].Vhost)
		assert.Equal(t, "api.example.com", *doc.Status.Gateways[0].Vhost)
		assert.Equal(t, "edge-gw", doc.Status.Gateways[1].GatewayName)
		assert.Equal(t, "FAILED", doc.Status.Gateways[1].Status)
	})
}

// TestLLMProviderListCarriesStatusPerRow: the list view is where a user sees
// which providers are healthy, so a list item without status is a list the
// console cannot render. The placement map is keyed by provider UUID.
func TestLLMProviderListCarriesStatusPerRow(t *testing.T) {
	p := provider()
	gw := uuid.New()
	resp, err := gwconv.LLMProviderList(
		[]*gwmodels.LLMProvider{p},
		map[uuid.UUID][]gwmodels.DeploymentSummary{
			p.UUID: {{GatewayID: gw, GatewayName: "prod-gw", Status: gwmodels.DeploymentStatusDeployed}},
		},
		20, 0, 1)
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	require.NotNil(t, resp.List[0].Status, "a list row carries its own placement")
	require.Len(t, resp.List[0].Status.Gateways, 1)
	assert.Equal(t, "prod-gw", resp.List[0].Status.Gateways[0].GatewayName)
}

// TestLLMProviderDocumentNeedsAnArtifact: metadata.name comes entirely from the
// artifact, which is populated by a join. The v1 converter dereferences it
// unchecked; this one must not, or a row missing its artifact panics inside a
// handler.
func TestLLMProviderDocumentNeedsAnArtifact(t *testing.T) {
	p := provider()
	p.Artifact = nil
	_, err := gwconv.LLMProviderDocument(p, nil)
	require.Error(t, err)

	_, err = gwconv.LLMProviderDocument(nil, nil)
	require.Error(t, err)
}

// TestLLMProviderListCarriesEveryRequiredField is the one that matters for the
// list.
//
// A v2 list item is a DOCUMENT, and LlmProviderSpec requires
// displayName, version, context, template and upstream. If the list query did
// not load them, every list response would violate its own schema and a strict
// generated client would fail to deserialize the whole page - silently for a
// hand-written client, loudly for everyone else.
//
// The proposal listed this as an open item. It is not one: the list query loads
// the full model with its Artifact preloaded, so the fields are there. This
// test is what stops that becoming untrue.
func TestLLMProviderListCarriesEveryRequiredField(t *testing.T) {
	resp, err := gwconv.LLMProviderList([]*gwmodels.LLMProvider{provider()}, nil, 20, 0, 1)
	require.NoError(t, err)
	require.Len(t, resp.List, 1)

	item := resp.List[0]
	assert.NotEmpty(t, item.Spec.DisplayName, "displayName is required")
	assert.NotEmpty(t, item.Spec.Version, "version is required")
	assert.NotEmpty(t, item.Spec.Context, "context is required")
	assert.NotEmpty(t, item.Spec.Template, "template is required")
	require.NotNil(t, item.Spec.Upstream.Main, "upstream is required")

	assert.Equal(t, "ai-control-plane.choreolabs.com/v1", item.ApiVersion, "a list item is a full document, envelope included")
	assert.Equal(t, "LlmProvider", item.Kind)
	assert.Equal(t, "openai-provider", item.Metadata.Name)
}

// TestLLMProviderListOmitsTheHeavyBlock: sparse means the optional blocks a
// list view never renders are left out. openapi is a whole OpenAPI document
// per row.
func TestLLMProviderListOmitsTheHeavyBlock(t *testing.T) {
	p := provider()
	p.OpenAPISpec = "openapi: 3.0.3\ninfo:\n  title: huge\n"

	item, err := gwconv.LLMProviderDocument(p, nil)
	require.NoError(t, err)
	require.NotNil(t, item.Spec.Openapi, "the ITEM endpoint returns it")

	resp, err := gwconv.LLMProviderList([]*gwmodels.LLMProvider{p}, nil, 20, 0, 1)
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Nil(t, resp.List[0].Spec.Openapi, "the LIST omits it")
}

// TestLLMProviderListEnvelope pins the house shape, {count, list, pagination}.
// v1 returned {providers, total, limit, offset} - a different key per
// collection, so no client could write one pager.
func TestLLMProviderListEnvelope(t *testing.T) {
	resp, err := gwconv.LLMProviderList(
		[]*gwmodels.LLMProvider{provider(), provider()}, nil, 20, 40, 97)
	require.NoError(t, err)

	require.NotNil(t, resp.Count)
	assert.Equal(t, int32(2), *resp.Count, "count is this page, not the collection")
	require.NotNil(t, resp.Pagination)
	require.NotNil(t, resp.Pagination.Total)
	assert.Equal(t, int32(97), *resp.Pagination.Total, "total is the collection")
	assert.Equal(t, int32(20), *resp.Pagination.Limit)
	assert.Equal(t, int32(40), *resp.Pagination.Offset)
}

// TestLLMProviderListEmptyIsAnArray: `list` is required, so an empty page must
// serialize as [] and not null, or a client iterating the field crashes on an
// empty collection.
func TestLLMProviderListEmptyIsAnArray(t *testing.T) {
	resp, err := gwconv.LLMProviderList(nil, nil, 20, 0, 0)
	require.NoError(t, err)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"list":[]`, "an empty page is [], never null")
}

// --- POST: reading a document the client sent -------------------------------

func postedDoc() gwspec.LlmProviderDocument {
	return gwspec.LlmProviderDocument{
		ApiVersion: "ai-control-plane.choreolabs.com/v1",
		Kind:       "LlmProvider",
		Metadata:   gwspec.DocumentMetadata{Name: "openai-provider"},
		Spec: gwspec.LlmProviderSpec{
			DisplayName: "OpenAI Provider",
			Version:     "v1.0",
			Context:     "/openai",
			Template:    "openai",
		},
	}
}

// TestFromDocumentMapsTheRename is the §5.2 rename in the inbound direction:
// metadata.name becomes the handle, spec.displayName becomes the label. v1 took
// both from the body as `id` and `name`, which let a provider be created with a
// handle unrelated to its name.
func TestFromDocumentMapsTheRename(t *testing.T) {
	req, err := gwconv.LLMProviderFromDocument(postedDoc())
	require.NoError(t, err)

	assert.Equal(t, "openai-provider", req.Id, "metadata.name is the handle")
	assert.Equal(t, "OpenAI Provider", req.Name, "spec.displayName is the label")
	assert.Equal(t, "v1.0", req.Version)
	assert.Equal(t, "/openai", req.Context)
	assert.Equal(t, "openai", req.Template)
}

// TestFromDocumentChecksApiVersionAndKind. Nothing validates a request body
// against the OpenAPI schema at runtime, so "kind must match the route; never
// ignored" is only true if the handler makes it true. A server that ignores
// kind accepts a Gateway document on the provider route and creates a provider
// from whatever fields line up.
func TestFromDocumentChecksApiVersionAndKind(t *testing.T) {
	t.Run("wrong kind", func(t *testing.T) {
		doc := postedDoc()
		doc.Kind = "Gateway"
		_, err := gwconv.LLMProviderFromDocument(doc)
		require.ErrorIs(t, err, gwconv.ErrDocument)
		assert.Contains(t, err.Error(), "kind", "the message names the field")
	})
	t.Run("wrong apiVersion", func(t *testing.T) {
		doc := postedDoc()
		doc.ApiVersion = "ai-control-plane.choreolabs.com/v2"
		_, err := gwconv.LLMProviderFromDocument(doc)
		require.ErrorIs(t, err, gwconv.ErrDocument)
		assert.Contains(t, err.Error(), "apiVersion")
	})
	t.Run("empty envelope", func(t *testing.T) {
		_, err := gwconv.LLMProviderFromDocument(gwspec.LlmProviderDocument{})
		require.ErrorIs(t, err, gwconv.ErrDocument,
			"an empty body must not be read as a valid document")
	})
}

// TestFromDocumentChecksRequiredMetadata: the service rejects a missing handle
// or name with a generic invalid-input error that names no field. Catching it
// here is the difference between "Invalid input" and a message a caller can act
// on.
func TestFromDocumentChecksRequiredMetadata(t *testing.T) {
	t.Run("no metadata.name", func(t *testing.T) {
		doc := postedDoc()
		doc.Metadata.Name = "   "
		_, err := gwconv.LLMProviderFromDocument(doc)
		require.ErrorIs(t, err, gwconv.ErrDocument)
		assert.Contains(t, err.Error(), "metadata.name")
	})
	t.Run("no spec.displayName", func(t *testing.T) {
		doc := postedDoc()
		doc.Spec.DisplayName = ""
		_, err := gwconv.LLMProviderFromDocument(doc)
		require.ErrorIs(t, err, gwconv.ErrDocument)
		assert.Contains(t, err.Error(), "displayName")
	})
}

// TestFromDocumentIgnoresServerOwnedMetadata: uid and the timestamps are
// readOnly in the schema, and nothing enforces readOnly at runtime. A client
// that sends them is describing a record it did not create; they must not
// reach the create request.
func TestFromDocumentIgnoresServerOwnedMetadata(t *testing.T) {
	doc := postedDoc()
	uid := "11111111-2222-3333-4444-555555555555"
	when := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	by := "someone-else"
	doc.Metadata.Uid = &uid
	doc.Metadata.CreationTimestamp = &when
	doc.Metadata.CreatedBy = &by

	req, err := gwconv.LLMProviderFromDocument(doc)
	require.NoError(t, err)
	assert.Equal(t, "openai-provider", req.Id, "the handle still comes from metadata.name")

	raw, err := json.Marshal(req)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), uid, "a client-sent uid must not reach the create request")
	assert.NotContains(t, string(raw), by, "a client-sent createdBy must not reach the create request")
}

// TestRoundTripThroughTheDocument: what the create route accepts, the read
// route returns. If these two drift, a client cannot read a resource, edit one
// field and post it back — which is the whole point of a document API.
func TestRoundTripThroughTheDocument(t *testing.T) {
	out, err := gwconv.LLMProviderDocument(provider(), nil)
	require.NoError(t, err)

	back, err := gwconv.LLMProviderFromDocument(out)
	require.NoError(t, err)

	assert.Equal(t, "openai-provider", back.Id)
	assert.Equal(t, "OpenAI Provider", back.Name)
	assert.Equal(t, "v1.0", back.Version)
	assert.Equal(t, "/openai", back.Context)
	assert.Equal(t, "openai", back.Template)
	require.NotNil(t, back.Upstream.Main)
	require.NotNil(t, back.Upstream.Main.Url)
	assert.Equal(t, "https://api.openai.com/v1", *back.Upstream.Main.Url,
		"the nested spec body survives the round trip, not just the scalars")
}

// TestRoundTripPreservesPlacement is the trap this avoids: a caller reads a
// provider, edits its description and posts it back. If the read did not report
// the gateways it is placed on, the posted document would declare none — and on
// an update route that reads spec.gateways, that is an undeploy nobody asked
// for. v1's response carried the same list in its `gateways` field.
func TestRoundTripPreservesPlacement(t *testing.T) {
	gwA, gwB := uuid.New(), uuid.New()
	doc, err := gwconv.LLMProviderDocument(provider(), []gwmodels.DeploymentSummary{
		{GatewayID: gwA, GatewayName: "prod-gw", Status: gwmodels.DeploymentStatusDeployed},
		{GatewayID: gwB, GatewayName: "edge-gw", Status: gwmodels.DeploymentStatusDeployed},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{gwA.String(), gwB.String()}, doc.Spec.Gateways,
		"the read reports where it is placed, so posting it back declares the same")

	back, err := gwconv.LLMProviderFromDocument(doc)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{gwA.String(), gwB.String()}, back.Gateways,
		"and the create request carries it through")
}

// TestNotReportedPlacementLeavesGatewaysAlone: when placement was not read, the
// document must not claim the provider is placed nowhere.
func TestNotReportedPlacementLeavesGatewaysAlone(t *testing.T) {
	doc, err := gwconv.LLMProviderDocument(provider(), nil)
	require.NoError(t, err)
	assert.Nil(t, doc.Status)
	assert.Empty(t, doc.Spec.Gateways)
}

// TestSecuritySurvivesTheBridge. security decides whether a provider and its
// proxies require a credential. The v2 schema was generated from a snapshot of
// the core's spec taken before these resources moved into the module, and that
// snapshot had no security property — so a v2 create could not set it and a v2
// read could not report it, silently. The generator now reconciles against the
// module's own document; this is what stops it regressing.
func TestSecuritySurvivesTheBridge(t *testing.T) {
	p := provider()
	p.Configuration.Security = &gwmodels.SecurityConfig{}

	doc, err := gwconv.LLMProviderDocument(p, nil)
	require.NoError(t, err)
	assert.NotNil(t, doc.Spec.Security, "a read must report the provider's security configuration")

	back, err := gwconv.LLMProviderFromDocument(doc)
	require.NoError(t, err)
	assert.NotNil(t, back.Security, "and a create must be able to set it")
}

// updateBody builds a PUT body with the given spec keys.
func updateBody(t *testing.T, spec map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"apiVersion": gwconv.GroupVersion,
		"kind":       gwconv.KindLlmProvider,
		"metadata":   map[string]any{"name": "openai"},
		"spec":       spec,
	})
	require.NoError(t, err)
	return raw
}

func minimalUpdateSpec() map[string]any {
	return map[string]any{
		"displayName": "OpenAI",
		"version":     "v1",
		"context":     "/openai",
		"template":    "openai",
		"upstream":    map[string]any{"main": map[string]any{"url": "https://api.openai.com"}},
	}
}

// An omitted spec.gateways must preserve placement; a declared [] must undeploy. The struct cannot
// tell these apart, because the field is omitempty - which is the bug this guards.
func TestUpdateFromDocumentKeepsGatewaysOmittedApartFromEmpty(t *testing.T) {
	spec := minimalUpdateSpec()
	_, req, err := gwconv.LLMProviderUpdateFromDocument(updateBody(t, spec))
	require.NoError(t, err)
	assert.Nil(t, req.Gateways, "an omitted spec.gateways must preserve placement, not undeploy")

	spec["gateways"] = []string{}
	_, req, err = gwconv.LLMProviderUpdateFromDocument(updateBody(t, spec))
	require.NoError(t, err)
	require.NotNil(t, req.Gateways, "a declared empty spec.gateways is an undeploy and must reach the service")
	assert.Empty(t, req.Gateways)

	spec["gateways"] = []string{"gw-1", "gw-2"}
	_, req, err = gwconv.LLMProviderUpdateFromDocument(updateBody(t, spec))
	require.NoError(t, err)
	assert.Equal(t, []string{"gw-1", "gw-2"}, req.Gateways)
}

// An omitted optional block stays nil so the shared update path preserves it. Clearing security
// would leave the provider and its proxies requiring no credential.
func TestUpdateFromDocumentLeavesOmittedBlocksNil(t *testing.T) {
	_, req, err := gwconv.LLMProviderUpdateFromDocument(updateBody(t, minimalUpdateSpec()))
	require.NoError(t, err)

	assert.Nil(t, req.Security, "omitted spec.security must be preserved, not cleared")
	assert.Nil(t, req.RateLimiting)
	assert.Nil(t, req.AccessControl)
	assert.Nil(t, req.Resilience)
	assert.Nil(t, req.Policies)
	assert.Nil(t, req.ModelProviders)
	assert.Nil(t, req.Openapi)
}

func TestUpdateFromDocumentCarriesDisplayNameAndDeclaredFields(t *testing.T) {
	spec := minimalUpdateSpec()
	spec["description"] = "the one we use"
	_, req, err := gwconv.LLMProviderUpdateFromDocument(updateBody(t, spec))
	require.NoError(t, err)

	require.NotNil(t, req.Name)
	assert.Equal(t, "OpenAI", *req.Name, "spec.displayName is the label v1 calls name")
	require.NotNil(t, req.Version)
	assert.Equal(t, "v1", *req.Version)
	require.NotNil(t, req.Context)
	assert.Equal(t, "/openai", *req.Context)
	require.NotNil(t, req.Template)
	assert.Equal(t, "openai", *req.Template)
	require.NotNil(t, req.Description)
	assert.Equal(t, "the one we use", *req.Description)
	require.NotNil(t, req.Upstream)
}

func TestUpdateFromDocumentRefusesABadDocument(t *testing.T) {
	cases := map[string]map[string]any{
		"wrong apiVersion": {
			"apiVersion": "v1", "kind": gwconv.KindLlmProvider,
			"metadata": map[string]any{"name": "openai"}, "spec": minimalUpdateSpec(),
		},
		"wrong kind": {
			"apiVersion": gwconv.GroupVersion, "kind": "Gateway",
			"metadata": map[string]any{"name": "openai"}, "spec": minimalUpdateSpec(),
		},
		"no metadata.name": {
			"apiVersion": gwconv.GroupVersion, "kind": gwconv.KindLlmProvider,
			"metadata": map[string]any{}, "spec": minimalUpdateSpec(),
		},
		"no spec.displayName": {
			"apiVersion": gwconv.GroupVersion, "kind": gwconv.KindLlmProvider,
			"metadata": map[string]any{"name": "openai"}, "spec": map[string]any{"version": "v1"},
		},
		"no spec": {
			"apiVersion": gwconv.GroupVersion, "kind": gwconv.KindLlmProvider,
			"metadata": map[string]any{"name": "openai"},
		},
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(doc)
			require.NoError(t, err)
			_, _, err = gwconv.LLMProviderUpdateFromDocument(raw)
			require.Error(t, err)
			assert.ErrorIs(t, err, gwconv.ErrDocument)
		})
	}
}
