// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import (
	"gorm.io/gorm"
)

var migration006 = migration{
	ID: 6,
	Migrate: func(db *gorm.DB) error {
		insertDefaultOrg := `INSERT INTO organizations 
(id, open_choreo_org_name, user_idp_id, org_name) 
VALUES 
('af779290-c22d-4100-aefd-484d81fff60e', 'default', '8f307351-25c5-4fc6-85e0-f51c2d458f06', 'default');
`
		insertDefaultProject := `INSERT INTO projects 
(id, name, open_choreo_project, org_id, display_name) 
VALUES 
('9c2c0915-7c33-4cb8-8ceb-030aff811f8d', 'default', 'default','af779290-c22d-4100-aefd-484d81fff60e', 'Default');
`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, insertDefaultOrg, insertDefaultProject)
		})
	},
}
