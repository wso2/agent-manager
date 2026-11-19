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

package modhost

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// TestCheckManifestAuditDetails: an audit action's detail keys must name a
// kind the audit trail knows, since the kind decides how the value is
// redacted.
func TestCheckManifestAuditDetails(t *testing.T) {
	for name, tc := range map[string]struct {
		details map[string]string
		ok      bool
	}{
		"known kinds":   {details: map[string]string{"sourceUrl": "url", "count": "count"}, ok: true},
		"unknown kind":  {details: map[string]string{"token": "secret"}},
		"malformed key": {details: map[string]string{"Bad-Key": "name"}},
	} {
		t.Run(name, func(t *testing.T) {
			mf := modkit.Manifest{Name: "widgets", AuditActions: []modkit.AuditAction{{Name: "widgets.update", Details: tc.details}}}
			if err := checkManifest(mf); (err == nil) != tc.ok {
				t.Errorf("checkManifest = %v, want ok %v", err, tc.ok)
			}
		})
	}
}
