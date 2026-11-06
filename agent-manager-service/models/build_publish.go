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

package models

import (
	"encoding/json"
)

// PublishBuildWorkloadRequest is what a build's workload step sends to the build
// publisher endpoint: the two documents amp-generate-workload builds.
type PublishBuildWorkloadRequest struct {
	// Workload is the generated Workload ({metadata, spec}). Only
	// spec.container.image and spec.endpoints are taken from it; its name,
	// namespace and owner are derived from the verified build instead.
	Workload json.RawMessage `json:"workload"`
	// WorkloadOverrides is the build's environment configuration
	// ({container: {env, files}}), or null when it has none.
	WorkloadOverrides json.RawMessage `json:"workloadOverrides,omitempty"`
}
