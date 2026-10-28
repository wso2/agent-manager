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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  LLMProviderResponse,
  RouteException,
  UpdateLLMProviderRequest,
} from "@agent-management-platform/types";
import {
  AccessControlPanel,
  type AccessControlItem,
  type AccessControlMode,
  type AccessControlStatus,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import { useOpenApiSpec } from "../hooks/useOpenApiSpec";
import {
  extractResourcesFromSpec,
  getResourceKey,
  parseOpenApiSpec,
  type ResourceItem,
} from "../utils/openapiResources";

function buildOperationSpec(
  rootSpec: Record<string, unknown>,
  method: string,
  path: string,
): Record<string, unknown> | null {
  const methodKey = method.toLowerCase();
  const paths = rootSpec.paths as Record<string, unknown> | undefined;
  const pathEntry = paths?.[path] as Record<string, unknown> | undefined;
  const operation = pathEntry?.[methodKey] as
    | Record<string, unknown>
    | undefined;
  if (!operation) return null;

  const operationPathItem: Record<string, unknown> = {
    [methodKey]: operation,
  };

  const commonPathKeys = ["parameters", "servers", "summary", "description"];
  commonPathKeys.forEach((key) => {
    if (pathEntry?.[key] !== undefined) {
      operationPathItem[key] = pathEntry[key];
    }
  });

  return {
    ...(rootSpec.openapi ? { openapi: rootSpec.openapi } : {}),
    ...(rootSpec.swagger ? { swagger: rootSpec.swagger } : {}),
    ...(rootSpec.info ? { info: rootSpec.info } : {}),
    ...(rootSpec.servers ? { servers: rootSpec.servers } : {}),
    ...(rootSpec.components ? { components: rootSpec.components } : {}),
    ...(rootSpec.security ? { security: rootSpec.security } : {}),
    ...(rootSpec.tags ? { tags: rootSpec.tags } : {}),
    ...(rootSpec.basePath ? { basePath: rootSpec.basePath } : {}),
    ...(rootSpec.host ? { host: rootSpec.host } : {}),
    ...(rootSpec.schemes ? { schemes: rootSpec.schemes } : {}),
    ...(rootSpec.consumes ? { consumes: rootSpec.consumes } : {}),
    ...(rootSpec.produces ? { produces: rootSpec.produces } : {}),
    ...(rootSpec.definitions ? { definitions: rootSpec.definitions } : {}),
    ...(rootSpec.securityDefinitions
      ? { securityDefinitions: rootSpec.securityDefinitions }
      : {}),
    paths: {
      [path]: operationPathItem,
    },
  };
}

export type LLMProviderAccessControlTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

export function LLMProviderAccessControlTab({
  providerData,
  openapiSpecUrl,
  isLoading = false,
  onUpdate,
  isUpdating,
}: LLMProviderAccessControlTabProps) {
  const lastSavedRef = useRef<{
    mode: AccessControlMode;
    exceptionKeys: string[];
  } | null>(null);

  const [mode, setMode] = useState<AccessControlMode>("allow");
  const [exceptionKeys, setExceptionKeys] = useState<string[]>([]);
  const fallbackOpenapi = providerData?.openapi?.trim() ?? "";
  const {
    text: openapiText,
    setText: setOpenapiText,
    isModified: isSpecModified,
    markSaved: markSpecSaved,
    reset: resetSpec,
    isLoading: specLoading,
    error: specError,
  } = useOpenApiSpec(openapiSpecUrl, fallbackOpenapi);
  const [status, setStatus] = useState<AccessControlStatus | null>(null);

  useEffect(() => {
    if (specError) {
      setStatus({
        message: "Failed to load OpenAPI spec.",
        severity: "error",
      });
    }
  }, [specError]);

  useEffect(() => {
    if (!providerData) return;
    const rawMode = String(providerData.accessControl?.mode || "")
      .toLowerCase()
      .replace(/-/g, "_");
    const resolvedMode: AccessControlMode =
      rawMode === "deny_all" ? "deny" : "allow";
    const exceptions = providerData.accessControl?.exceptions || [];
    const exceptionItems = exceptions.flatMap((ex) =>
      (ex.methods ?? ["GET"]).map((method) => ({
        method,
        path: ex.path ?? "",
      })),
    );
    const nextExceptionKeys = exceptionItems.map((e) => getResourceKey(e));
    setMode(resolvedMode);
    setExceptionKeys(nextExceptionKeys);
    lastSavedRef.current = {
      mode: resolvedMode,
      exceptionKeys: nextExceptionKeys,
    };
  }, [providerData]);

  const parseOpenApiText = useCallback((text: string): ResourceItem[] => {
    if (!text.trim()) return [];
    const spec = parseOpenApiSpec(text);
    return spec ? extractResourcesFromSpec(spec) : [];
  }, []);

  const normalized = useMemo(() => {
    const base = parseOpenApiText(openapiText);
    return base.map((r) => ({ ...r, method: r.method.toUpperCase() }));
  }, [openapiText, parseOpenApiText]);

  const parsedOpenApiSpec = useMemo(
    () => parseOpenApiSpec(openapiText),
    [openapiText],
  );

  const items = useMemo<AccessControlItem[]>(() => {
    return normalized.map((resource) => {
      const operationSpec = parsedOpenApiSpec
        ? buildOperationSpec(parsedOpenApiSpec, resource.method, resource.path)
        : null;
      return {
        key: getResourceKey(resource),
        method: resource.method,
        path: resource.path,
        summary: resource.summary,
        operationSpec,
      };
    });
  }, [normalized, parsedOpenApiSpec]);

  const isDirty = useMemo(() => {
    const saved = lastSavedRef.current;
    if (!saved) return false;
    const currentKeys = [...exceptionKeys].sort().join("\0");
    const savedKeys = [...saved.exceptionKeys].sort().join("\0");
    // The spec counts only when the user imported one; the spec fetched from
    // the template URL is the baseline, not an unsaved change.
    return (
      mode !== saved.mode || currentKeys !== savedKeys || isSpecModified
    );
  }, [mode, exceptionKeys, isSpecModified]);

  useUnsavedChangesGuard(isDirty);

  const updateAccessControl = useCallback(async () => {
    if (!providerData) return;
    const accessControlMode = mode === "allow" ? "allow_all" : "deny_all";
    const keyToResource = new Map<string, ResourceItem>();
    normalized.forEach((r) => keyToResource.set(getResourceKey(r), r));

    const byPath = new Map<string, string[]>();
    for (const key of exceptionKeys) {
      const resource = keyToResource.get(key);
      if (!resource) continue;
      const path = resource.path ?? "";
      const methods = byPath.get(path) ?? [];
      if (!methods.includes(resource.method)) methods.push(resource.method);
      byPath.set(path, methods);
    }
    const exceptionPayload: RouteException[] = Array.from(byPath.entries()).map(
      ([path, methods]) => ({ path, methods }),
    );

    try {
      await onUpdate({
        accessControl: {
          mode: accessControlMode,
          exceptions: exceptionPayload,
        },
      });
      lastSavedRef.current = {
        mode,
        exceptionKeys: [...exceptionKeys],
      };
      markSpecSaved();
      setStatus({
        message: "Access control updated successfully.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update access control.",
        severity: "error",
      });
    }
  }, [providerData, mode, exceptionKeys, normalized, markSpecSaved, onUpdate]);

  const handleImportSpec = useCallback(
    async (file: File) => {
      try {
        const text = await file.text();
        const imported = parseOpenApiText(text);
        if (!imported.length) {
          setStatus({
            message: "No resources found in specification.",
            severity: "error",
          });
          return;
        }
        setOpenapiText(text);
        setStatus({
          message: "Specification imported. Click Save to apply.",
          severity: "success",
        });
      } catch {
        setStatus({
          message: "Failed to import specification.",
          severity: "error",
        });
      }
    },
    [parseOpenApiText, setOpenapiText],
  );

  const handleDiscard = useCallback(() => {
    const saved = lastSavedRef.current;
    if (saved) {
      setMode(saved.mode);
      setExceptionKeys([...saved.exceptionKeys]);
    } else if (providerData) {
      const rawMode = String(providerData.accessControl?.mode || "")
        .toLowerCase()
        .replace(/-/g, "_");
      const resolvedMode: AccessControlMode =
        rawMode === "deny_all" ? "deny" : "allow";
      const exceptions = providerData.accessControl?.exceptions || [];
      const exceptionItems = exceptions.flatMap((ex) =>
        (ex.methods ?? ["GET"]).map((method) => ({
          method,
          path: ex.path ?? "",
        })),
      );
      setMode(resolvedMode);
      setExceptionKeys(exceptionItems.map((e) => getResourceKey(e)));
    }
    resetSpec();
    setStatus(null);
  }, [providerData, resetSpec]);

  if (!isLoading && !specLoading && !providerData) {
    return null;
  }

  return (
    <AccessControlPanel
      items={items}
      mode={mode}
      onModeChange={setMode}
      exceptionKeys={exceptionKeys}
      onExceptionKeysChange={setExceptionKeys}
      isLoading={isLoading || specLoading}
      isSaving={isUpdating}
      isDirty={isDirty}
      onSave={() => void updateAccessControl()}
      onDiscard={handleDiscard}
      status={status}
      onClearStatus={() => setStatus(null)}
      showImportButton
      onImportSpec={handleImportSpec}
      availableEmptyTitle="No available resources"
      availableEmptyDescription="Import a specification or use a template with OpenAPI to see resources here."
    />
  );
}
