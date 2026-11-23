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

package gwconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// GroupVersion is the document schema version.
const GroupVersion = "ai-control-plane.choreolabs.com/v1"

// KindLlmProvider is the kind of an LLM provider document.
const KindLlmProvider = "LlmProvider"

// LLMProviderDocument renders a provider as its document.
func LLMProviderDocument(
	model *gwmodels.LLMProvider, placement []gwmodels.DeploymentSummary,
) (gwspec.LlmProviderDocument, error) {
	// metadata.name comes entirely from the joined artifact, so a row without one cannot be named.
	if model == nil || model.Artifact == nil {
		return gwspec.LlmProviderDocument{}, errors.New("provider has no artifact: cannot name the document")
	}

	// Bridged through JSON: both sides are generated from the same schemas, so a hand-written
	// mapping of the nested types would be chances to drop a field silently. What it cannot
	// carry is set explicitly below.
	v1 := ConvertModelToSpecLLMProviderResponse(model)
	raw, err := json.Marshal(v1)
	if err != nil {
		return gwspec.LlmProviderDocument{}, fmt.Errorf("marshal provider %s: %w", model.UUID, err)
	}
	var spec gwspec.LlmProviderSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return gwspec.LlmProviderDocument{}, fmt.Errorf("unmarshal provider %s into its document spec: %w", model.UUID, err)
	}

	// §5.2 rename: v1's `name` is the label, which v2 carries as spec.displayName.
	spec.DisplayName = model.Artifact.Name

	// v2 declares catalog membership here, replacing v1's PUT /llm-providers/{id}/catalog.
	inCatalog := model.Artifact.InCatalog
	spec.InCatalog = &inCatalog

	// spec.gateways is the declared placement, status.gateways the observed one. Reporting the
	// declaration stops a read-edit-post round trip from sending an empty list and undeploying.
	if placement != nil {
		gateways := make([]string, 0, len(placement))
		for _, p := range placement {
			gateways = append(gateways, p.GatewayID.String())
		}
		spec.Gateways = gateways
	}

	meta := gwspec.DocumentMetadata{Name: model.Artifact.Handle}
	if uid := model.UUID.String(); uid != "" {
		meta.Uid = &uid
	}
	if !model.Artifact.CreatedAt.IsZero() {
		created := model.Artifact.CreatedAt
		meta.CreationTimestamp = &created
	}
	if !model.Artifact.UpdatedAt.IsZero() {
		updated := model.Artifact.UpdatedAt
		meta.UpdateTimestamp = &updated
	}
	if model.CreatedBy != "" {
		by := model.CreatedBy
		meta.CreatedBy = &by
	}

	return gwspec.LlmProviderDocument{
		ApiVersion: GroupVersion,
		Kind:       KindLlmProvider,
		Metadata:   meta,
		Spec:       spec,
		Status:     resourceStatus(placement),
	}, nil
}

// resourceStatus renders one entry per gateway, replacing v1's single status string plus flat ID
// list, which could not say which gateway had failed. nil means "not reported" and leaves status
// absent; an empty array means "reported, placed nowhere".
func resourceStatus(placement []gwmodels.DeploymentSummary) *gwspec.ResourceStatus {
	if placement == nil {
		return nil
	}
	gateways := make([]gwspec.DeploymentSummary, 0, len(placement))
	for _, p := range placement {
		id, name, status := p.GatewayID.String(), p.GatewayName, string(p.Status)
		entry := gwspec.DeploymentSummary{GatewayId: id, GatewayName: name, Status: status}
		if p.DeployedAt != nil {
			entry.DeployedAt = p.DeployedAt
		}
		if p.VHost != "" {
			vhost := p.VHost
			entry.Vhost = &vhost
		}
		gateways = append(gateways, entry)
	}
	return &gwspec.ResourceStatus{Gateways: gateways}
}

// LLMProviderList renders a page of providers as the list envelope. Each row is a full
// document, not a summary type: v2 dropped the *ListItem family because a summary cannot be fed
// back to the server. Rows are sparse only in spec.openapi, which a list view never renders.
func LLMProviderList(
	models []*gwmodels.LLMProvider,
	placement map[uuid.UUID][]gwmodels.DeploymentSummary,
	limit, offset, total int,
) (gwspec.LLMProviderListResponse, error) {
	docs := make([]gwspec.LlmProviderDocument, 0, len(models))
	for _, m := range models {
		doc, err := LLMProviderDocument(m, placement[m.UUID])
		if err != nil {
			return gwspec.LLMProviderListResponse{}, err
		}
		doc.Spec.Openapi = nil
		docs = append(docs, doc)
	}

	count := int32(len(docs))
	lim, off, tot := int32(limit), int32(offset), int32(total)
	return gwspec.LLMProviderListResponse{
		Count:      &count,
		List:       docs,
		Pagination: &gwspec.Pagination{Limit: &lim, Offset: &off, Total: &tot},
	}, nil
}

