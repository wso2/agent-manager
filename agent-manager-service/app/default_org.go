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
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// ensureDefaultOrg gives the trial organization, the one file sign-in names,
// its default environment if it has none. Organizations otherwise get it on
// first use, so without this a fresh install is empty until something is
// written to one. Runs before modules activate, so a module can rely on it
// existing; no module hook or module event sees its creation. With an
// identity provider it does nothing: the provider's organization is set up on
// its first request.
func ensureDefaultOrg(ctx context.Context, db *gorm.DB, authMode string, cfg config.DefaultOrgConfig) error {
	if cfg.OUID == "" || authMode != config.AuthModeFile {
		return nil
	}
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: cfg.OUID, OuHandle: cfg.Handle})
	if _, err := repositories.EnsureOrg(ctx, db, cfg.OUID); err != nil {
		return fmt.Errorf("default organization %s (%s): %w", cfg.Handle, cfg.OUID, err)
	}
	return nil
}
