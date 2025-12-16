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

package clientmocks

import (
	"context"
	"sync"

	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
)

// Ensure TraceObserverClientMock implements TraceObserverClient interface
var _ traceobserversvc.TraceObserverClient = (*TraceObserverClientMock)(nil)

type TraceObserverClientMock struct {
	// ListTraces
	ListTracesFunc  func(ctx context.Context, params traceobserversvc.ListTracesParams) (*traceobserversvc.TraceOverviewResponse, error)
	listTracesMutex sync.RWMutex
	listTracesCalls []struct {
		Ctx    context.Context
		Params traceobserversvc.ListTracesParams
	}

	// TraceDetailsById
	TraceDetailsByIdFunc  func(ctx context.Context, params traceobserversvc.TraceDetailsByIdParams) (*traceobserversvc.TraceResponse, error)
	traceDetailsByIdMutex sync.RWMutex
	traceDetailsByIdCalls []struct {
		Ctx    context.Context
		Params traceobserversvc.TraceDetailsByIdParams
	}
}

func (m *TraceObserverClientMock) ListTraces(ctx context.Context, params traceobserversvc.ListTracesParams) (*traceobserversvc.TraceOverviewResponse, error) {
	m.listTracesMutex.Lock()
	m.listTracesCalls = append(m.listTracesCalls, struct {
		Ctx    context.Context
		Params traceobserversvc.ListTracesParams
	}{
		Ctx:    ctx,
		Params: params,
	})
	m.listTracesMutex.Unlock()

	if m.ListTracesFunc != nil {
		return m.ListTracesFunc(ctx, params)
	}

	return &traceobserversvc.TraceOverviewResponse{}, nil
}

func (m *TraceObserverClientMock) ListTracesCalls() []struct {
	Ctx    context.Context
	Params traceobserversvc.ListTracesParams
} {
	m.listTracesMutex.RLock()
	defer m.listTracesMutex.RUnlock()
	return m.listTracesCalls
}

func (m *TraceObserverClientMock) TraceDetailsById(ctx context.Context, params traceobserversvc.TraceDetailsByIdParams) (*traceobserversvc.TraceResponse, error) {
	m.traceDetailsByIdMutex.Lock()
	m.traceDetailsByIdCalls = append(m.traceDetailsByIdCalls, struct {
		Ctx    context.Context
		Params traceobserversvc.TraceDetailsByIdParams
	}{
		Ctx:    ctx,
		Params: params,
	})
	m.traceDetailsByIdMutex.Unlock()

	if m.TraceDetailsByIdFunc != nil {
		return m.TraceDetailsByIdFunc(ctx, params)
	}

	return &traceobserversvc.TraceResponse{}, nil
}

func (m *TraceObserverClientMock) TraceDetailsByIdCalls() []struct {
	Ctx    context.Context
	Params traceobserversvc.TraceDetailsByIdParams
} {
	m.traceDetailsByIdMutex.RLock()
	defer m.traceDetailsByIdMutex.RUnlock()
	return m.traceDetailsByIdCalls
}
