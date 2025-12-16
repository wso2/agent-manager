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

package services

import (
	"context"
	"fmt"
	"log/slog"

	traceobserver "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/trace_observer"
)

type ObservabilityManagerService interface {
	ListTraces(ctx context.Context, params traceobserver.ListTracesParams) (*traceobserver.TraceOverviewResponse, error)
	GetTraceDetails(ctx context.Context, params traceobserver.TraceDetailsByIdParams) (*traceobserver.TraceResponse, error)
}

type observabilityManagerService struct {
	TraceObserverClient traceobserver.TraceObserverClient
	logger              *slog.Logger
}

func NewObservabilityManager(
	traceObserverClient traceobserver.TraceObserverClient,
	logger *slog.Logger,
) ObservabilityManagerService {
	return &observabilityManagerService{
		TraceObserverClient: traceObserverClient,
		logger:              logger,
	}
}

// ListTraces retrieves trace overviews from the trace observer service
func (s *observabilityManagerService) ListTraces(ctx context.Context, params traceobserver.ListTracesParams) (*traceobserver.TraceOverviewResponse, error) {
	s.logger.Info("Listing traces", "serviceName", params.ServiceName, "limit", params.Limit, "offset", params.Offset)

	response, err := s.TraceObserverClient.ListTraces(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list traces", "serviceName", params.ServiceName, "error", err)
		return nil, fmt.Errorf("failed to list traces: %w", err)
	}

	s.logger.Info("Retrieved traces successfully", "serviceName", params.ServiceName, "totalCount", response.TotalCount)
	return response, nil
}

// GetTraceDetails retrieves detailed trace information by trace ID
func (s *observabilityManagerService) GetTraceDetails(ctx context.Context, params traceobserver.TraceDetailsByIdParams) (*traceobserver.TraceResponse, error) {
	s.logger.Info("Getting trace details", "traceId", params.TraceID, "serviceName", params.ServiceName)

	response, err := s.TraceObserverClient.TraceDetailsById(ctx, params)
	if err != nil {
		s.logger.Error("Failed to get trace details", "traceId", params.TraceID, "serviceName", params.ServiceName, "error", err)
		return nil, fmt.Errorf("failed to get trace details: %w", err)
	}

	s.logger.Info("Retrieved trace details successfully", "traceId", params.TraceID, "spanCount", response.TotalCount)
	return response, nil
}
