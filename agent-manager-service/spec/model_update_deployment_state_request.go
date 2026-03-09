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

package spec

// DeploymentState constants
const (
	DeploymentStateActive   = "Active"
	DeploymentStateUndeploy = "Undeploy"
)

// UpdateDeploymentStateRequest represents a request to update deployment state
type UpdateDeploymentStateRequest struct {
	// Environment is the target environment name
	Environment string `json:"environment"`
	// State is the desired deployment state (Active or Undeploy)
	State string `json:"state"`
}

// UpdateDeploymentStateResponse represents the response after updating deployment state
type UpdateDeploymentStateResponse struct {
	// Message is the success message
	Message string `json:"message"`
	// Environment is the environment that was updated
	Environment string `json:"environment"`
	// State is the new deployment state
	State string `json:"state"`
}
