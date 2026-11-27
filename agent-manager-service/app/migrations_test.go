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

package app

import (
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
)

func TestPrintMigrations(t *testing.T) {
	var b strings.Builder
	printMigrations(&b, []modhost.MigrationResult{
		{Module: "ai-gateway", Applied: 0, Total: 3},
		{Module: "observability", External: &modhost.ExternalResult{Store: "clickhouse", Applied: 1, Total: 1}},
		{Module: "other", External: &modhost.ExternalResult{Skipped: "no store address"}},
		{Module: "down", External: &modhost.ExternalResult{Store: "clickhouse", Error: "connection refused"}},
		{Module: "quiet"},
	})
	want := "ai-gateway: applied 0 of 3 migration(s)\n" +
		"observability: applied 1 of 1 clickhouse schema file(s)\n" +
		"other: skipped its external schema: no store address\n" +
		"down: external schema not applied, the module stays off until it is: connection refused\n"
	if b.String() != want {
		t.Errorf("printMigrations wrote\n%s\nwant\n%s", b.String(), want)
	}
}
