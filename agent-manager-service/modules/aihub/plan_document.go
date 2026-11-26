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

package aihub

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// A plan as a document, the shape Kubernetes resources have, so a plan can be kept in a file and applied, by a
// person or an operator, as well as edited in the console:
//
//	apiVersion: ai-control-plane.choreolabs.com/v1
//	kind: SubscriptionPlan
//	metadata:
//	  name: gold              # the handle; the rest of metadata is set by the server
//	spec:
//	  displayName: Gold
//	  conditions: [...]
//	status:                   # observed, set by the server and ignored on input
//	  deprecated: false
//	  usage: {inUse: true, mcpProxies: 1, subscriptions: 3}   # GET of one plan only
//
// apiVersion is the version of the document's schema, separate from the URL's, as in Kubernetes. It is the
// version the AI Gateway's documents (LLM providers) carry.

const (
	documentAPIVersion = "ai-control-plane.choreolabs.com/v1"
	documentKind       = "SubscriptionPlan"
	contentTypeYAML    = "application/yaml"
)

type documentMetadata struct {
	Name              string     `json:"name"`
	UID               string     `json:"uid,omitempty"`
	CreationTimestamp *time.Time `json:"creationTimestamp,omitempty"`
	UpdateTimestamp   *time.Time `json:"updateTimestamp,omitempty"`
	CreatedBy         string     `json:"createdBy,omitempty"`
	// Origin is where the plan was made: ai-control-plane, or the developer portal that copied it in.
	Origin string `json:"origin,omitempty"`
}

// planSpec is a plan's spec as the server returns it. A posted spec is read into createRequest or updateRequest,
// which also take the older "limits" field.
type planSpec struct {
	DisplayName string      `json:"displayName"`
	Description string      `json:"description,omitempty"`
	Status      string      `json:"status"`
	ExpiryTime  *time.Time  `json:"expiryTime,omitempty"`
	Conditions  []Condition `json:"conditions"`
}

// usageResponse says what offers or uses a plan.
type usageResponse struct {
	InUse         bool `json:"inUse"`
	MCPProxies    int  `json:"mcpProxies"`
	Subscriptions int  `json:"subscriptions"`
}

// planStatus is what the server observes of a plan. A deprecated plan is read only: it can't be changed, offered
// or subscribed to again, and stays for the subscriptions on it.
type planStatus struct {
	Deprecated           bool           `json:"deprecated"`
	DeprecationTimestamp *time.Time     `json:"deprecationTimestamp,omitempty"`
	Usage                *usageResponse `json:"usage,omitempty"`
}

type planDocument struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   documentMetadata `json:"metadata"`
	Spec       planSpec         `json:"spec"`
	Status     planStatus       `json:"status"`
}

type pagination struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type listResponse struct {
	List       []planDocument `json:"list"`
	Count      int            `json:"count"`
	Pagination pagination     `json:"pagination"`
}

// documentFor renders a plan as its document. status carries usage only when use is set (GET of one plan).
func documentFor(p *plan, use *usage) planDocument {
	doc := planDocument{
		APIVersion: documentAPIVersion, Kind: documentKind,
		Metadata: documentMetadata{Name: p.Handle, UID: p.UUID, CreatedBy: p.CreatedBy, Origin: p.Origin},
		Status:   planStatus{Deprecated: p.deprecated(), DeprecationTimestamp: p.DeprecatedAt},
		Spec: planSpec{
			DisplayName: p.Name, Description: p.Description, Status: p.Status, ExpiryTime: p.ExpiryTime,
			Conditions: p.Conditions,
		},
	}
	if !p.CreatedAt.IsZero() {
		created := p.CreatedAt
		doc.Metadata.CreationTimestamp = &created
	}
	if !p.UpdatedAt.IsZero() {
		updated := p.UpdatedAt
		doc.Metadata.UpdateTimestamp = &updated
	}
	if doc.Spec.Conditions == nil {
		doc.Spec.Conditions = []Condition{}
	}
	if use != nil {
		doc.Status.Usage = &usageResponse{
			InUse: use.inUse(), MCPProxies: use.MCPProxies, Subscriptions: use.Subscriptions,
		}
	}
	return doc
}

