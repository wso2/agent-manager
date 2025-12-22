/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
}

export interface TraceStatus {
  errorCount: number;
}

export interface TraceOverview {
  traceId: string;
  rootSpanId: string;
  rootSpanName: string;
  rootSpanKind?: string;
  startTime: string;
  endTime: string;
  durationInNanos: number;
  spanCount: number;
  tokenUsage?: TokenUsage;
  status?: TraceStatus;
  input?: string;
  output?: string;
}

export interface TraceListResponse {
  traces: TraceOverview[];
  totalCount: number;
}

// Keep Trace as an alias for backward compatibility
export type Trace = TraceOverview;

export interface SpanStatus {
  error: boolean;
  errorType?: string;
}

export interface LLMTokenUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadInputTokens?: number;
  totalTokens: number;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: string;
}

export interface PromptMessage {
  role: "system" | "user" | "assistant" | "tool" | "unknown";
  content?: string;
  toolCalls?: ToolCall[];
}

export interface ToolDefinition {
  name: string;
  description?: string;
  parameters?: string;
}

export interface LLMData {
  tools?: ToolDefinition[];
  model?: string;
  vendor?: string;
  temperature?: number;
  tokenUsage?: LLMTokenUsage;
}

export interface ToolData {
  name?: string;
}

export interface EmbeddingData {
  model?: string;
  vendor?: string;
  tokenUsage?: LLMTokenUsage;
}

export interface RetrieverData {
  vectorDB?: string;
  topK?: number;
}

export interface AgentData {
  name?: string;
  tools?: string[];
  model?: string;
  framework?: string;
  systemPrompt?: string;
  tokenUsage?: LLMTokenUsage;
}

export interface AmpAttributes {
  kind: string;
  input?: PromptMessage[] | string[] | string;
  output?: PromptMessage[] | string;
  status?: SpanStatus;
  data?: LLMData | ToolData | EmbeddingData | RetrieverData | AgentData;
}

export interface Span {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  name: string;
  service: string;
  startTime: string;
  endTime?: string;
  durationInNanos: number;
  kind?: string;
  status?: string;
  attributes?: Record<string, unknown>;
  resource?: Record<string, unknown>;
  ampAttributes?: AmpAttributes;
}

export interface TraceDetailsResponse {
  spans: Span[];
  totalCount: number;
  tokenUsage?: TokenUsage;
  status?: TraceStatus;
}

export interface GetTracePathParams {
  orgName: string | undefined;
  projName: string | undefined;
  agentName: string | undefined;
  traceId: string | undefined;
  environment?: string;
}

export type GetTraceListPathParams = { 
  orgName: string | undefined,
  projName: string | undefined,
  agentName: string | undefined,
  environment?: string,
  startTime?: string,
  endTime?: string,
  limit?: number,
  offset?: number,
  sortOrder?: 'asc' | 'desc',
};

export enum TraceListTimeRange {
  TEN_MINUTES = '10m',
  THIRTY_MINUTES = '30m',
  ONE_HOUR = '1h',
  THREE_HOURS = '3h',
  SIX_HOURS = '6h',
  TWELVE_HOURS = '12h',
  ONE_DAY = '1d',
  THREE_DAYS = '3d',
  SEVEN_DAYS = '7d',
}
