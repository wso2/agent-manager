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

import { useMemo, useState, type ReactNode } from "react";
import {
  Alert,
  Box,
  Button,
  IconButton,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowLeft, Copy, KeyRound, Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import {
  Route,
  Routes,
  useLocation,
  useNavigate,
  useParams,
  useResolvedPath,
} from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import {
  useConfirmationDialog,
  usePipelineEnvironments,
} from "@agent-management-platform/shared-component";
import {
  useCreateWebhook,
  useDeleteWebhook,
  useListWebhooks,
  useRotateWebhookSecret,
  useTestWebhook,
  useUpdateWebhook,
} from "@agent-management-platform/api-client";
import type {
  WebhookResponse,
  WebhookTargetParams,
  WebhookTestResponse,
} from "@agent-management-platform/types";
import { WebhookForm, type EnvironmentOption } from "./WebhookForm";
import { WebhooksTable } from "./WebhooksTable";
import { WebhookDeliveriesDrawer } from "./WebhookDeliveriesDrawer";

export interface WebhooksSectionProps {
  target: WebhookTargetParams;
  canManage: boolean;
  title: string;
  description: string;
  /**
   * "page" renders in its own PageLayout, like the monitors pages; "embedded"
   * renders inside an existing layout (org settings).
   */
  layout: "page" | "embedded";
  /** For agent webhooks: the environment preselected for a new webhook. */
  currentEnvironment?: string;
  /** Where the Add Webhook button goes: the page header or the table toolbar. */
  addButtonPlacement?: "header" | "toolbar";
}

/** Shown once, right after a secret is generated. */
interface RevealedSecret {
  webhookName: string;
  secret: string;
}

/**
 * Webhooks of one scope: the list, and full-page create and edit forms at
 * "new" and ":webhookId/edit" below the current route. Mount it on a "/*"
 * route.
 */
export function WebhooksSection(props: WebhooksSectionProps) {
  const { target } = props;
  const listPath = useResolvedPath(".").pathname;
  const { data, isLoading, error } = useListWebhooks(target);
  const isAgent = target.scope === "agent";
  const pipelineEnvs = usePipelineEnvironments(
    isAgent ? target.orgName : undefined,
    isAgent ? target.projName : undefined,
  );
  const environmentOptions = useMemo<EnvironmentOption[] | undefined>(
    () =>
      isAgent
        ? pipelineEnvs.map((e) => ({ name: e.name, label: e.displayName ?? e.name }))
        : undefined,
    [isAgent, pipelineEnvs],
  );
  const shared = {
    ...props,
    listPath,
    webhooks: data?.webhooks ?? [],
    isLoading,
    error,
    environmentOptions,
  };

  return (
    <Routes>
      <Route index element={<WebhooksList {...shared} />} />
      <Route path="new" element={<WebhookEditor {...shared} />} />
      <Route path=":webhookId/edit" element={<WebhookEditor {...shared} />} />
    </Routes>
  );
}

interface SharedProps extends WebhooksSectionProps {
  listPath: string;
  webhooks: WebhookResponse[];
  isLoading: boolean;
  error: unknown;
  environmentOptions?: EnvironmentOption[];
}

function Chrome({
  layout,
  title,
  description,
  actions,
  backHref,
  children,
}: {
  layout: "page" | "embedded";
  title?: string;
  description?: string;
  actions?: ReactNode;
  backHref?: string;
  children: ReactNode;
}) {
  const navigate = useNavigate();
  if (layout === "page") {
    return (
      <PageLayout
        title={title ?? ""}
        description={description}
        disableIcon
        actions={actions}
        backHref={backHref}
        backLabel={backHref ? "Back to Webhooks" : undefined}
      >
        {children}
      </PageLayout>
    );
  }
  return (
    <Stack spacing={3}>
      {backHref && (
        <Box>
          <Button
            variant="text"
            startIcon={<ArrowLeft size={16} />}
            onClick={() => navigate(backHref)}
          >
            Back to Webhooks
          </Button>
        </Box>
      )}
      {title && (
        <Stack direction="row" justifyContent="space-between" alignItems="flex-start" gap={2}>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="h5">{title}</Typography>
            {description && (
              <Typography variant="body2" color="text.secondary">
                {description}
              </Typography>
            )}
          </Box>
          {actions}
        </Stack>
      )}
      {children}
    </Stack>
  );
}

function WebhooksList({
  target,
  canManage,
  title,
  layout,
  addButtonPlacement = "toolbar",
  webhooks,
  isLoading,
  error,
  environmentOptions,
}: SharedProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const { mutate: remove } = useDeleteWebhook(target);
  const { mutate: rotate } = useRotateWebhookSecret(target);
  const { mutate: sendTest, isPending: testing, variables: testingId } = useTestWebhook(target);
  const { addConfirmation } = useConfirmationDialog();
  const [viewing, setViewing] = useState<WebhookResponse | undefined>();
  const [rotated, setRotated] = useState<RevealedSecret | null>(null);
  const [testResult, setTestResult] = useState<
    { name: string; result: WebhookTestResponse } | null
  >(null);

  // A secret created on the form page arrives in the navigation state.
  const created = (location.state as { created?: RevealedSecret } | null)?.created ?? null;
  const secret = rotated ?? created;
  const dismissSecret = () => {
    setRotated(null);
    if (created) navigate(".", { replace: true, state: null });
  };

  const envLabel = (name: string) =>
    environmentOptions?.find((e) => e.name === name)?.label ?? name;

  const handleDelete = (w: WebhookResponse) =>
    addConfirmation({
      analytics: { entity: "webhook", action: "delete" },
      title: "Delete Webhook",
      description: `"${w.name}" will stop receiving events. Events already queued for it are dropped.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash2 size={16} />,
      onConfirm: () => remove(w.id),
    });

  const handleRotate = (w: WebhookResponse) =>
    addConfirmation({
      analytics: { entity: "webhook", action: "rotate-secret" },
      title: "Rotate Signing Secret",
      description:
        "A new secret is generated and the current one stops working immediately. " +
        "Update the receiver before the next event is sent.",
      confirmButtonText: "Rotate",
      confirmButtonIcon: <KeyRound size={16} />,
      onConfirm: () =>
        rotate(w.id, {
          onSuccess: (res) => setRotated({ webhookName: w.name, secret: res.signingSecret }),
        }),
    });

  const addButton = (
    <Button
      variant="contained"
      startIcon={<Plus />}
      disabled={!canManage}
      onClick={() => navigate("new")}
      sx={{ flexShrink: 0, whiteSpace: "nowrap" }}
    >
      Add Webhook
    </Button>
  );

  const handleTest = (w: WebhookResponse) => {
    setTestResult(null);
    sendTest(w.id, { onSuccess: (result) => setTestResult({ name: w.name, result }) });
  };

  return (
    <Chrome
      layout={layout}
      title={layout === "page" ? title : undefined}
      actions={addButtonPlacement === "header" ? addButton : undefined}
    >
      <Stack spacing={3}>
        {secret && (
          <Alert
            severity="success"
            onClose={dismissSecret}
            action={
              <IconButton
                size="small"
                aria-label="Copy signing secret"
                onClick={() => navigator.clipboard.writeText(secret.secret)}
              >
                <Copy size={16} />
              </IconButton>
            }
          >
            <Typography variant="body2" fontWeight={600}>
              Signing secret for {secret.webhookName}. Copy it now; it is not shown again.
            </Typography>
            <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
              {secret.secret}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Requests carry Standard Webhooks headers: verify webhook-signature
              (HMAC-SHA256 of &quot;id.timestamp.body&quot;) with this secret.
            </Typography>
          </Alert>
        )}
        {testResult && (
          <Alert
            severity={testResult.result.delivered ? "success" : "error"}
            onClose={() => setTestResult(null)}
          >
            {testResult.result.delivered
              ? `Test event delivered to ${testResult.name}` +
                (testResult.result.statusCode ? ` (HTTP ${testResult.result.statusCode}).` : ".")
              : `Test event to ${testResult.name} failed: ${testResult.result.error ?? "unknown error"}`}
          </Alert>
        )}
        {!canManage && (
          <Alert severity="info">You can view these webhooks but not change them.</Alert>
        )}
        <WebhooksTable
          webhooks={webhooks}
          isLoading={isLoading}
          error={error}
          canManage={canManage}
          showEnvironments={target.scope === "agent"}
          envLabel={envLabel}
          testingId={testing ? testingId : undefined}
          toolbarActions={addButtonPlacement === "toolbar" ? addButton : undefined}
          onEdit={(w) => navigate(`${w.id}/edit`)}
          onTest={handleTest}
          onDeliveries={setViewing}
          onRotate={handleRotate}
          onDelete={handleDelete}
        />
      </Stack>
      <WebhookDeliveriesDrawer
        target={target}
        webhook={viewing}
        onClose={() => setViewing(undefined)}
      />
    </Chrome>
  );
}

function WebhookEditor({
  target,
  layout,
  listPath,
  webhooks,
  isLoading,
  environmentOptions,
  currentEnvironment,
}: SharedProps) {
  const navigate = useNavigate();
  const { webhookId } = useParams<{ webhookId: string }>();
  const { mutate: create, isPending: creating } = useCreateWebhook(target);
  const { mutate: update, isPending: updating } = useUpdateWebhook(target);
  const editing = webhookId ? webhooks.find((w) => w.id === webhookId) : undefined;
  const defaultEnvironments = useMemo(
    () => (currentEnvironment ? [currentEnvironment] : []),
    [currentEnvironment],
  );
  const backToList = (state?: unknown) => navigate(listPath, { state });

  const title = webhookId ? "Edit Webhook" : "Add Webhook";
  const description = webhookId
    ? "Change where this webhook posts and which events it receives."
    : "Choose where events are sent and which events to send.";

  if (webhookId && !editing) {
    return (
      <Chrome layout={layout} title={title} backHref={listPath}>
        {isLoading ? (
          <Skeleton variant="rounded" height={320} />
        ) : (
          <Alert severity="error">This webhook no longer exists.</Alert>
        )}
      </Chrome>
    );
  }

  return (
    <Chrome layout={layout} title={title} description={description} backHref={listPath}>
      <WebhookForm
        orgName={target.orgName}
        scope={target.scope}
        webhook={editing}
        environmentOptions={environmentOptions}
        defaultEnvironments={defaultEnvironments}
        saving={creating || updating}
        submitLabel={webhookId ? "Save" : "Add Webhook"}
        onCancel={() => backToList()}
        onSubmit={(body) => {
          if (editing) {
            update({ id: editing.id, body }, { onSuccess: () => backToList() });
            return;
          }
          create(body, {
            onSuccess: (res) =>
              backToList(
                res.signingSecret
                  ? { created: { webhookName: res.name, secret: res.signingSecret } }
                  : undefined,
              ),
          });
        }}
      />
    </Chrome>
  );
}

export default WebhooksSection;
