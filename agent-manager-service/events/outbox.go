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

package events

import (
	"context"
	"encoding/json"
)

// OutboxBatchSize is the most events one Claim returns. Each is published
// with its own timeout, so a batch must finish well inside the claim's lease.
const OutboxBatchSize = 10

// PendingEvent is a durably queued event claimed for one publisher.
type PendingEvent struct {
	ID    string
	Body  json.RawMessage
	Lease string
}

// Outbox keeps events until the bus confirms publication. Claims expire after
// crashes, and completion is conditional on ownership of the claim.
type Outbox interface {
	Put(ctx context.Context, e Event) error
	Claim(ctx context.Context) ([]PendingEvent, error)
	Complete(ctx context.Context, e PendingEvent) error
	Retry(ctx context.Context, e PendingEvent) error
}
