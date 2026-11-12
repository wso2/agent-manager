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

// Package off is the trace backend for TRACE_BACKEND=off: nothing is stored or read.
package off

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

// Store answers every call with store.ErrDisabled.
type Store struct{}

var _ store.TraceStore = Store{}

// QueryTraces returns store.ErrDisabled.
func (Store) QueryTraces(context.Context, store.Scope, store.TraceQuery) (*store.TracePage, error) {
	return nil, store.ErrDisabled
}

// QueryTraceSpans returns store.ErrDisabled.
func (Store) QueryTraceSpans(context.Context, store.Scope, string, store.SpanQuery) (*store.SpanPage, error) {
	return nil, store.ErrDisabled
}

// GetSpanDetails returns store.ErrDisabled.
func (Store) GetSpanDetails(context.Context, store.Scope, string, string) (*store.SpanDetails, error) {
	return nil, store.ErrDisabled
}
