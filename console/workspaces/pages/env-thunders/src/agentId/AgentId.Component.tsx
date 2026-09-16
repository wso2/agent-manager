/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Avatar,
  Box,
  Button,
  Card,
  CircularProgress,
  Divider,
  Form,
  FormControl,
  IconButton,
  ListingTable,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Folder,
  HelpCircle,
  RotateCcwKey,
  Shield,
  ShieldAlert,
  ShieldOff,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import { IDENTITY_ENV_PARAM, type ThunderGroup, type ThunderRole } from "@agent-management-platform/types";
import { PageLayout, TextInput } from "@agent-management-platform/views";
import {
  useAddAgentIdentityGroupMembers,
  useAddAgentIdentityRoleAssignees,
  useListAgentIdentityAgents,
  useListAgentIdentityGroups,
  useListAgentIdentityRoles,
  useListEnvironments,
  useRemoveAgentIdentityGroupMembers,
  useRemoveAgentIdentityRoleAssignees,
} from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  monospaceInputSx,
  useAgentIdentityCredentials,
  useAgentRolesAndGroups,
  usePipelineEnvironmentsState,
  useThunderInstanceForEnv,
} from "@agent-management-platform/shared-component";
import { useAssignmentDelta } from "../subComponents/agentIdentity/useAssignmentDelta";

type IdentityItem = {
  id: string;
  name: string;
  description?: string;
};

const CATALOG_PAGE_SIZE = 100;

function IdentityAssignmentTable({
  items,
  isLoading,
  canEdit,
  removeTooltip,
  onRemove,
  emptyTitle,
  emptyDescription,
  emptyIcon,
}: {
  items: IdentityItem[];
  isLoading: boolean;
  canEdit?: boolean;
  removeTooltip?: string;
  onRemove?: (id: string) => void;
  emptyTitle: string;
  emptyDescription: string;
  emptyIcon: React.ReactNode;
}) {
  if (isLoading) {
    return (
      <Stack spacing={1}>
        <Skeleton variant="rounded" height={48} />
        <Skeleton variant="rounded" height={48} />
      </Stack>
    );
  }

  if (items.length === 0) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={emptyIcon}
          title={emptyTitle}
          description={emptyDescription}
          minHeight={160}
        />
      </ListingTable.Container>
    );
  }

  return (
    <ListingTable.Container>
      <ListingTable variant="table">
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            {canEdit && <ListingTable.Cell align="right" width="80px" />}
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {items.map((item) => (
            <ListingTable.Row key={item.id} variant="table">
              <ListingTable.Cell>
                <ListingTable.CellIcon
                  icon={
                    <Avatar sx={{ width: 28, height: 28, fontSize: 12 }}>
                      {item.name.charAt(0).toUpperCase()}
                    </Avatar>
                  }
                  primary={item.name}
                  secondary={item.description ?? undefined}
                />
              </ListingTable.Cell>
              {canEdit && onRemove && (
                <ListingTable.Cell align="right">
                  <Tooltip title={removeTooltip ?? "Remove"}>
                    <IconButton size="small" onClick={() => onRemove(item.id)}>
                      <Trash size={16} />
                    </IconButton>
                  </Tooltip>
                </ListingTable.Cell>
              )}
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
    </ListingTable.Container>
  );
}

function TabPanel({
  value,
  index,
  children,
}: {
  value: number;
  index: number;
  children: React.ReactNode;
}) {
  return (
    <Box role="tabpanel" hidden={value !== index} sx={{ px: 2, py: 2 }}>
      {value === index ? children : null}
    </Box>
  );
}

/**
 * Shared loading/error/empty fallback for this page's several independent
 * queries (identity binding, Thunder instance, environment list) — each
 * needs the same three-way triage, just with different icons/copy.
 */
const QueryStateFallback: React.FC<{
  isLoading: boolean;
  isError: boolean;
  errorTitle: string;
  errorDescription: string;
  isEmptyValue: boolean;
  emptyIcon: React.ReactNode;
  emptyTitle: string;
  emptyDescription: string;
}> = ({
  isLoading, isError, errorTitle, errorDescription,
  isEmptyValue, emptyIcon, emptyTitle, emptyDescription,
}) => {
    if (isLoading) {
      return <Skeleton variant="rounded" height={120} />;
    }
    if (isError) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<AlertTriangle size={56} />}
            title={errorTitle}
            description={errorDescription}
            minHeight={160}
          />
        </ListingTable.Container>
      );
    }
    if (isEmptyValue) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={emptyIcon}
            title={emptyTitle}
            description={emptyDescription}
            minHeight={160}
          />
        </ListingTable.Container>
      );
    }
    return null;
  };

