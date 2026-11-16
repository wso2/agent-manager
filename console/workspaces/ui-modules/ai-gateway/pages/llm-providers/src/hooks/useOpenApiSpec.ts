/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useState } from "react";

const specCache = new Map<string, string>();

export function useOpenApiSpec(
  url: string | undefined,
  fallbackText?: string,
): {
  text: string;
  setText: (value: string) => void;
  /** True when `text` differs from the spec as loaded (or last marked saved). */
  isModified: boolean;
  /** Treat the current `text` as the unmodified baseline (e.g. after a save). */
  markSaved: () => void;
  /** Revert `text` to the baseline. */
  reset: () => void;
  isLoading: boolean;
  error: Error | null;
} {
  const [text, setText] = useState(fallbackText ?? "");
  // The spec as loaded from the provider or the template URL. Loading updates
  // both values together, so only user edits (e.g. importing a spec) make
  // `text` diverge from it.
  const [baseline, setBaseline] = useState(fallbackText ?? "");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const setLoaded = useCallback((value: string) => {
    setText(value);
    setBaseline(value);
  }, []);

  useEffect(() => {
    setLoaded(fallbackText ?? "");
  }, [fallbackText, setLoaded]);

  useEffect(() => {
    if (!url || (fallbackText ?? "").trim()) return;

    const cached = specCache.get(url);
    if (cached) {
      setLoaded(cached);
      return;
    }

    const controller = new AbortController();
    setIsLoading(true);
    setError(null);
    fetch(url, { signal: controller.signal })
      .then((r) => {
        if (!r.ok) {
          throw new Error(`Failed to fetch: ${r.status} ${r.statusText}`);
        }
        return r.text();
      })
      .then((t) => {
        specCache.set(url, t);
        setLoaded(t);
      })
      .catch((err) => {
        if (err?.name !== "AbortError") {
          setError(err instanceof Error ? err : new Error(String(err)));
        }
      })
      .finally(() => setIsLoading(false));

    return () => controller.abort();
  }, [url, fallbackText, setLoaded]);

  const setTextStable = useCallback((value: string) => {
    setText(value);
  }, []);

  const markSaved = useCallback(() => {
    setBaseline(text);
  }, [text]);

  const reset = useCallback(() => {
    setText(baseline);
  }, [baseline]);

  return {
    text,
    setText: setTextStable,
    isModified: text !== baseline,
    markSaved,
    reset,
    isLoading,
    error,
  };
}
