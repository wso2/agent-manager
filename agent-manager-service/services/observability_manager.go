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

	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// Service-level request/response types (not exposing client types)
type ListTracesRequest struct {
	ServiceName string
	StartTime   string
	EndTime     string
	Limit       int
	Offset      int
	SortOrder   string
}

type TraceDetailsRequest struct {
	TraceID     string
	ServiceName string
}

type ObservabilityManagerService interface {
	ListTraces(ctx context.Context, req ListTracesRequest) (*models.TraceOverviewResponse, error)
	GetTraceDetails(ctx context.Context, req TraceDetailsRequest) (*models.TraceResponse, error)
}

type observabilityManagerService struct {
	TraceObserverClient traceobserversvc.TraceObserverClient
	logger              *slog.Logger
}

func NewObservabilityManager(
	traceObserverClient traceobserversvc.TraceObserverClient,
	logger *slog.Logger,
) ObservabilityManagerService {
	return &observabilityManagerService{
		TraceObserverClient: traceObserverClient,
		logger:              logger,
	}
}

// ListTraces retrieves trace overviews from the trace observer service
func (s *observabilityManagerService) ListTraces(ctx context.Context, req ListTracesRequest) (*models.TraceOverviewResponse, error) {
	s.logger.Info("Listing traces", "serviceName", req.ServiceName, "limit", req.Limit, "offset", req.Offset)

	// Convert service request to client params
	clientParams := traceobserversvc.ListTracesParams{
		ServiceName: req.ServiceName,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		Limit:       req.Limit,
		Offset:      req.Offset,
		SortOrder:   req.SortOrder,
	}

	// Call the trace observer client
	clientResponse, err := s.TraceObserverClient.ListTraces(ctx, clientParams)
	if err != nil {
		s.logger.Error("Failed to list traces", "serviceName", req.ServiceName, "error", err)
		return nil, fmt.Errorf("failed to list traces: %w", err)
	}

	// Convert client response to service model
	traces := make([]models.TraceOverview, len(clientResponse.Traces))
	for i, trace := range clientResponse.Traces {
		traces[i] = models.TraceOverview{
			TraceID:         trace.TraceID,
			RootSpanID:      trace.RootSpanID,
			RootSpanName:    trace.RootSpanName,
			StartTime:       trace.StartTime,
			EndTime:         trace.EndTime,
			DurationInNanos: trace.DurationInNanos,
			SpanCount:       trace.SpanCount,
		}
	}

	response := &models.TraceOverviewResponse{
		Traces:     traces,
		TotalCount: clientResponse.TotalCount,
	}

	s.logger.Info("Retrieved traces successfully", "serviceName", req.ServiceName, "totalCount", response.TotalCount)
	return response, nil
}

// GetTraceDetails retrieves detailed trace information by trace ID
func (s *observabilityManagerService) GetTraceDetails(ctx context.Context, req TraceDetailsRequest) (*models.TraceResponse, error) {
	s.logger.Info("Getting trace details", "traceId", req.TraceID, "serviceName", req.ServiceName)

	// Convert service request to client params
	clientParams := traceobserversvc.TraceDetailsByIdParams{
		TraceID:     req.TraceID,
		ServiceName: req.ServiceName,
	}

	// Call the trace observer client
	clientResponse, err := s.TraceObserverClient.TraceDetailsById(ctx, clientParams)
	if err != nil {
		s.logger.Error("Failed to get trace details", "traceId", req.TraceID, "serviceName", req.ServiceName, "error", err)
		return nil, fmt.Errorf("failed to get trace details: %w", err)
	}

	// Convert client response to service model
	spans := make([]models.Span, len(clientResponse.Spans))
	for i, span := range clientResponse.Spans {
		spans[i] = models.Span{
			TraceID:         span.TraceID,
			SpanID:          span.SpanID,
			ParentSpanID:    span.ParentSpanID,
			Name:            span.Name,
			Service:         span.Service,
			Kind:            span.Kind,
			StartTime:       span.StartTime,
			EndTime:         span.EndTime,
			DurationInNanos: span.DurationInNanos,
			Status:          span.Status,
			Attributes:      span.Attributes,
			Resource:        span.Resource,
		}
	}

	response := &models.TraceResponse{
		Spans:      spans,
		TotalCount: clientResponse.TotalCount,
	}

	s.logger.Info("Retrieved trace details successfully", "traceId", req.TraceID, "spanCount", response.TotalCount)
	return response, nil
}
