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

package client

import (
	"crypto/sha256"
	"encoding/hex"
)

// GitSecretReferenceName makes the name unique to the organization even when
// OpenChoreo places multiple organizations in the same namespace.
func GitSecretReferenceName(ouID, name string) string {
	sum := sha256.Sum256([]byte(ouID + "\x00" + name))
	return "amp-git-" + hex.EncodeToString(sum[:24])
}

// workflowParamGitSecretKVPath is the build workflow parameter carrying the
// secret store key of the run's Git credentials.
const workflowParamGitSecretKVPath = "gitSecretKVPath"
