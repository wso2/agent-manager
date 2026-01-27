export type ApiConfig = {
  baseUrl: string;
  apiKey?: string;
};

export type ReportResponse = {
  report: string;
};

export type ServiceCapabilities = {
  services: {
    openai: boolean;
    serpapi: boolean;
    twelveData: boolean;
    alphaVantage: boolean;
  };
  capabilities: {
    news_search: boolean;
    market_data: boolean;
    fundamentals: boolean;
    ai_agents: boolean;
  };
};

const STORAGE_KEY = "agentSettings";
export const DEFAULT_API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://default.localhost:9080/finance-insight";

const buildHeaders = (apiKey?: string) => {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  if (apiKey) {
    headers.Authorization = `Bearer ${apiKey}`;
    headers["X-API-Key"] = apiKey;
  }

  return headers;
};

export const getApiConfig = (): ApiConfig => {
  if (typeof window === "undefined") {
    return { baseUrl: DEFAULT_API_BASE_URL };
  }

  const stored = window.localStorage.getItem(STORAGE_KEY);
  if (!stored) {
    return { baseUrl: DEFAULT_API_BASE_URL };
  }

  try {
    const parsed = JSON.parse(stored) as ApiConfig;
    return {
      baseUrl: parsed.baseUrl || DEFAULT_API_BASE_URL,
      apiKey: parsed.apiKey || "",
    };
  } catch {
    return { baseUrl: DEFAULT_API_BASE_URL };
  }
};

export const setApiConfig = (config: ApiConfig) => {
  if (typeof window === "undefined") {
    return;
  }

  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
};

type SendOptions = {
  onTrace?: (message: string) => void;
  onJobId?: (jobId: string) => void;
  signal?: AbortSignal;
};

const isAbortError = (error: unknown) =>
  error instanceof DOMException && error.name === "AbortError";

export const sendMessage = async (
  message: string,
  options?: SendOptions,
): Promise<ReportResponse> => {
  const { baseUrl, apiKey } = getApiConfig();
  const onTrace = options?.onTrace;
  const signal = options?.signal;
  
  console.log('[API] Starting async job:', `${baseUrl}/chat/async`);
  
  // Start async job
  let startResponse: Response;
  try {
    startResponse = await fetch(`${baseUrl}/chat/async`, {
      method: "POST",
      headers: buildHeaders(apiKey),
      body: JSON.stringify({ message }),
      signal,
    });
  } catch (error) {
    if (isAbortError(error)) {
      throw error;
    }
    console.error('[API] Fetch error:', error);
    throw new Error("Cannot connect to backend. Make sure the API server is running.");
  }

  if (!startResponse.ok) {
    console.error('[API] Response not OK:', startResponse.status, startResponse.statusText);
    throw new Error(`Server error: ${startResponse.status} ${startResponse.statusText}`);
  }

  const startData = await startResponse.json();
  const { jobId } = startData;
  console.log('[API] Job started:', jobId);
  options?.onJobId?.(jobId);

  // Poll for status
  const seenTraces = new Set<string>();
  let lastTraceCount = 0;

  const pollStatus = async (): Promise<ReportResponse> => {
    while (true) {
      if (signal?.aborted) {
        throw new DOMException("Request cancelled", "AbortError");
      }
      await new Promise(resolve => setTimeout(resolve, 2000)); // Poll every 2 seconds

      try {
        const statusResponse = await fetch(`${baseUrl}/chat/async/${jobId}/status`, {
          headers: buildHeaders(apiKey),
          signal,
        });

        if (!statusResponse.ok) {
          console.error('[API] Status check failed:', statusResponse.status);
          throw new Error(`Failed to check job status: ${statusResponse.status}`);
        }

        const statusData = await statusResponse.json();
        console.log('[API] Job status:', statusData.status, 'traces:', statusData.traceCount);

        // Emit new traces
        if (onTrace && statusData.traces) {
          for (const trace of statusData.traces) {
            const traceKey = trace.seq ? `seq-${trace.seq}` : `${trace.timestamp}-${trace.message}`;
            if (!seenTraces.has(traceKey) && statusData.traceCount > lastTraceCount) {
              seenTraces.add(traceKey);
              onTrace(trace.message);
            }
          }
          lastTraceCount = statusData.traceCount;
        }

        // Check if job is complete
        if (statusData.status === 'completed' || statusData.status === 'failed' || statusData.status === 'cancelled') {
          console.log('[API] Job finished:', statusData.status);
          
          // Get final result
          const resultResponse = await fetch(`${baseUrl}/chat/async/${jobId}/result`, {
            headers: buildHeaders(apiKey),
            signal,
          });

          if (!resultResponse.ok) {
            const errorData = await resultResponse.json().catch(() => ({ error: 'Unknown error' }));
            console.error('[API] Result fetch failed:', errorData);
            throw new Error(errorData.error || 'Failed to get result');
          }

          const resultData = await resultResponse.json();
          console.log('[API] Final result received');

          if (resultData.status === "cancelled") {
            throw new DOMException("Request cancelled", "AbortError");
          }

          return {
            report: resultData.result?.report ?? "",
          };
        }

      } catch (error) {
        if (!isAbortError(error)) {
          console.error('[API] Polling error:', error);
        }
        throw error;
      }
    }
  };

  return await pollStatus();
};

export const cancelJob = async (jobId: string): Promise<boolean> => {
  const { baseUrl, apiKey } = getApiConfig();
  try {
    const response = await fetch(`${baseUrl}/chat/async/${jobId}/cancel`, {
      method: "POST",
      headers: buildHeaders(apiKey),
    });
    return response.ok;
  } catch {
    return false;
  }
};

export const testConnection = async (config?: ApiConfig): Promise<boolean> => {
  const resolved = config ?? getApiConfig();
  try {
    const response = await fetch(`${resolved.baseUrl}/health`, {
      headers: buildHeaders(resolved.apiKey),
      cache: "no-store",
    });
    return response.ok;
  } catch {
    return false;
  }
};

export const fetchConfig = async (): Promise<ServiceCapabilities | null> => {
  try {
    const { baseUrl, apiKey } = getApiConfig();
    const response = await fetch(`${baseUrl}/config`, {
      headers: buildHeaders(apiKey),
    });

    if (!response.ok) {
      return null;
    }

    return await response.json();
  } catch (error) {
    console.error("Failed to fetch config:", error);
    return null;
  }
};
