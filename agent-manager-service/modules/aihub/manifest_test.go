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
	"testing"

	"github.com/stretchr/testify/assert"
)

// The short names the routes use are the manifest's permissions.
func TestPermissionNamesMatchManifest(t *testing.T) {
	mf := Module().Manifest()
	var names []string
	for _, p := range mf.Permissions {
		names = append(names, p.Name)
	}
	assert.Equal(t, Name, mf.Name)
	assert.ElementsMatch(t, []string{
		PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete,
		PermWebhookSecretRead, PermWebhookSecretCreate, PermWebhookSecretRotate, PermWebhookSecretDelete,
	}, names)
}

// The module ships no UI of its own (the console builds it in) and its migrations from schema/.
func TestModuleFiles(t *testing.T) {
	m := Module()
	assert.Nil(t, m.UI())
	assert.NotNil(t, m.Migrations())
}
