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

package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
)

type ctxTX struct{}

func DB(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(ctxTX{}).(*gorm.DB)
	if ok {
		return tx
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		timeoutCtx, cancel := context.WithTimeout(ctx,
			time.Duration(config.GetConfig().DbOperationTimeoutSeconds)*time.Second)
		// Note: We don't defer cancel() here because the returned *gorm.DB
		// will be used beyond this function's scope.
		_ = cancel
		return db.WithContext(timeoutCtx)
	}

	return db.WithContext(ctx)
}

func CtxWithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, ctxTX{}, tx)
}

func IsRecordNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
