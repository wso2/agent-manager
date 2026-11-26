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

package gwmodels

import "time"

// ModelPrice is one model's price, as a gateway's price file lists it, sanitized: the
// catalog is global and read-only, and a stored model is never changed. Prices are exact
// decimal strings in USD per 1 million tokens; a field the file lacked is nil.
type ModelPrice struct {
	Model                 string    `gorm:"column:model;primaryKey"`
	Provider              *string   `gorm:"column:provider"`
	InputPerMillion       *string   `gorm:"column:input_per_million"`
	OutputPerMillion      *string   `gorm:"column:output_per_million"`
	CachedInputPerMillion *string   `gorm:"column:cached_input_per_million"`
	CreatedAt             time.Time `gorm:"column:created_at"`
}

func (ModelPrice) TableName() string { return "mod_ai_gateway_model_prices" }
