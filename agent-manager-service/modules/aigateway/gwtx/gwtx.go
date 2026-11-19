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

// Package gwtx carries the gateway's open transaction in a context, so code that renders or places an artifact
// inside a save reads the save's own rows.
package gwtx

import (
	"context"

	"gorm.io/gorm"
)

type key struct{}

// With returns ctx carrying tx.
func With(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, key{}, tx)
}

// From is the transaction ctx carries; nil when it carries none.
func From(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(key{}).(*gorm.DB)
	return tx
}

// Or is the transaction ctx carries, or db bound to ctx when it carries none.
func Or(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx := From(ctx); tx != nil {
		return tx
	}
	return db.WithContext(ctx)
}
