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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { lazy, Suspense, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Typography } from "@wso2/oxygen-ui";
import { ShieldCheck, Trash } from "@wso2/oxygen-ui-icons-react";
import { useDeleteA2AProxy, useRefreshA2AProxy, useSetA2AProxy } from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import type { A2AProxyResponse, A2AProxyStatus, AgentCardPathParams } from "@agent-management-platform/types";
import { A2AProxyFields } from "./A2AProxyFields";
import {
  draftChanged,
  draftFromProxy,
  dropsStoredSecret,
  hasStoredSecret,
  proxyRequestOf,
  upstreamAuthError,
} from "./a2aProxyDraft";

const ProxySecurityDrawer = lazy(() =>
  import("@agent-management-platform/deploy").then((m) => ({ default: m.ProxySecurityDrawer })),
);

const PROXY_CHIP: Record<A2AProxyStatus, { label: string; color: "warning" | "success" | "error" }> = {
  pending: { label: "Publishing to gateway", color: "warning" },
  published: { label: "Routed through gateway", color: "success" },
  failed: { label: "Gateway publish failed", color: "error" },
};

interface EnvA2AProxySectionProps {
  params: AgentCardPathParams;
  /** Undefined while the environment is direct; render only once that is known. */
  proxy?: A2AProxyResponse;
  /** A routing save for this environment is in flight, from this section or another form. */
  saving?: boolean;
}

/** Stands in for the routing controls until the environment's routing is known. */
export function A2AProxyUnknown({ failed }: { failed: boolean }) {
  return failed ? (
    <Typography variant="body2" color="error" sx={{ mb: 2 }}>Couldn&apos;t load gateway routing.</Typography>
  ) : (
    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>Loading gateway routing…</Typography>
  );
}

/** Gateway routing for one environment of an external A2A agent that has a registered card URL. */
export function EnvA2AProxySection({ params, proxy, saving = false }: EnvA2AProxySectionProps) {
  const [draft, setDraft] = useState(() => draftFromProxy(proxy));
  const [securityOpen, setSecurityOpen] = useState(false);
  const { mutate: saveProxy, isPending: isSaving } = useSetA2AProxy();
  const { mutate: removeProxy, isPending: isRemoving } = useDeleteA2AProxy();
  const { mutate: republish, isPending: isRepublishing } = useRefreshA2AProxy();
  const { addConfirmation } = useConfirmationDialog();
  const proxied = !!proxy;
  const savedHeader = proxy?.upstreamAuthHeader ?? "";

  // Re-seed when the saved routing changes, not on every status poll, so typing is not overwritten.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => setDraft(draftFromProxy(proxy)), [proxied, savedHeader]);

  const error = upstreamAuthError(draft, proxy);
  const busy = isSaving || isRemoving || saving;
  const canSave = draftChanged(draft, proxy) && !error && !busy;

  // The secret is write-only: once saved it is forgotten here, so the field cannot resubmit it.
  const submitDraft = () => saveProxy(
    { params, body: proxyRequestOf(draft, proxy) },
    {
      onSuccess: () => setDraft((d) => (
        { ...d, value: "", secret: d.header.trim() ? "keep" : "replace" }
      )),
    },
  );

  const save = () => {
    if (!canSave) return;
    if (draft.enabled && !dropsStoredSecret(draft, proxy)) {
      submitDraft();
      return;
    }
    addConfirmation(draft.enabled
      ? {
          analytics: { entity: "a2a-proxy", action: "clear-upstream-secret" },
          title: "Remove the stored upstream value?",
          description: "The gateway stops sending the header to the agent, and the value cannot be recovered.",
          confirmButtonText: "Remove value",
          confirmButtonColor: "error",
          confirmButtonIcon: <Trash size={16} />,
          onConfirm: submitDraft,
        }
      : {
          analytics: { entity: "a2a-proxy", action: "disable" },
          title: "Stop routing through the gateway?",
          description: "Callers need the agent's own address again, gateway API keys and CORS stop applying, and any stored upstream value is removed.",
          confirmButtonText: "Stop routing",
          confirmButtonColor: "error",
          confirmButtonIcon: <Trash size={16} />,
          onConfirm: () => removeProxy(params),
        });
  };

  return (
    <Box sx={{ mb: 2 }}>
      <Box display="flex" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography variant="overline" color="text.secondary">Gateway routing</Typography>
        {proxy && (
          <Chip size="small" variant="outlined" label={PROXY_CHIP[proxy.status].label} color={PROXY_CHIP[proxy.status].color} />
        )}
        {proxy && (
          <Button size="small" variant="text" startIcon={<ShieldCheck size={14} />} onClick={() => setSecurityOpen(true)}>
            Security settings
          </Button>
        )}
      </Box>
      {proxy?.status === "failed" && (
        <Alert
          severity="error"
          sx={{ mb: 1 }}
          action={
            <Button size="small" color="inherit" disabled={isRepublishing} onClick={() => republish(params)}>Retry</Button>
          }
        >
          {proxy.lastError || "The gateway publish failed."}
        </Alert>
      )}
      <A2AProxyFields
        draft={draft}
        onChange={setDraft}
        hasStoredSecret={hasStoredSecret(proxy)}
        error={error}
        disabled={busy}
      />
      <Box display="flex" justifyContent="flex-end" sx={{ mt: 1 }}>
        <Button size="small" variant="contained" disabled={!canSave} onClick={save}>Save</Button>
      </Box>
      {proxy && securityOpen && (
        <Suspense fallback={null}>
          <ProxySecurityDrawer
            open
            onClose={() => setSecurityOpen(false)}
            orgName={params.orgName ?? ""}
            projName={params.projName ?? ""}
            agentName={params.agentName ?? ""}
            environment={params.envId ?? ""}
          />
        </Suspense>
      )}
    </Box>
  );
}
