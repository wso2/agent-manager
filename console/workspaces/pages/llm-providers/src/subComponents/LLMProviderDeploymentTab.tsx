/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  LLMProviderResponse,
  UpdateLLMProviderRequest,
} from "@agent-management-platform/types";
import {
  EnvironmentGatewaySelector,
  useConfirmationDialog,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import { Alert, Button, Collapse, Skeleton, Stack } from "@wso2/oxygen-ui";

export type LLMProviderDeploymentTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  orgName: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

const sameGatewaySet = (a: string[], b: string[]): boolean => {
  const setA = new Set(a);
  const setB = new Set(b);
  return setA.size === setB.size && [...setA].every((uuid) => setB.has(uuid));
};

export function LLMProviderDeploymentTab({
  providerData,
  orgName,
  isLoading = false,
  onUpdate,
  isUpdating,
}: LLMProviderDeploymentTabProps) {
  const seededRef = useRef<{ uuid: string; gateways: string[] } | null>(null);
  const [selectedGatewayIds, setSelectedGatewayIds] = useState<string[]>([]);
  const [isSelectionValid, setIsSelectionValid] = useState(true);
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const { addConfirmation } = useConfirmationDialog();

  // Re-seed only when the server's gateway set disagrees with the last seed:
  // a background refetch that changes unrelated provider fields must not
  // discard an unsaved selection.
  useEffect(() => {
    if (!providerData) return;
    const gateways = providerData.gateways ?? [];
    const seeded = seededRef.current;
    if (
      seeded &&
      seeded.uuid === providerData.uuid &&
      sameGatewaySet(seeded.gateways, gateways)
    ) {
      return;
    }
    seededRef.current = { uuid: providerData.uuid, gateways };
    setSelectedGatewayIds(gateways);
  }, [providerData]);

  const deployedGatewayIds = useMemo(
    () => providerData?.gateways ?? [],
    [providerData],
  );

  const isDirty = useMemo(
    () => !sameGatewaySet(selectedGatewayIds, deployedGatewayIds),
    [selectedGatewayIds, deployedGatewayIds],
  );

  const handleDiscard = useCallback(() => {
    if (!providerData) return;
    setSelectedGatewayIds(providerData.gateways ?? []);
    setStatus(null);
  }, [providerData]);

  const runUpdate = useCallback(async () => {
    try {
      await onUpdate({ gateways: selectedGatewayIds });
      // The PUT response drops per-gateway deploy outcomes; only the refetched
      // GET (the mutation invalidates ["llm-provider"]) reports what actually
      // deployed. Recording the requested set as the seed makes the effect
      // re-seed from any refetch that disagrees with it, so a gateway whose
      // deploy failed reverts to undeployed.
      if (seededRef.current) {
        seededRef.current = {
          ...seededRef.current,
          gateways: selectedGatewayIds,
        };
      }
      setStatus({
        message: "Deployment updated successfully.",
        severity: "success",
      });
    } catch {
      setStatus({ message: "Failed to update deployment.", severity: "error" });
    }
  }, [onUpdate, selectedGatewayIds]);

  useUnsavedChangesGuard(isDirty);

  const handleSave = useCallback(() => {
    if (!providerData) return;
    const undeploysEverything =
      selectedGatewayIds.length === 0 &&
      (providerData.gateways ?? []).length > 0;
    if (undeploysEverything) {
      addConfirmation({
        analytics: { entity: "llm-provider", action: "undeploy" },
        title: "Undeploy from all gateways?",
        description:
          "This will undeploy the provider from all gateways. Invoke URLs " +
          "will stop working.",
        confirmButtonColor: "error",
        confirmButtonText: "Undeploy",
        onConfirm: () => void runUpdate(),
      });
      return;
    }
    void runUpdate();
  }, [providerData, selectedGatewayIds, addConfirmation, runUpdate]);

  if (isLoading) {
    return (
      <Stack spacing={1}>
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} variant="rounded" height={40} />
        ))}
      </Stack>
    );
  }

  if (!providerData) {
    return null;
  }

  return (
    <Stack spacing={2}>
      <EnvironmentGatewaySelector
        orgId={orgName ?? ""}
        value={selectedGatewayIds}
        onChange={setSelectedGatewayIds}
        lockedGatewayIds={providerData.gateways ?? []}
        onValidityChange={setIsSelectionValid}
        disabled={isUpdating}
      />
      <Stack spacing={1.5} width="100%">
        <Collapse in={!!status} timeout={300}>
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%" }}
            >
              {status.message}
            </Alert>
          )}
        </Collapse>
        <Stack direction="row" spacing={1.5} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={handleDiscard}
            disabled={!isDirty || isUpdating}
          >
            Discard
          </Button>
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={!isDirty || isUpdating || !isSelectionValid}
          >
            {isUpdating ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
