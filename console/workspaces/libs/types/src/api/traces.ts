export interface Trace {
  traceId: string;
  rootSpanId: string;
  rootSpanName: string;
  startTime: string;
  endTime: string;
}

export interface TraceListResponse {
  traces: Trace[];
  totalCount: number;
}

export interface Span {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  name: string;
  service: string;
  startTime: string;
  endTime: string;
  durationInNanos: number;
  kind: string;
  status: string;
  attributes: Record<string, unknown>;
}

export interface TraceDetailsResponse {
  spans: Span[];
  totalCount: number;
}

export interface GetTracePathParams {
  orgName: string;
  projName: string;
  agentName: string;
  envId: string;
  traceId: string;
}

export type GetTraceListPathParams = { 
  orgName: string,
  projName: string,
  agentName: string,
  envId: string,
  startTime: string,
  endTime: string,
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