interface AgentIdentitySectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Client ID/secret regenerate UI for one environment. The client secret is
 * never stored server-side, so the only way to see one is right after a
 * regenerate call — internal agents get it injected straight into the
 * workload instead, but the same regenerate action and client ID display
 * apply to both.
 */
const AgentIdentitySection: React.FC<AgentIdentitySectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const [activeTab, setActiveTab] = useState(0);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();

  const {
    binding, provisioned, isLoading, isError, error,
    revealed, isRegenerating, regenerate: handleRegenerate,
  } = useAgentIdentityCredentials({ orgId, projectId, agentId, envId });

  const {
    thunderInstance,
    isLoading: isLoadingThunderInstance,
    isError: isThunderInstanceError,
    error: thunderInstanceError,
  } = useThunderInstanceForEnv({ orgId, envId });

  const { data: identityAgentsData, isLoading: isLoadingIdentityAgents } =
    useListAgentIdentityAgents({ orgName: orgId, envName: envId });
  const { data: allRolesData, isLoading: isLoadingAllRoles } = useListAgentIdentityRoles(
    { orgName: orgId, envName: envId },
    { offset: 0, limit: CATALOG_PAGE_SIZE },
  );
  const { data: allGroupsData, isLoading: isLoadingAllGroups } = useListAgentIdentityGroups(
    { orgName: orgId, envName: envId },
    { offset: 0, limit: CATALOG_PAGE_SIZE },
  );

  const { mutateAsync: addRoleAssignees } = useAddAgentIdentityRoleAssignees();
  const { mutateAsync: removeRoleAssignees } = useRemoveAgentIdentityRoleAssignees();
  const { mutateAsync: addGroupMembers } = useAddAgentIdentityGroupMembers();
  const { mutateAsync: removeGroupMembers } = useRemoveAgentIdentityGroupMembers();

  const { roles, groups, isLoading: isLoadingRolesAndGroups } = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  const thunderAgentId = useMemo(
    () =>
      identityAgentsData?.agents.find(
        (item) => item.agentName === agentId && item.projectName === projectId,
      )?.thunderAgentId,
    [identityAgentsData, agentId, projectId],
  );

  const allRoles: ThunderRole[] = useMemo(() => allRolesData?.roles ?? [], [allRolesData]);
  const allGroups: ThunderGroup[] = useMemo(() => allGroupsData?.groups ?? [], [allGroupsData]);
  // The picker below fetches a single page of the catalog and filters it
  // client-side, so roles/groups beyond this page are invisible to search —
  // surface that instead of silently hiding them.
  const hasMoreRoles = (allRolesData?.total ?? 0) > allRoles.length;
  const hasMoreGroups = (allGroupsData?.total ?? 0) > allGroups.length;

  const roleIds = useMemo(() => roles.map((role) => role.id), [roles]);
  const groupIds = useMemo(() => groups.map((group) => group.id), [groups]);
  const roleDelta = useAssignmentDelta<ThunderRole>(roleIds, (role) => role.id);
  const groupDelta = useAssignmentDelta<ThunderGroup>(groupIds, (group) => group.id);

  const displayedRoles = useMemo(
    () => [
      ...roles.filter((role) => !roleDelta.removedIds.has(role.id)),
      ...roleDelta.pendingAdds,
    ],
    [roles, roleDelta.removedIds, roleDelta.pendingAdds],
  );
  const displayedGroups = useMemo(
    () => [
      ...groups.filter((group) => !groupDelta.removedIds.has(group.id)),
      ...groupDelta.pendingAdds,
    ],
    [groups, groupDelta.removedIds, groupDelta.pendingAdds],
  );
  const availableRoles = useMemo(
    () => allRoles.filter((role) => !roleDelta.excludedIds.has(role.id)),
    [allRoles, roleDelta.excludedIds],
  );
  const availableGroups = useMemo(
    () => allGroups.filter((group) => !groupDelta.excludedIds.has(group.id)),
    [allGroups, groupDelta.excludedIds],
  );

  const canEditAssignments = !!thunderAgentId && provisioned;
  const isDirty = roleDelta.isDirty || groupDelta.isDirty;

  const handleCancelChanges = () => {
    roleDelta.reset();
    groupDelta.reset();
    setSaveError(undefined);
  };

  const handleSaveChanges = async () => {
    if (!orgId || !envId || !thunderAgentId) return;
    setSaveError(undefined);
    setIsSaving(true);
    const envParams = { orgName: orgId, envName: envId };

    // Each pending mutation is tracked with its own id/name/kind so a
    // partial failure (Promise.allSettled, not Promise.all) only clears the
    // ones that actually succeeded — the rest stay queued in roleDelta /
    // groupDelta for the user to retry, instead of the whole batch being
    // silently lost or resubmitted.
    type PendingOp = {
      kind: "roleAdd" | "roleRemove" | "groupAdd" | "groupRemove";
      id: string;
      name: string;
    };

    const ops: PendingOp[] = [
      ...roleDelta.pendingAdds.map((role): PendingOp => ({ kind: "roleAdd", id: role.id, name: role.name })),
      ...[...roleDelta.removedIds].map((roleId): PendingOp => ({
        kind: "roleRemove", id: roleId, name: roles.find((role) => role.id === roleId)?.name ?? roleId,
      })),
      ...groupDelta.pendingAdds.map((group): PendingOp => ({ kind: "groupAdd", id: group.id, name: group.name })),
      ...[...groupDelta.removedIds].map((groupId): PendingOp => ({
        kind: "groupRemove", id: groupId, name: groups.find((group) => group.id === groupId)?.name ?? groupId,
      })),
    ];

    const results = await Promise.allSettled(
      ops.map((op) => {
        switch (op.kind) {
          case "roleAdd":
            return addRoleAssignees({
              params: { ...envParams, roleId: op.id },
              body: { assignments: [{ id: thunderAgentId, type: "agent" }] },
            });
          case "roleRemove":
            return removeRoleAssignees({
              params: { ...envParams, roleId: op.id },
              body: { assignments: [{ id: thunderAgentId, type: "agent" }] },
            });
          case "groupAdd":
            return addGroupMembers({
              params: { ...envParams, groupId: op.id },
              body: { agentIds: [thunderAgentId] },
            });
          case "groupRemove":
            return removeGroupMembers({
              params: { ...envParams, groupId: op.id },
              body: { agentIds: [thunderAgentId] },
            });
        }
      }),
    );

    const succeeded = {
      roleAdd: new Set<string>(),
      roleRemove: new Set<string>(),
      groupAdd: new Set<string>(),
      groupRemove: new Set<string>(),
    };
    const failed: PendingOp[] = [];
    results.forEach((result, index) => {
      const op = ops[index];
      if (result.status === "fulfilled") {
        succeeded[op.kind].add(op.id);
      } else {
        failed.push(op);
      }
    });

    roleDelta.clearPendingAdds(succeeded.roleAdd);
    roleDelta.clearRemovedIds(succeeded.roleRemove);
    groupDelta.clearPendingAdds(succeeded.groupAdd);
    groupDelta.clearRemovedIds(succeeded.groupRemove);

    if (failed.length > 0) {
      setSaveError(
        `Failed to update: ${failed.map((op) => op.name).join(", ")}. The rest were saved — ` +
        "retry to apply the remaining changes.",
      );
    }
    setIsSaving(false);
  };

  if (isLoading || isError || !binding) {
    return (
      <QueryStateFallback
        isLoading={isLoading}
        isError={isError}
        errorTitle="Failed to load agent identity"
        errorDescription={getErrorMessage(error)}
        isEmptyValue={!binding}
        emptyIcon={<ShieldOff size={56} />}
        emptyTitle="No agent identity"
        emptyDescription="This environment doesn't have an agent identity yet."
      />
    );
  }

  const isExternal = binding.provisioningType === "external";

  const clientSecretLabelAction = !isExternal ? (
    <Tooltip title="This agent's client secret is injected directly into the workload — the values above are shown for reference, but you don't need to copy anything from here to configure the agent itself.">
      <HelpCircle size={16} />
    </Tooltip>
  ) : undefined;

  let body: React.ReactNode;
  if (revealed || binding.clientId) {
    // The client ID itself isn't sensitive, unlike the secret, so it's always
    // safe to show. Outside of `revealed` (only populated once, right after
    // regenerating — the backend never stores the secret) the secret field is
    // a static placeholder, not a real masked value the user could reveal,
    // just an indicator that a secret exists.
    body = (
      <Stack spacing={1.5}>
        <TextInput
          slotProps={{ input: { readOnly: true } }}
          label="Client ID"
          value={revealed?.clientId ?? binding.clientId}
          copyable
          fullWidth
          size="small"
          sx={monospaceInputSx}
        />
        <TextInput
          slotProps={{ input: { readOnly: true } }}
          label="Client Secret"
          labelAction={clientSecretLabelAction}
          value={revealed?.clientSecret ?? "••••••••"}
          type={revealed ? "password" : undefined}
          showPasswordToggle={!!revealed}
          fullWidth
          size="small"
          sx={monospaceInputSx}
        />
        <Typography variant="body2" color="text.secondary">
          {revealed
            ? "This secret will not be shown again — copy it now."
            : "This secret was already generated and can't be shown again — regenerate to get a new one."}
        </Typography>
      </Stack>
    );
  } else if (binding.status === "failed") {
    body = (
      <Typography variant="body2" color="text.secondary">
        Provisioning failed — check the identity settings for details.
      </Typography>
    );
  } else {
    body = (
      <Typography variant="body2" color="text.secondary">
        Provisioning in progress…
      </Typography>
    );
  }

  return (
    <Card variant="outlined">
      <Tabs
        value={activeTab}
        onChange={(_event, value) => setActiveTab(value as number)}
        variant="scrollable"
        allowScrollButtonsMobile
      >
        <Tab label="Overview" />
        <Tab label="Groups" />
        <Tab label="Roles" />
      </Tabs>
      <Divider />

      {saveError != null && (
        <Alert severity="error" sx={{ mx: 2, mt: 2 }}>
          {saveError}
        </Alert>
      )}
      {!canEditAssignments && !isLoadingIdentityAgents && (
        <Alert severity="info" sx={{ mx: 2, mt: 2 }}>
          This agent has no active identity binding in this environment, so roles and groups
          cannot be changed yet.
        </Alert>
      )}

      <TabPanel value={activeTab} index={0}>
        <Stack spacing={3}>
          <Form.Section>
            <Box display="flex" alignItems="center" justifyContent="space-between">
              <Form.Subheader>Client Credentials</Form.Subheader>
              {provisioned && (
                <Button
                  variant="text"
                  size="small"
                  onClick={() => void handleRegenerate()}
                  disabled={isRegenerating}
                  startIcon={
                    isRegenerating ? <CircularProgress size={16} /> : <RotateCcwKey size={16} />
                  }
                >
                  {isRegenerating ? "Regenerating..." : "Regenerate Secret"}
                </Button>
              )}
            </Box>

            {body}

            {thunderAgentId && (
              <TextInput
                slotProps={{ input: { readOnly: true } }}
                label="Agent ID"
                value={thunderAgentId}
                copyable
                fullWidth
                size="small"
                sx={monospaceInputSx}
              />
            )}
          </Form.Section>

          <Form.Section>
            <Form.Subheader>OAuth2 Endpoints</Form.Subheader>

            {isLoadingThunderInstance || isThunderInstanceError || !thunderInstance ? (
              <QueryStateFallback
                isLoading={isLoadingThunderInstance}
                isError={isThunderInstanceError}
                errorTitle="Failed to load identity provider"
                errorDescription={getErrorMessage(thunderInstanceError)}
                isEmptyValue={!thunderInstance}
                emptyIcon={<ShieldAlert size={56} />}
                emptyTitle="No identity provider"
                emptyDescription="No identity provider found for this environment."
              />
            ) : (
              <Stack spacing={1.5}>
                <TextInput
                  slotProps={{ input: { readOnly: true } }}
                  label="Issuer URL"
                  value={thunderInstance.issuerUrl}
                  copyable
                  fullWidth
                  size="small"
                  sx={monospaceInputSx}
                />
                <TextInput
                  slotProps={{ input: { readOnly: true } }}
                  label="Token Endpoint"
                  value={thunderInstance.tokenUrl}
                  copyable
                  fullWidth
                  size="small"
                  sx={monospaceInputSx}
                />
                <TextInput
                  slotProps={{ input: { readOnly: true } }}
                  label="JWKS Endpoint"
                  value={thunderInstance.jwksUrl}
                  copyable
                  fullWidth
                  size="small"
                  sx={monospaceInputSx}
                />
              </Stack>
            )}
          </Form.Section>

          {isExternal && thunderInstance && (
            <Alert severity="info">
              <Typography variant="body2">
                Configure your agent to request a JWT token from the identity provider
                endpoints above, using the client ID and secret shown earlier, so it can
                authenticate its requests.
              </Typography>
            </Alert>
          )}
        </Stack>
      </TabPanel>

      <TabPanel value={activeTab} index={1}>
        <Stack spacing={2}>
          {canEditAssignments && (
            <Form.ElementWrapper label="Add Group" name="add-group">
              {isLoadingAllGroups ? (
                <CircularProgress size={20} />
              ) : (
                <Stack spacing={0.5}>
                  <Autocomplete
                    options={availableGroups}
                    getOptionLabel={(option) => option.name}
                    onChange={groupDelta.handleAdd}
                    value={null}
                    renderInput={(params) => <TextField {...params} placeholder="Search groups..." />}
                    noOptionsText="No groups available"
                  />
                  {hasMoreGroups && (
                    <Typography variant="caption" color="text.secondary">
                      Showing the first {CATALOG_PAGE_SIZE} groups. Refine the group name in your
                      identity provider if you can&apos;t find the one you&apos;re looking for.
                    </Typography>
                  )}
                </Stack>
              )}
            </Form.ElementWrapper>
          )}
          <IdentityAssignmentTable
            items={displayedGroups}
            isLoading={isLoadingRolesAndGroups}
            canEdit={canEditAssignments}
            removeTooltip="Remove from group"
            onRemove={groupDelta.handleRemove}
            emptyIcon={<Folder size={56} />}
            emptyTitle="No groups assigned"
            emptyDescription="This agent is not a member of any groups in this environment."
          />
        </Stack>
      </TabPanel>

      <TabPanel value={activeTab} index={2}>
        <Stack spacing={2}>
          {canEditAssignments && (
            <Form.ElementWrapper label="Add Role" name="add-role">
              {isLoadingAllRoles ? (
                <CircularProgress size={20} />
              ) : (
                <Stack spacing={0.5}>
                  <Autocomplete
                    options={availableRoles}
                    getOptionLabel={(option) => option.name}
                    onChange={roleDelta.handleAdd}
                    value={null}
                    renderInput={(params) => <TextField {...params} placeholder="Search roles..." />}
                    noOptionsText="No roles available"
                  />
                  {hasMoreRoles && (
                    <Typography variant="caption" color="text.secondary">
                      Showing the first {CATALOG_PAGE_SIZE} roles. Refine the role name in your
                      identity provider if you can&apos;t find the one you&apos;re looking for.
                    </Typography>
                  )}
                </Stack>
              )}
            </Form.ElementWrapper>
          )}
          <IdentityAssignmentTable
            items={displayedRoles}
            isLoading={isLoadingRolesAndGroups}
            canEdit={canEditAssignments}
            removeTooltip="Remove role"
            onRemove={roleDelta.handleRemove}
            emptyIcon={<Shield size={56} />}
            emptyTitle="No roles assigned"
            emptyDescription="This agent has no role assignments in this environment."
          />
        </Stack>
      </TabPanel>

      {isDirty && (
        <Box sx={{ px: 2, pb: 2 }}>
          <Stack direction="row" spacing={1}>
            <Button variant="outlined" disabled={isSaving} onClick={handleCancelChanges}>
              Cancel
            </Button>
            <Button variant="contained" disabled={isSaving} onClick={() => void handleSaveChanges()}>
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </Stack>
        </Box>
      )}
    </Card>
  );
};

