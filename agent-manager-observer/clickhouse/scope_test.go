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

package clickhouse

import (
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

func TestScopeWhere_OnlyGivenFields(t *testing.T) {
	where, args := scopeWhere(tenantTable, scope.UIDs{Namespace: "default", Component: "abc"})

	if where != "Namespace = ? AND ComponentUid = ?" {
		t.Errorf("unexpected condition: %q", where)
	}
	if len(args) != 2 || args[0] != "default" || args[1] != "abc" {
		t.Errorf("unexpected args: %v", args)
	}
	if strings.Contains(where, "default") || strings.Contains(where, "abc") {
		t.Error("request values must never appear inside the SQL text")
	}
}

func TestScopeWhere_ExporterLayout(t *testing.T) {
	where, _ := scopeWhere(exporterTable, scope.UIDs{Project: "p"})
	want := "ResourceAttributes['openchoreo.dev/project-uid'] = ?"
	if where != want {
		t.Errorf("got %q, want %q", where, want)
	}
}

func TestScopeWhere_Empty(t *testing.T) {
	where, _ := scopeWhere(tenantTable, scope.UIDs{})

	if where != "" {
		t.Errorf("unexpected condition: %q", where)
	}
}
