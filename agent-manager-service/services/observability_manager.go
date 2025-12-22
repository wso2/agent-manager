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
	"errors"
	"fmt"
	"log/slog"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// ErrTraceNotFound is returned when a trace is not found
var ErrTraceNotFound = errors.New("trace not found")

// Service-level request/response types (not exposing client types)
type ListTracesRequest struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Environment string
	StartTime   string
	EndTime     string
	Limit       int
	Offset      int
	SortOrder   string
}

type TraceDetailsRequest struct {
	TraceID     string
	OrgName     string
	ProjectName string
	AgentName   string
	Environment string
}

type ObservabilityManagerService interface {
	ListTraces(ctx context.Context, req ListTracesRequest) (*models.TraceOverviewResponse, error)
	GetTraceDetails(ctx context.Context, req TraceDetailsRequest) (*models.TraceResponse, error)
}

type observabilityManagerService struct {
	traceObserverClient traceobserversvc.TraceObserverClient
	openChoreoClient    openchoreosvc.OpenChoreoSvcClient
	logger              *slog.Logger
}

func NewObservabilityManager(
	traceObserverClient traceobserversvc.TraceObserverClient,
	openChoreoClient openchoreosvc.OpenChoreoSvcClient,
	logger *slog.Logger,
) ObservabilityManagerService {
	return &observabilityManagerService{
		traceObserverClient: traceObserverClient,
		openChoreoClient:    openChoreoClient,
		logger:              logger,
	}
}