/**
 * Agent-level "Agent ID" page — client ID/secret/regenerate, roles/groups,
 * and OAuth2 endpoint details for this agent's identity, one environment at
 * a time via the selector below. Linked to from the Overview page's
 * per-environment "Agent ID" section (EnvAgentRolesGroupsSection) as well as
 * the agent's own left-nav.
 */
export const AgentIdComponent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const [searchParams, setSearchParams] = useSearchParams();

  const {
    environments: pipelineEnvs,
    isLoading: isEnvironmentsLoading,
    isError: isEnvironmentsError,
  } = usePipelineEnvironmentsState(orgId, projectId);
  const envNames = useMemo(() => pipelineEnvs.map((env) => env.name), [pipelineEnvs]);
  const { data: environmentsList = [] } = useListEnvironments({ orgName: orgId });
  const getEnvDisplayName = (name: string) =>
    environmentsList.find((env) => env.name === name)?.displayName ?? name;

  const requestedEnvName = searchParams.get(IDENTITY_ENV_PARAM) ?? "";
  const envName = envNames.includes(requestedEnvName) ? requestedEnvName : (envNames[0] ?? "");

  const setSelectedEnvName = (name: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(IDENTITY_ENV_PARAM, name);
        return next;
      },
      { replace: true },
    );
  };

  // Once environments have loaded, a requested env that doesn't exist (e.g. a
  // stale/invalid deep link) falls back to envNames[0] above — normalize the
  // query param to match so the URL and the rendered environment agree,
  // rather than leaving the query param pointing at an environment that
  // isn't actually being shown.
  useEffect(() => {
    if (envNames.length > 0 && requestedEnvName && !envNames.includes(requestedEnvName)) {
      setSelectedEnvName(envName);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [envNames, requestedEnvName, envName]);

  const showEnvSelector = envNames.length > 1;

  let content: React.ReactNode;
  if (isEnvironmentsLoading || isEnvironmentsError || !envName) {
    content = (
      <QueryStateFallback
        isLoading={isEnvironmentsLoading}
        isError={isEnvironmentsError}
        errorTitle="Failed to load environments"
        errorDescription="Something went wrong while loading this agent's environments. Please try again."
        isEmptyValue={!envName}
        emptyIcon={<ShieldOff size={56} />}
        emptyTitle="No environments"
        emptyDescription="This agent isn't deployed to any environment yet."
      />
    );
  } else {
    content = (
      <AgentIdentitySection
        key={envName}
        orgId={orgId ?? ""}
        projectId={projectId ?? ""}
        agentId={agentId ?? ""}
        envId={envName}
      />
    );
  }

  return (
    <PageLayout docs={["agentId", "useAgentIdInPlatformHosted", "retrieveAgentIdForExternal"]}
      title="Agent ID"
      disableIcon
      actions={
        showEnvSelector ? (
          <FormControl size="small" sx={{ minWidth: 160 }}>
            <Select
              value={envName}
              onChange={(event) => setSelectedEnvName(event.target.value as string)}
              renderValue={(value) => (
                <Typography>
                  {getEnvDisplayName(value as string)} Environment
                </Typography>
              )}
            >
              {envNames.map((name) => (
                <MenuItem key={name} value={name}>
                  {getEnvDisplayName(name)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        ) : undefined
      }
    >
      <Stack spacing={2}>{content}</Stack>
    </PageLayout>
  );
};

export default AgentIdComponent;
