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

// Migrate only runs up to latestVersion, so a migration appended to the list
// without bumping it is silently never applied.
func TestLatestVersionMatchesLastMigration(t *testing.T) {
	if len(migrations) == 0 {
		t.Fatal("no migrations registered")
	}
	if last := migrations[len(migrations)-1].ID; last != latestVersion {
		t.Fatalf("latestVersion is %d but the last migration in the list is %d — bump latestVersion", latestVersion, last)
	}
	for i, m := range migrations {
		if want := int32(i + 1); m.ID != want {
			t.Fatalf("migrations[%d] has ID %d, want %d: the list must stay sorted and gap-free", i, m.ID, want)
		}
	}
}
