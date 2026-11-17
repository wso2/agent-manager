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

// ensureDefaultOrg creates the configured organization and its default
// environment if they do not exist. Organizations are otherwise created on
// first use, so without this a fresh install lists none until something is
// written to one. Runs after the module host starts, so environment hooks
// see the default environment.
func ensureDefaultOrg(ctx context.Context, db *gorm.DB, cfg config.DefaultOrgConfig) error {
	if cfg.OUID == "" {
		return nil
	}
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: cfg.OUID, OuHandle: cfg.Handle})
	if _, err := repositories.EnsureOrg(ctx, db, cfg.OUID); err != nil {
		return fmt.Errorf("default organization %s (%s): %w", cfg.Handle, cfg.OUID, err)
	}
	return nil
}
