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

package gateway

import (
	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

// gatewayView is the flat set of fields the human output prints from a Gateway document.
type gatewayView struct {
	name, displayName, uid, gwType, hosting, status, endpoint string
}

func viewOf(g *aigateway.GatewayDocument) gatewayView {
	v := gatewayView{name: g.Metadata.Name, displayName: g.Spec.DisplayName}
	if g.Metadata.Uid != nil {
		v.uid = g.Metadata.Uid.String()
	}
	if g.Spec.Type != nil {
		v.gwType = string(*g.Spec.Type)
	}
	if g.Spec.Hosting != nil {
		v.hosting = string(*g.Spec.Hosting)
	}
	if g.Status != nil && g.Status.State != nil {
		v.status = string(*g.Status.State)
	}
	if g.Spec.Endpoint != nil {
		v.endpoint = *g.Spec.Endpoint
	}
	return v
}
