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

import "gorm.io/gorm"

// migration061 records which findings each Shadow Agent Discovery scan saw,
// so a scan's findings can be listed (a finding is merged across scans and
// otherwise keeps only its latest scan). Existing findings are linked to
// their latest scan.
var migration061 = migration{
	ID: 61,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, `
			CREATE TABLE IF NOT EXISTS shadow_scan_findings (
				ou_id      TEXT NOT NULL,
				scan_id    UUID NOT NULL REFERENCES shadow_scans(id) ON DELETE CASCADE,
				finding_id UUID NOT NULL REFERENCES shadow_findings(id) ON DELETE CASCADE,
				PRIMARY KEY (scan_id, finding_id)
			)`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_scan_findings_finding
				ON shadow_scan_findings (finding_id)`,
				`INSERT INTO shadow_scan_findings (ou_id, scan_id, finding_id)
				SELECT f.ou_id, f.last_scan_id, f.id
				FROM shadow_findings f
				JOIN shadow_scans s ON s.id = f.last_scan_id
				WHERE f.last_scan_id IS NOT NULL
				ON CONFLICT DO NOTHING`,
			)
		})
	},
}