// ListTraces retrieves trace overviews from the trace observer service
func (s *observabilityManagerService) ListTraces(ctx context.Context, req ListTracesRequest) (*models.TraceOverviewResponse, error) {
	s.logger.Info("Listing traces", "agentName", req.AgentName, "limit", req.Limit, "offset", req.Offset)

	// Fetch component to get UID
	component, err := s.openChoreoClient.GetAgentComponent(ctx, req.OrgName, req.ProjectName, req.AgentName)
	if err != nil {
		s.logger.Error("Failed to get agent component", "agentName", req.AgentName, "error", err)
		return nil, fmt.Errorf("failed to get agent component: %w", err)
	}

	// Fetch environment to get UID (if specified)

	environment, err := s.openChoreoClient.GetEnvironment(ctx, req.OrgName, req.Environment)
	if err != nil {
		s.logger.Error("Failed to get environment", "environment", req.Environment, "error", err)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Convert service request to client params
	clientParams := traceobserversvc.ListTracesParams{
		ServiceName:    req.AgentName,
		ComponentUid:   component.UUID,
		EnvironmentUid: environment.UUID,
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		Limit:          req.Limit,
		Offset:         req.Offset,
		SortOrder:      req.SortOrder,
	}

	// Call the trace observer client
	clientResponse, err := s.traceObserverClient.ListTraces(ctx, clientParams)
	if err != nil {
		s.logger.Error("Failed to list traces", "agentName", req.AgentName, "error", err)
		return nil, fmt.Errorf("failed to list traces: %w", err)
	}

	s.logger.Info("Successfully listed traces", "agentName", req.AgentName, "traceCount", len(clientResponse.Traces))
	// Convert client response to service model
	traces := make([]models.TraceOverview, len(clientResponse.Traces))
	for i, trace := range clientResponse.Traces {
		var tokenUsage *models.TokenUsage
		if trace.TokenUsage != nil {
			tokenUsage = &models.TokenUsage{
				InputTokens:  trace.TokenUsage.InputTokens,
				OutputTokens: trace.TokenUsage.OutputTokens,
				TotalTokens:  trace.TokenUsage.TotalTokens,
			}
		}

		var traceStatus *models.TraceStatus
		if trace.Status != nil {
			traceStatus = &models.TraceStatus{
				ErrorCount: trace.Status.ErrorCount,
			}
		}

		traces[i] = models.TraceOverview{
			TraceID:         trace.TraceID,
			RootSpanID:      trace.RootSpanID,
			RootSpanName:    trace.RootSpanName,
			RootSpanKind:    trace.RootSpanKind,
			StartTime:       trace.StartTime,
			EndTime:         trace.EndTime,
			DurationInNanos: trace.DurationInNanos,
			SpanCount:       trace.SpanCount,
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           trace.Input,
			Output:          trace.Output,
		}
	}

	response := &models.TraceOverviewResponse{
		Traces:     traces,
		TotalCount: clientResponse.TotalCount,
	}

	s.logger.Info("Retrieved traces successfully", "agentName", req.AgentName, "totalCount", response.TotalCount)
	return response, nil
}

// GetTraceDetails retrieves detailed trace information by trace ID
func (s *observabilityManagerService) GetTraceDetails(ctx context.Context, req TraceDetailsRequest) (*models.TraceResponse, error) {
	s.logger.Info("Getting trace details", "traceId", req.TraceID, "agentName", req.AgentName)

	// Fetch component to get UID
	component, err := s.openChoreoClient.GetAgentComponent(ctx, req.OrgName, req.ProjectName, req.AgentName)
	if err != nil {
		s.logger.Error("Failed to get agent component", "agentName", req.AgentName, "error", err)
		return nil, fmt.Errorf("failed to get agent component: %w", err)
	}

	environment, err := s.openChoreoClient.GetEnvironment(ctx, req.OrgName, req.Environment)
	if err != nil {
		s.logger.Error("Failed to get environment", "environment", req.Environment, "error", err)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Convert service request to client params
	clientParams := traceobserversvc.TraceDetailsByIdParams{
		TraceID:        req.TraceID,
		ServiceName:    req.AgentName,
		ComponentUid:   component.UUID,
		EnvironmentUid: environment.UUID,
	}

	// Call the trace observer client
	clientResponse, err := s.traceObserverClient.TraceDetailsById(ctx, clientParams)
	if err != nil {
		// Check if it's a 404 error using typed error check
		if traceobserversvc.IsNotFound(err) {
			s.logger.Warn("Trace not found", "traceId", req.TraceID, "agentName", req.AgentName)
			return nil, ErrTraceNotFound
		}
		s.logger.Error("Failed to get trace details", "traceId", req.TraceID, "agentName", req.AgentName, "error", err)
		return nil, fmt.Errorf("failed to get trace details: %w", err)
	}

	// Convert client response to service model
	spans := make([]models.Span, len(clientResponse.Spans))
	for i, span := range clientResponse.Spans {
		// Convert AmpAttributes if present
		var ampAttrs *models.AmpAttributes
		if span.AmpAttributes != nil {
			ampAttrs = &models.AmpAttributes{
				Kind:   span.AmpAttributes.Kind,
				Status: span.AmpAttributes.Status,
			}

			// Handle Input - can be []PromptMessage (LLM), string (tool), []string (embedding), etc.
			if span.AmpAttributes.Input != nil {
				// Try to convert to []PromptMessage first (for LLM spans)
				if inputMessages, ok := span.AmpAttributes.Input.([]traceobserversvc.PromptMessage); ok {
					input := make([]models.PromptMessage, len(inputMessages))
					for j, msg := range inputMessages {
						// Convert tool calls if present
						var toolCalls []models.ToolCall
						if len(msg.ToolCalls) > 0 {
							toolCalls = make([]models.ToolCall, len(msg.ToolCalls))
							for k, tc := range msg.ToolCalls {
								toolCalls[k] = models.ToolCall{
									ID:        tc.ID,
									Name:      tc.Name,
									Arguments: tc.Arguments,
								}
							}
						}

						input[j] = models.PromptMessage{
							Role:      msg.Role,
							Content:   msg.Content,
							ToolCalls: toolCalls,
						}
					}
					ampAttrs.Input = input
				} else {
					// Otherwise keep as is (string for tool spans, []string for embedding, etc.)
					ampAttrs.Input = span.AmpAttributes.Input
				}
			}

			// Handle Output - can be []PromptMessage (LLM), string (tool), etc.
			if span.AmpAttributes.Output != nil {
				// Try to convert to []PromptMessage first (for LLM spans)
				if outputMessages, ok := span.AmpAttributes.Output.([]traceobserversvc.PromptMessage); ok {
					output := make([]models.PromptMessage, len(outputMessages))
					for j, msg := range outputMessages {
						// Convert tool calls if present
						var toolCalls []models.ToolCall
						if len(msg.ToolCalls) > 0 {
							toolCalls = make([]models.ToolCall, len(msg.ToolCalls))
							for k, tc := range msg.ToolCalls {
								toolCalls[k] = models.ToolCall{
									ID:        tc.ID,
									Name:      tc.Name,
									Arguments: tc.Arguments,
								}
							}
						}

						output[j] = models.PromptMessage{
							Role:      msg.Role,
							Content:   msg.Content,
							ToolCalls: toolCalls,
						}
					}
					ampAttrs.Output = output
				} else {
					// Otherwise keep as is (string for tool spans, etc.)
					ampAttrs.Output = span.AmpAttributes.Output
				}
			}

			// Pass through Data field as-is - let the frontend handle type-specific rendering
			// This avoids duplicating type definitions and keeps the service decoupled
			if span.AmpAttributes.Data != nil {
				ampAttrs.Data = span.AmpAttributes.Data
			}
		}

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
			AmpAttributes:   ampAttrs,
		}
	}

	// Convert TokenUsage if present
	var tokenUsage *models.TokenUsage
	if clientResponse.TokenUsage != nil {
		tokenUsage = &models.TokenUsage{
			InputTokens:  clientResponse.TokenUsage.InputTokens,
			OutputTokens: clientResponse.TokenUsage.OutputTokens,
			TotalTokens:  clientResponse.TokenUsage.TotalTokens,
		}
	}

	// Convert TraceStatus if present
	var traceStatus *models.TraceStatus
	if clientResponse.Status != nil {
		traceStatus = &models.TraceStatus{
			ErrorCount: clientResponse.Status.ErrorCount,
		}
	}

	response := &models.TraceResponse{
		Spans:      spans,
		TotalCount: clientResponse.TotalCount,
		TokenUsage: tokenUsage,
		Status:     traceStatus,
	}

	s.logger.Info("Retrieved trace details successfully", "traceId", req.TraceID, "spanCount", response.TotalCount)
	return response, nil
}
