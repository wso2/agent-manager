/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import React, { useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  Form,
  IconButton,
  ListingTable,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Shield, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  useAddAgentIdentityGroupMembers,
  useAddAgentIdentityRoleAssignees,
  useGetAgent,
  useGetAgentGroups,
  useGetAgentRoles,
  useGetProject,
  useListAgentIdentityAgents,
  useListAgentIdentityGroups,
  useListAgentIdentityRoles,
  useRemoveAgentIdentityGroupMembers,
  useRemoveAgentIdentityRoleAssignees,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type ThunderGroup,
  type ThunderRole,
} from "@agent-management-platform/types";
import { EditFormSkeleton } from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { useAssignmentDelta } from "./useAssignmentDelta";
import { withSearchParams } from "../../utils/withSearchParams";

type TabId = "roles" | "groups";

type AssignableItem = { id: string; name: string; description?: string };

// Role/group catalogs are picked from one generous page rather than a
// dedicated "fetch all" hook — mirrors the convention used in
// GroupEditPage/RoleEditPage.
const CATALOG_PAGE_SIZE = 100;

// Shared shape for the two editable tabs below: a header/description, an
// optional add-picker (hidden until the catalog loads or when this agent
// can't be edited), and a table of assignments with a per-row remove button.
function AssignmentTab<T extends AssignableItem>({
  header,
  description,
  addLabel,
  addPlaceholder,
  noOptionsText,
  removeTooltip,
  emptyText,
  emptyIcon,
  canEdit,
  isLoadingCatalog,
  availableItems,
  displayedItems,
  catalogTotal,
  onAdd,
  onRemove,
}: {
  header: string;
  description: React.ReactNode;
  addLabel: string;
  addPlaceholder: string;
  noOptionsText: string;
  removeTooltip: string;
  emptyText: string;
  emptyIcon: React.ReactNode;
  canEdit: boolean;
  isLoadingCatalog: boolean;
  availableItems: T[];
  displayedItems: T[];
  catalogTotal: number;
  onAdd: (e: React.SyntheticEvent, value: T | null) => void;
  onRemove: (id: string) => void;
}) {
  return (
    <>
      <Form.Header>{header}</Form.Header>
      <Typography variant="body2" color="text.secondary">
        {description}
      </Typography>
      {catalogTotal > CATALOG_PAGE_SIZE && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          Showing the first {CATALOG_PAGE_SIZE} of {catalogTotal} in this
          environment. The add picker below only excludes items already
          listed here.
        </Alert>
      )}

      {canEdit && (
        <Box sx={{ mt: 1, mb: 2 }}>
          {isLoadingCatalog ? (
            <CircularProgress size={20} />
          ) : (
            <Form.ElementWrapper label={addLabel} name={addLabel}>
              <Autocomplete
                options={availableItems}
                getOptionLabel={(option) => option.name}
                onChange={onAdd}
                value={null}
                renderInput={(autocompleteParams) => (
                  <TextField {...autocompleteParams} placeholder={addPlaceholder} />
                )}
                noOptionsText={noOptionsText}
              />
            </Form.ElementWrapper>
          )}
        </Box>
      )}

      {displayedItems.length === 0 ? (
        <ListingTable.Container>
          <ListingTable.EmptyState illustration={emptyIcon} title={emptyText} />
        </ListingTable.Container>
      ) : (
        <ListingTable.Container>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Description</ListingTable.Cell>
                {canEdit && <ListingTable.Cell />}
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {displayedItems.map((item) => (
                <ListingTable.Row key={item.id}>
                  <ListingTable.Cell>{item.name}</ListingTable.Cell>
                  <ListingTable.Cell>{item.description ?? "-"}</ListingTable.Cell>
                  {canEdit && (
                    <ListingTable.Cell align="right">
                      <Tooltip title={removeTooltip}>
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
      )}
    </>
  );
}

export const AgentDetailPage: React.FC = () => {
  const { orgId, projectName, agentName } = useParams<{
    orgId: string;
    projectName: string;
    agentName: string;
  }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<TabId>("roles");
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const { data: rolesData, isLoading: isLoadingRoles, error: rolesError } =
    useGetAgentRoles(
      { orgName: orgId, projName: projectName, agentName },
      { environment: envName },
    );
  const { data: groupsData, isLoading: isLoadingGroups, error: groupsError } =
    useGetAgentGroups(
      { orgName: orgId, projName: projectName, agentName },
      { environment: envName },
    );
  const { data: agentData, isLoading: isLoadingAgent } = useGetAgent({
    orgName: orgId,
    projName: projectName,
    agentName,
  });
  const { data: projectData, isLoading: isLoadingProject } = useGetProject({
    orgName: orgId,
    projName: projectName,
  });
  const { data: identityAgentsData, isLoading: isLoadingIdentityAgents } =
    useListAgentIdentityAgents({ orgName: orgId, envName });
  const { data: allRolesData, isLoading: isLoadingAllRoles } =
    useListAgentIdentityRoles(
      { orgName: orgId, envName },
      { offset: 0, limit: CATALOG_PAGE_SIZE },
    );
  const { data: allGroupsData, isLoading: isLoadingAllGroups } =
    useListAgentIdentityGroups(
      { orgName: orgId, envName },
      { offset: 0, limit: CATALOG_PAGE_SIZE },
    );

  const { mutateAsync: addRoleAssignees } = useAddAgentIdentityRoleAssignees();
  const { mutateAsync: removeRoleAssignees } = useRemoveAgentIdentityRoleAssignees();
  const { mutateAsync: addGroupMembers } = useAddAgentIdentityGroupMembers();
  const { mutateAsync: removeGroupMembers } = useRemoveAgentIdentityGroupMembers();

  // The env-scoped assignment APIs identify an agent by its Thunder-bound ID,
  // not by project/agent name, so it has to be looked up from the identities
  // agents list. Agents that haven't bound to Thunder yet can't be assigned
  // roles/groups (mirrors useAgentLookup's restriction on other pages).
  const thunderAgentId = useMemo(
    () =>
      identityAgentsData?.agents.find(
        (a) => a.agentName === agentName && a.projectName === projectName,
      )?.thunderAgentId,
    [identityAgentsData, agentName, projectName],
  );

  const roles: ThunderRole[] = useMemo(() => rolesData?.roles ?? [], [rolesData]);
  const groups: ThunderGroup[] = useMemo(() => groupsData?.groups ?? [], [groupsData]);
  const allRoles: ThunderRole[] = useMemo(() => allRolesData?.roles ?? [], [allRolesData]);
  const allGroups: ThunderGroup[] = useMemo(() => allGroupsData?.groups ?? [], [allGroupsData]);

  const initialRoleIds = useMemo(() => roles.map((r) => r.id), [roles]);
  const initialGroupIds = useMemo(() => groups.map((g) => g.id), [groups]);

  const roleDelta = useAssignmentDelta<ThunderRole>(initialRoleIds, (r) => r.id);
  const groupDelta = useAssignmentDelta<ThunderGroup>(initialGroupIds, (g) => g.id);

  const displayedRoles = useMemo(
    () => [
      ...roles.filter((r) => !roleDelta.removedIds.has(r.id)),
      ...roleDelta.pendingAdds,
    ],
    [roles, roleDelta.removedIds, roleDelta.pendingAdds],
  );
  const displayedGroups = useMemo(
    () => [
      ...groups.filter((g) => !groupDelta.removedIds.has(g.id)),
      ...groupDelta.pendingAdds,
    ],
    [groups, groupDelta.removedIds, groupDelta.pendingAdds],
  );

  const availableRoles = useMemo(
    () => allRoles.filter((r) => !roleDelta.excludedIds.has(r.id)),
    [allRoles, roleDelta.excludedIds],
  );
  const availableGroups = useMemo(
    () => allGroups.filter((g) => !groupDelta.excludedIds.has(g.id)),
    [allGroups, groupDelta.excludedIds],
  );

  const agentsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.agents;
  const agentsPath = orgId
    ? withSearchParams(generatePath(agentsNode.path, { orgId }), searchParams)
    : "#";

  const handleSave = async () => {
    if (!orgId || !envName || !thunderAgentId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    const envParams = { orgName: orgId, envName };
    try {
      const addRoleIds = roleDelta.pendingAdds.map((r) => r.id);
      const removeRoleIds = [...roleDelta.removedIds];
      const addGroupIds = groupDelta.pendingAdds.map((g) => g.id);
      const removeGroupIds = [...groupDelta.removedIds];

      // None of these calls depends on another's result, so they run
      // concurrently rather than paying for round-trips one at a time.
      await Promise.all([
        ...addRoleIds.map((roleId) =>
          addRoleAssignees({
            params: { ...envParams, roleId },
            body: { assignments: [{ id: thunderAgentId, type: "agent" as const }] },
          }),
        ),
        ...removeRoleIds.map((roleId) =>
          removeRoleAssignees({
            params: { ...envParams, roleId },
            body: { assignments: [{ id: thunderAgentId, type: "agent" as const }] },
          }),
        ),
        ...addGroupIds.map((groupId) =>
          addGroupMembers({
            params: { ...envParams, groupId },
            body: { agentIds: [thunderAgentId] },
          }),
        ),
        ...removeGroupIds.map((groupId) =>
          removeGroupMembers({
            params: { ...envParams, groupId },
            body: { agentIds: [thunderAgentId] },
          }),
        ),
      ]);

      setSaveSuccess(true);
      roleDelta.reset();
      groupDelta.reset();
    } catch {
      setSaveError("Failed to update this agent's roles/groups. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  // isLoadingAllRoles/isLoadingAllGroups gate only the add-pickers within
  // each tab (see AssignmentTab's isLoadingCatalog), not the page itself —
  // the assigned-roles/assigned-groups tables don't need the catalogs.
  const isLoading =
    isLoadingRoles ||
    isLoadingGroups ||
    isLoadingAgent ||
    isLoadingProject ||
    isLoadingIdentityAgents;

  const title = agentData?.displayName || agentName || "Agent";

  if (isLoading) {
    return (
      <PageLayout docs={["agentId", "useAgentIdInPlatformHosted", "authorizeAgentMcpTools"]} title={title} backHref={agentsPath} backLabel="Back to Agents" disableIcon>
        <EditFormSkeleton tabs={2} />
      </PageLayout>
    );
  }

  const isDirty = roleDelta.isDirty || groupDelta.isDirty;
  const canEdit = !!thunderAgentId;

  return (
    <PageLayout docs={["agentId", "useAgentIdInPlatformHosted", "authorizeAgentMcpTools"]}
      title={title}
      backHref={agentsPath}
      backLabel="Back to Agents"
      description={projectData?.displayName || projectName}
      disableIcon
    >
      <Stack spacing={3}>
        {(rolesError != null || groupsError != null) && (
          <Alert severity="error">
            Failed to load this agent&apos;s roles/groups. Please try again.
          </Alert>
        )}
        {!canEdit && (
          <Alert severity="info">
            This agent has no active identity binding in {envName}, so roles
            and groups can&apos;t be assigned yet.
          </Alert>
        )}
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && <Alert severity="success">Agent updated successfully.</Alert>}

        <Form.Section>
          <Tabs
            value={activeTab}
            onChange={(_e, newValue) => setActiveTab(newValue as TabId)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Roles" value="roles" />
            <Tab label="Groups" value="groups" />
          </Tabs>

          {activeTab === "roles" && (
            <AssignmentTab
              header="Assigned Roles"
              description={
                <>Roles assigned to this agent&apos;s identity in {envName}.</>
              }
              addLabel="Add Role"
              addPlaceholder="Search roles..."
              noOptionsText="No roles available"
              removeTooltip="Remove role"
              emptyText="No roles assigned to this agent."
              emptyIcon={<Shield size={64} />}
              canEdit={canEdit}
              isLoadingCatalog={isLoadingAllRoles}
              availableItems={availableRoles}
              displayedItems={displayedRoles}
              catalogTotal={allRolesData?.total ?? 0}
              onAdd={roleDelta.handleAdd}
              onRemove={roleDelta.handleRemove}
            />
          )}

          {activeTab === "groups" && (
            <AssignmentTab
              header="Group Memberships"
              description={
                <>Groups this agent&apos;s identity belongs to in {envName}.</>
              }
              addLabel="Add Group"
              addPlaceholder="Search groups..."
              noOptionsText="No groups available"
              removeTooltip="Remove from group"
              emptyText="This agent is not a member of any groups."
              emptyIcon={<Users size={64} />}
              canEdit={canEdit}
              isLoadingCatalog={isLoadingAllGroups}
              availableItems={availableGroups}
              displayedItems={displayedGroups}
              catalogTotal={allGroupsData?.total ?? 0}
              onAdd={groupDelta.handleAdd}
              onRemove={groupDelta.handleRemove}
            />
          )}
        </Form.Section>

        {isDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => navigate(agentsPath)}
              disabled={isSaving}
            >
              Cancel
            </Button>
            <Button variant="contained" onClick={handleSave} disabled={isSaving}>
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </Stack>
        )}
      </Stack>
    </PageLayout>
  );
};

export default AgentDetailPage;
