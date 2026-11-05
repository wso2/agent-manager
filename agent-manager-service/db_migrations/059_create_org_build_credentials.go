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

import (
	"gorm.io/gorm"
)

// org_build_credentials records each organization's build client: the OAuth2
// client a source build's workload step uses to call the build publisher
// endpoint. Only its ID and the name of the SecretReference holding it are
// stored; the secret itself stays in the secret store, which the build reads it
// from through that SecretReference.
var migration059 = migration{
	ID: 59,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			createTable := `
			CREATE TABLE IF NOT EXISTS org_build_credentials (
				id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id           VARCHAR(255) NOT NULL,
				org_uuid        VARCHAR(255) NOT NULL DEFAULT '',
				client_id       VARCHAR(255) NOT NULL,
				secret_ref_name VARCHAR(255) NOT NULL,
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

				CONSTRAINT uq_org_build_creds_ou_id UNIQUE (ou_id)
			)`
			return runSQL(tx, createTable)
		})
	},
}
