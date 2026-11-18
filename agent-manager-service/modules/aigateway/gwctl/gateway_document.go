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

package gwctl

import (
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

// kindGateway is the kind of a gateway document.
const kindGateway = "Gateway"

// gatewayDocument renders a gateway as its document.
func gatewayDocument(gw *gwsvc.GatewayResponse) gwspec.GatewayDocument {
	meta := gwspec.DocumentMetadata{Name: gw.Name}
	if gw.ID != "" {
		uid := gw.ID
		meta.Uid = &uid
	}
	if !gw.CreatedAt.IsZero() {
		created := gw.CreatedAt
		meta.CreationTimestamp = &created
	}
	if !gw.UpdatedAt.IsZero() {
		updated := gw.UpdatedAt
		meta.UpdateTimestamp = &updated
	}

	hosting := gwspec.GatewayHosting(gw.Hosting)
	kind := canonicalGatewayType(gw.FunctionalityType)
	critical := gw.IsCritical
	spec := gwspec.GatewaySpec{
		DisplayName: gw.DisplayName,
		Hosting:     &hosting,
		Type:        &kind,
		IsCritical:  &critical,
		Properties:  gw.Properties,
	}
	if gw.Description != "" {
		description := gw.Description
		spec.Description = &description
	}
	if gw.Vhost != "" {
		endpoint := gw.Vhost
		spec.Endpoint = &endpoint
	}
	if gw.RuntimeURL != "" {
		runtimeURL := gw.RuntimeURL
		spec.RuntimeUrl = &runtimeURL
	}

	state := gwspec.GATEWAYSTATUS_INACTIVE
	if gw.IsActive {
		state = gwspec.GATEWAYSTATUS_ACTIVE
	}
	status := &gwspec.GatewayDocumentStatus{State: &state}
	if gw.IdentityProviderMode != "" {
		mode := gwspec.GatewayIdentityProviderMode(gw.IdentityProviderMode)
		status.IdentityProviderMode = &mode
	}
	return gwspec.GatewayDocument{
		ApiVersion: gwconv.GroupVersion,
		Kind:       kindGateway,
		Metadata:   meta,
		Spec:       spec,
		Status:     status,
	}
}

// gatewayList renders a page of gateways as the list envelope.
func gatewayList(page *gwsvc.GatewayListResponse, limit, offset int) gwspec.GatewayListResponse {
	docs := make([]gwspec.GatewayDocument, 0, len(page.List))
	for i := range page.List {
		docs = append(docs, gatewayDocument(&page.List[i]))
	}
	count := int32(len(docs))
	lim, off, tot := int32(limit), int32(offset), int32(page.Pagination.Total)
	return gwspec.GatewayListResponse{
		Count:      &count,
		List:       docs,
		Pagination: &gwspec.Pagination{Limit: &lim, Offset: &off, Total: &tot},
	}
}

// checkGatewayDocument refuses a document of the wrong group or kind, or without a name or label.
func checkGatewayDocument(doc gwspec.GatewayDocument) error {
	if doc.ApiVersion != gwconv.GroupVersion {
		return fmt.Errorf("%w: apiVersion must be %q, got %q", gwconv.ErrDocument, gwconv.GroupVersion, doc.ApiVersion)
	}
	if doc.Kind != kindGateway {
		return fmt.Errorf("%w: kind must be %q on this route, got %q", gwconv.ErrDocument, kindGateway, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return fmt.Errorf("%w: metadata.name is required - it is the gateway's name", gwconv.ErrDocument)
	}
	if strings.TrimSpace(doc.Spec.DisplayName) == "" {
		return fmt.Errorf("%w: spec.displayName is required", gwconv.ErrDocument)
	}
	return nil
}

// selfHostedFromDocument turns a posted document into a self-hosted registration in environmentID.
// Only self-hosted egress gateways can be registered; the platform provisions managed ones.
func selfHostedFromDocument(doc gwspec.GatewayDocument, environmentID string) (gwsvc.SelfHostedGatewayInput, error) {
	if err := checkGatewayDocument(doc); err != nil {
		return gwsvc.SelfHostedGatewayInput{}, err
	}
	s := doc.Spec
	if s.Hosting != nil && *s.Hosting != gwspec.GATEWAYHOSTING_SELF_HOSTED {
		return gwsvc.SelfHostedGatewayInput{}, fmt.Errorf(
			"%w: spec.hosting must be %q - managed gateways are provisioned by the platform",
			gwconv.ErrDocument, gwspec.GATEWAYHOSTING_SELF_HOSTED)
	}
	if s.Type != nil && *s.Type != gwspec.GATEWAYTYPE_EGRESS {
		return gwsvc.SelfHostedGatewayInput{}, fmt.Errorf("%w: spec.type of a self-hosted gateway must be %q",
			gwconv.ErrDocument, gwspec.GATEWAYTYPE_EGRESS)
	}
	if s.Endpoint == nil || strings.TrimSpace(*s.Endpoint) == "" {
		return gwsvc.SelfHostedGatewayInput{}, fmt.Errorf("%w: spec.endpoint is required", gwconv.ErrDocument)
	}
	return gwsvc.SelfHostedGatewayInput{
		DisplayName: s.DisplayName,
		Handle:      doc.Metadata.Name,
		Description: derefString(s.Description),
		Environment: environmentID,
		Endpoints:   []string{*s.Endpoint},
		IsCritical:  derefBool(s.IsCritical),
		Properties:  s.Properties,
	}, nil
}

// gatewayUpdate is what a PUT replaces on a self-hosted gateway.
type gatewayUpdate struct {
	DisplayName string
	Description string
	IsCritical  bool
	Properties  map[string]interface{}
}

// gatewayUpdateFromDocument checks a put document against the stored gateway: its name, hosting, type and
// endpoint are fixed. An absent description or properties clears the stored one.
func gatewayUpdateFromDocument(doc gwspec.GatewayDocument, existing *gwmodels.Gateway) (gatewayUpdate, error) {
	if err := checkGatewayDocument(doc); err != nil {
		return gatewayUpdate{}, err
	}
	if doc.Metadata.Name != existing.Name {
		return gatewayUpdate{}, fmt.Errorf("%w: metadata.name does not match the gateway in the path", gwconv.ErrDocument)
	}
	s := doc.Spec
	if s.Hosting != nil && *s.Hosting != gwspec.GATEWAYHOSTING_SELF_HOSTED {
		return gatewayUpdate{}, fmt.Errorf("%w: spec.hosting cannot change", gwconv.ErrDocument)
	}
	if s.Type != nil && *s.Type != canonicalGatewayType(existing.GatewayFunctionalityType) {
		return gatewayUpdate{}, fmt.Errorf("%w: spec.type cannot change", gwconv.ErrDocument)
	}
	if s.Endpoint != nil && strings.TrimRight(strings.TrimSpace(*s.Endpoint), "/") != strings.TrimRight(existing.Vhost, "/") {
		return gatewayUpdate{}, fmt.Errorf("%w: spec.endpoint cannot change", gwconv.ErrDocument)
	}
	properties := s.Properties
	if properties == nil {
		properties = map[string]interface{}{}
	}
	return gatewayUpdate{
		DisplayName: s.DisplayName,
		Description: derefString(s.Description),
		IsCritical:  derefBool(s.IsCritical),
		Properties:  properties,
	}, nil
}
