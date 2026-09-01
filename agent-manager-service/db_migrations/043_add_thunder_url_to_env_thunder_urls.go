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
	"fmt"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
)

// migration043 adds thunder_url, the fully-resolved env-Thunder origin
// (scheme://host[:port]), so a row is self-describing instead of every reader
// reconstructing it from thunder_handle plus one global base-domain config —
// which only works when every environment shares one domain. thunder_handle
// becomes nullable: a caller can now supply thunder_url directly with no
// handle at all (see EnvironmentService.SetThunderURL).
//
// Existing handle-based rows are preserved. Their URL is backfilled through
// the same configuration-aware helper used by the live write path, so an
// upgrade respects TLS_ENABLED and IDP_HOST_BASE_DOMAIN instead of
// assuming the local-development origin.
//
// uq_env_thunder_urls_url is the new primary invariant. uq_env_thunder_urls_handle
// stays, scoped to non-null handles — Postgres never treats multiple NULLs as
// conflicting, so handle-less rows don't participate in it.
var migration043 = migration{
	ID: 43,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(
				tx,
				`ALTER TABLE env_thunder_urls ADD COLUMN IF NOT EXISTS thunder_url VARCHAR(2048)`,
			); err != nil {
				return err
			}

			var rows []struct {
				ID            string
				ThunderHandle string
			}
			if err := tx.Table("env_thunder_urls").
				Select("id, thunder_handle").
				Where("thunder_url IS NULL AND thunder_handle IS NOT NULL").
				Find(&rows).Error; err != nil {
				return fmt.Errorf("read pre-existing env_thunder_urls rows for backfill: %w", err)
			}

			for _, row := range rows {
				// The pre-migration schema only required thunder_handle NOT NULL,
				// with no CHECK against an empty string, so one is a storable
				// (if never application-written) value. ThunderOriginFromHandle
				// panics on "" — inside this transaction that would roll back
				// and re-panic, wedging the upgrade on every retry. Fail with a
				// clear, row-identifying error instead of hitting that panic.
				if row.ThunderHandle == "" {
					return fmt.Errorf("env_thunder_urls row %s has an empty thunder_handle — clear or fix it manually before re-running this migration", row.ID)
				}
				thunderURL := thundersvc.ThunderOriginFromHandle(row.ThunderHandle)
				if err := tx.Exec(
					`UPDATE env_thunder_urls SET thunder_url = ? WHERE id = ?`,
					thunderURL,
					row.ID,
				).Error; err != nil {
					return fmt.Errorf("backfill thunder_url for row %s: %w", row.ID, err)
				}
			}

			return runSQL(
				tx,
				`ALTER TABLE env_thunder_urls ALTER COLUMN thunder_url SET NOT NULL`,
				`ALTER TABLE env_thunder_urls ALTER COLUMN thunder_handle DROP NOT NULL`,
				`ALTER TABLE env_thunder_urls ADD CONSTRAINT uq_env_thunder_urls_url UNIQUE (thunder_url)`,
			)
		})
	},
}