// ErrDocument is the sentinel for a document the server will not accept; the handler maps it to
// 400. Nothing validates a body against the OpenAPI schema at runtime, so the message names the
// field itself.
var ErrDocument = errors.New("invalid document")

// LLMProviderFromDocument turns a posted document into the create request the service takes.
func LLMProviderFromDocument(doc gwspec.LlmProviderDocument) (gwspec.CreateLLMProviderRequest, error) {
	if err := checkDocument(doc); err != nil {
		return gwspec.CreateLLMProviderRequest{}, err
	}

	raw, err := json.Marshal(doc.Spec)
	if err != nil {
		return gwspec.CreateLLMProviderRequest{}, fmt.Errorf("marshal the posted spec: %w", err)
	}
	var req gwspec.CreateLLMProviderRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return gwspec.CreateLLMProviderRequest{}, fmt.Errorf("%w: %w", ErrDocument, err)
	}

	// The §5.2 rename in reverse: handle from metadata.name, label from spec.displayName.
	req.Id = doc.Metadata.Name
	req.Name = doc.Spec.DisplayName

	// metadata's other fields are readOnly, and there is no validator here to reject them.
	return req, nil
}

// LLMProviderUpdateFromDocument turns a PUT body into the document and v1's update request, so
// the document runs v1's update path. A spec field left absent stays absent and is preserved,
// matching v1 - the alternative clears security on a client that cannot echo back a redacted
// credential.
//
// It takes the raw body, not a decoded document, because the struct cannot answer whether a key
// was sent: spec.gateways is `omitempty`, so re-marshalling renders an explicit [] - an undeploy -
// identically to an absent key, which must preserve placement.
func LLMProviderUpdateFromDocument(
	body []byte,
) (gwspec.LlmProviderDocument, gwspec.UpdateLLMProviderRequest, error) {
	var doc gwspec.LlmProviderDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return doc, gwspec.UpdateLLMProviderRequest{}, fmt.Errorf("%w: %w", ErrDocument, err)
	}
	if err := checkDocument(doc); err != nil {
		return doc, gwspec.UpdateLLMProviderRequest{}, err
	}

	rawSpec, err := rawSpecOf(body)
	if err != nil {
		return doc, gwspec.UpdateLLMProviderRequest{}, err
	}
	var req gwspec.UpdateLLMProviderRequest
	if err := json.Unmarshal(rawSpec, &req); err != nil {
		return doc, gwspec.UpdateLLMProviderRequest{}, fmt.Errorf("%w: %w", ErrDocument, err)
	}

	// displayName is the label; the handle is never changed on update, so metadata.name is not
	// carried into the request.
	name := doc.Spec.DisplayName
	req.Name = &name

	// An absent key preserves placement; a declared [] is an undeploy. Decoding the raw spec keeps
	// the two apart, and this only restores nil for a key JSON never saw.
	if !declares(rawSpec, "gateways") {
		req.Gateways = nil
	} else if req.Gateways == nil {
		req.Gateways = []string{}
	}

	return doc, req, nil
}

// checkDocument verifies the parts a Go type cannot: a kind that does not match the route would
// otherwise be built out of whatever fields happened to line up.
func checkDocument(doc gwspec.LlmProviderDocument) error {
	if doc.ApiVersion != GroupVersion {
		return fmt.Errorf("%w: apiVersion must be %q, got %q", ErrDocument, GroupVersion, doc.ApiVersion)
	}
	if doc.Kind != KindLlmProvider {
		return fmt.Errorf("%w: kind must be %q on this route, got %q", ErrDocument, KindLlmProvider, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return fmt.Errorf("%w: metadata.name is required - it is the provider's handle", ErrDocument)
	}
	if strings.TrimSpace(doc.Spec.DisplayName) == "" {
		return fmt.Errorf("%w: spec.displayName is required", ErrDocument)
	}
	return nil
}

// rawSpecOf pulls the spec object out of the posted document, undecoded.
func rawSpecOf(body []byte) (json.RawMessage, error) {
	var envelope struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDocument, err)
	}
	if len(envelope.Spec) == 0 {
		return nil, fmt.Errorf("%w: spec is required", ErrDocument)
	}
	return envelope.Spec, nil
}

// declares reports whether the JSON object carries the key at all, sent null included.
func declares(raw []byte, key string) bool {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	_, ok := keys[key]
	return ok
}
