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

package dbmigrations

import "testing"

// The migration list is applied in order up to latestVersion, so it must be
// gapless, ascending and end at latestVersion. A migration added to the list
// without bumping latestVersion would silently never run.
func TestMigrationListIsGaplessAscendingAndEndsAtTheLatestVersion(t *testing.T) {
	if len(migrations) == 0 {
		t.Fatal("no migrations")
	}
	for i, m := range migrations {
		if want := int32(i + 1); m.ID != want {
			t.Fatalf("migration at position %d has id %d, want %d", i, m.ID, want)
		}
		if m.Migrate == nil {
			t.Fatalf("migration %d has no Migrate function", m.ID)
		}
	}
	if last := migrations[len(migrations)-1].ID; last != latestVersion {
		t.Fatalf("the last migration is %d but latestVersion is %d", last, latestVersion)
	}
}