// inDocument is a posted document, before its spec is read.
type inDocument struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
}

// readDocument reads a posted document, JSON or YAML, and checks its envelope: apiVersion and kind must be this
// route's, and metadata.name and spec are required. Nothing validates a body against the OpenAPI document at
// run time, so each message names the field. status and the server-set metadata are ignored.
func (a *planController) readDocument(w http.ResponseWriter, r *http.Request) (name string, spec json.RawMessage, ok bool) {
	body, ok := a.readBody(w, r)
	if !ok {
		return "", nil, false
	}
	if isYAML(r.Header.Get("Content-Type")) {
		converted, err := yamlToJSON(body)
		if err != nil {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid YAML: "+err.Error())
			return "", nil, false
		}
		body = converted
	}
	var doc inDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid request body")
		return "", nil, false
	}
	msg := ""
	switch {
	case doc.APIVersion != documentAPIVersion:
		msg = fmt.Sprintf("apiVersion must be %q, got %q", documentAPIVersion, doc.APIVersion)
	case doc.Kind != documentKind:
		msg = fmt.Sprintf("kind must be %q on this route, got %q", documentKind, doc.Kind)
	case strings.TrimSpace(doc.Metadata.Name) == "":
		msg = "metadata.name is required: it is the plan's handle"
	case len(doc.Spec) == 0 || string(doc.Spec) == "null":
		msg = "spec is required"
	}
	if msg != "" {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
		return "", nil, false
	}
	return doc.Metadata.Name, doc.Spec, true
}

// writeDocument writes a document, or a list of them, as YAML when the request accepts YAML first, else JSON.
// Errors are always JSON.
func (a *planController) writeDocument(w http.ResponseWriter, r *http.Request, status int, v any) {
	if !prefersYAML(r.Header.Get("Accept")) {
		a.writeJSON(w, status, v)
		return
	}
	out, err := jsonToYAML(v)
	if err != nil {
		a.log.ErrorContext(r.Context(), "could not render a document as YAML", "error", err)
		a.writeError(w, http.StatusInternalServerError, codeInternal, "Failed to render the document")
		return
	}
	w.Header().Set("Content-Type", contentTypeYAML)
	w.WriteHeader(status)
	_, _ = w.Write(out)
}

// isYAML reports whether a Content-Type names YAML.
func isYAML(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && yamlMediaTypes[mt]
}

// prefersYAML reports whether the first media type an Accept header names is YAML.
func prefersYAML(accept string) bool {
	for _, part := range strings.Split(accept, ",") {
		mt, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil || mt == "*/*" {
			continue
		}
		return yamlMediaTypes[mt]
	}
	return false
}

var yamlMediaTypes = map[string]bool{contentTypeYAML: true, "application/x-yaml": true, "text/yaml": true}

// yamlToJSON turns one YAML document into JSON. A timestamp stays the text it was written as, so it is parsed
// as RFC 3339 like a JSON one.
func yamlToJSON(in []byte) ([]byte, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(in, &node); err != nil {
		return nil, err
	}
	if len(node.Content) == 0 {
		return nil, fmt.Errorf("the body is empty")
	}
	v, err := yamlValue(node.Content[0])
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// yamlValue is the JSON value of a YAML node: mappings with string keys, sequences, and scalars, which keep
// their type except timestamps (text).
func yamlValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: a key must be a string", k.Line)
			}
			v, err := yamlValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[k.Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := yamlValue(c)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case yaml.AliasNode:
		return yamlValue(n.Alias)
	case yaml.ScalarNode:
		if n.Tag == "!!timestamp" {
			return n.Value, nil
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("line %d: unsupported YAML", n.Line)
}

// jsonToYAML renders v as YAML through its JSON form, so the field names and omissions are the JSON ones. Keys
// come out sorted, which puts apiVersion, kind, metadata, spec and status in that order.
func jsonToYAML(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return yaml.Marshal(generic)
}
