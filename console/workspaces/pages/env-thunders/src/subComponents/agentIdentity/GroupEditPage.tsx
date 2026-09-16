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
  useListAgentIdentityAgents,
  useListAgentIdentityRoles,
  useGetAgentIdentityGroup,
  useGetAgentIdentityGroupMembers,
  useGetAgentIdentityGroupRoles,
  useAddAgentIdentityGroupMembers,
  useRemoveAgentIdentityGroupMembers,
  useAddAgentIdentityRoleAssignees,
  useRemoveAgentIdentityRoleAssignees,
  useOrgAgentDisplayNames,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type AgentIdentityAgentResponse,
  type ThunderRole,
} from "@agent-management-platform/types";
import { EditFormSkeleton } from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { AgentNameWithProject } from "./AgentNameWithProject";
import { useAgentLookup } from "./useAgentLookup";
import { useAssignmentDelta } from "./useAssignmentDelta";
import { withSearchParams } from "../../utils/withSearchParams";

type ActiveTab = "agents" | "roles";

// Group members are paginated server-side, but agent-identity groups are
// expected to stay small, so one generous page stands in for "all members"
// (mirrors the simpler `limit: 100` picker convention used elsewhere in the
// identities pages, rather than adding a dedicated "fetch all" hook).
const MEMBERS_PAGE_SIZE = 100;

// Same convention for the "Add Role" picker's role catalog (mirrors
// RoleEditPage's GROUPS_PAGE_SIZE for its "Add Group" picker).
const ROLES_PAGE_SIZE = 100;

export const GroupEditPage: React.FC = () => {
  const { orgId, groupId } = useParams<{
    orgId: string;
    groupId: string;
  }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<ActiveTab>("agents");

  const params = { orgName: orgId, envName, groupId: groupId ?? "" };

  const { data: groupData, isLoading: isLoadingGroup } = useGetAgentIdentityGroup(params);
  const { data: membersData, isLoading: isLoadingMembers } = useGetAgentIdentityGroupMembers(
    params,
    { offset: 0, limit: MEMBERS_PAGE_SIZE },
  );
  const {
    data: rolesData,
    isLoading: isLoadingRoles,
    isError: isRolesError,
  } = useGetAgentIdentityGroupRoles(params);
  const { data: agentsData, isLoading: isLoadingAgents } = useListAgentIdentityAgents({
    orgName: orgId,
    envName,
  });
  const { data: allRolesData, isLoading: isLoadingAllRoles } = useListAgentIdentityRoles(
    { orgName: orgId, envName },
    { offset: 0, limit: ROLES_PAGE_SIZE },
  );

  const { mutateAsync: addMembers } = useAddAgentIdentityGroupMembers();
  const { mutateAsync: removeMembers } = useRemoveAgentIdentityGroupMembers();
  const { mutateAsync: addRoleAssignees } = useAddAgentIdentityRoleAssignees();
  const { mutateAsync: removeRoleAssignees } = useRemoveAgentIdentityRoleAssignees();

  const agentDisplayResolver = useOrgAgentDisplayNames({ orgName: orgId });
  const {
    agents,
    displayName,
    displayNameForAgent,
    projectDisplayName,
    projectDisplayNameForAgent,
  } = useAgentLookup(agentsData?.agents ?? [], agentDisplayResolver);

  const initialMemberIds: string[] = useMemo(
    () => (membersData?.members ?? []).map((m) => m.id),
    [membersData],
  );
  const initialRoles: ThunderRole[] = useMemo(() => rolesData?.roles ?? [], [rolesData]);
  const allRoles: ThunderRole[] = useMemo(() => allRolesData?.roles ?? [], [allRolesData]);

  const memberDelta = useAssignmentDelta<AgentIdentityAgentResponse>(
    initialMemberIds,
    (a) => a.thunderAgentId as string,
  );
  const { pendingAdds, removedIds } = memberDelta;

  const initialRoleIds = useMemo(() => initialRoles.map((r) => r.id), [initialRoles]);
  const roleDelta = useAssignmentDelta<ThunderRole>(initialRoleIds, (r) => r.id);

  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const groupsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.groups;
  const groupsPath = orgId
    ? withSearchParams(generatePath(groupsNode.path, { orgId }), searchParams)
    : "#";

  const displayedMemberIds = useMemo(
    () => [...memberDelta.activeIds, ...memberDelta.pendingAddIds],
    [memberDelta.activeIds, memberDelta.pendingAddIds],
  );

  const availableAgents = useMemo(
    () => agents.filter((a) => !memberDelta.excludedIds.has(a.thunderAgentId as string)),
    [agents, memberDelta.excludedIds],
  );

  const handleAddAgent = memberDelta.handleAdd;
  const handleRemoveAgent = memberDelta.handleRemove;

  const displayedRoles = useMemo(
    () => [
      ...initialRoles.filter((r) => !roleDelta.removedIds.has(r.id)),
      ...roleDelta.pendingAdds,
    ],
    [initialRoles, roleDelta.removedIds, roleDelta.pendingAdds],
  );

  const availableRoles = useMemo(
    () => allRoles.filter((r) => !roleDelta.excludedIds.has(r.id)),
    [allRoles, roleDelta.excludedIds],
  );

  const handleAddRole = roleDelta.handleAdd;
  const handleRemoveRole = roleDelta.handleRemove;

  const handleSave = async () => {
    if (!orgId || !envName || !groupId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const idsToAdd = pendingAdds
        .map((a) => a.thunderAgentId as string)
        .filter((id) => !initialMemberIds.includes(id));
      const idsToRemove = [...removedIds];
      const roleIdsToAdd = [...roleDelta.pendingAdds.map((r) => r.id)];
      const roleIdsToRemove = [...roleDelta.removedIds];

      // Each entry below is one network call; track which call each settled
      // result corresponds to so a partial failure only leaves the failed
      // part of the delta queued for retry, instead of losing track of
      // what actually saved.
      type Op =
        | { kind: "memberAdd" }
        | { kind: "memberRemove" }
        | { kind: "roleAdd"; roleId: string }
        | { kind: "roleRemove"; roleId: string };
      const ops: Op[] = [];
      const tasks: Promise<unknown>[] = [];

      if (idsToAdd.length > 0) {
        ops.push({ kind: "memberAdd" });
        tasks.push(addMembers({ params, body: { agentIds: idsToAdd } }));
      }
      if (idsToRemove.length > 0) {
        ops.push({ kind: "memberRemove" });
        tasks.push(removeMembers({ params, body: { agentIds: idsToRemove } }));
      }
      roleIdsToAdd.forEach((roleId) => {
        ops.push({ kind: "roleAdd", roleId });
        tasks.push(
          addRoleAssignees({
            params: { orgName: orgId, envName, roleId },
            body: { assignments: [{ id: groupId, type: "group" }] },
          }),
        );
      });
      roleIdsToRemove.forEach((roleId) => {
        ops.push({ kind: "roleRemove", roleId });
        tasks.push(
          removeRoleAssignees({
            params: { orgName: orgId, envName, roleId },
            body: { assignments: [{ id: groupId, type: "group" }] },
          }),
        );
      });

      const results = await Promise.allSettled(tasks);

      let memberAddFailed = false;
      let memberRemoveFailed = false;
      const failedRoleIds = new Set<string>();

      results.forEach((result, i) => {
        if (result.status === "fulfilled") return;
        const op = ops[i];
        if (op.kind === "memberAdd") memberAddFailed = true;
        else if (op.kind === "memberRemove") memberRemoveFailed = true;
        else failedRoleIds.add(op.roleId);
      });

      // Clear only the parts of the delta that were confirmed saved; failed
      // parts stay queued so the user can see and retry them.
      if (idsToAdd.length > 0 && !memberAddFailed) memberDelta.clearPendingAdds(idsToAdd);
      if (idsToRemove.length > 0 && !memberRemoveFailed) memberDelta.clearRemovedIds(idsToRemove);
      const succeededRoleAdds = roleIdsToAdd.filter((id) => !failedRoleIds.has(id));
      const succeededRoleRemoves = roleIdsToRemove.filter((id) => !failedRoleIds.has(id));
      if (succeededRoleAdds.length > 0) roleDelta.clearPendingAdds(succeededRoleAdds);
      if (succeededRoleRemoves.length > 0) roleDelta.clearRemovedIds(succeededRoleRemoves);

      const failedParts: string[] = [];
      if (memberAddFailed) failedParts.push("adding members");
      if (memberRemoveFailed) failedParts.push("removing members");
      if (failedRoleIds.size > 0) failedParts.push("updating role assignments");

      if (failedParts.length > 0) {
        setSaveError(`Failed to update group (${failedParts.join(", ")}). Please try again.`);
      } else {
        setSaveSuccess(true);
      }
    } catch {
      setSaveError("Failed to update group. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading = isLoadingGroup || isLoadingMembers || isLoadingAgents;

  if (isLoading) {
    return (
      <PageLayout docs={["authorizeAgentMcpTools", "agentId", "authorization"]} title="Group" backHref={groupsPath} backLabel="Back to Groups" disableIcon>
        <EditFormSkeleton tabs={2} />
      </PageLayout>
    );
  }

  const isDirty = memberDelta.isDirty || roleDelta.isDirty;

  return (
    <PageLayout docs={["authorizeAgentMcpTools", "agentId", "authorization"]}
      title={groupData?.name || "Group"}
      backHref={groupsPath}
      backLabel="Back to Groups"
      description={groupData?.description}
      disableIcon
    >
      <Stack spacing={3}>
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && (
          <Alert severity="success">Group updated successfully.</Alert>
        )}

        <Form.Section>
          <Tabs
            value={activeTab}
            onChange={(_e, v) => setActiveTab(v as ActiveTab)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Agents" value="agents" />
            <Tab label="Roles" value="roles" />
          </Tabs>

          {/* ── Agents tab ── */}
          {activeTab === "agents" && (
            <>
              <Form.Header>Agents</Form.Header>
              <Typography variant="body2" color="text.secondary">
                Search and add agents to this group.
              </Typography>
              {(membersData?.total ?? 0) > MEMBERS_PAGE_SIZE && (
                <Alert severity="warning" sx={{ mt: 1 }}>
                  Showing the first {MEMBERS_PAGE_SIZE} of {membersData?.total} members. The
                  add-agent picker below only excludes agents from this page.
                </Alert>
              )}

              <Box sx={{ mt: 1 }}>
                <Form.ElementWrapper label="Add Agent" name="addAgent">
                  <Autocomplete
                    id="addAgent"
                    options={availableAgents}
                    getOptionLabel={(option) =>
                      displayNameForAgent(option as AgentIdentityAgentResponse)
                    }
                    renderOption={(optionProps, option) => {
                      const { key, ...liProps } = optionProps;
                      const agent = option as AgentIdentityAgentResponse;
                      return (
                        <li key={key} {...liProps}>
                          <Box>
                            <AgentNameWithProject
                              name={displayNameForAgent(agent)}
                              projectName={projectDisplayNameForAgent(agent)}
                            />
                          </Box>
                        </li>
                      );
                    }}
                    onChange={handleAddAgent}
                    value={null}
                    renderInput={(autocompleteParams) => (
                      <TextField {...autocompleteParams} placeholder="Search agents..." />
                    )}
                    noOptionsText="No agents available"
                  />
                </Form.ElementWrapper>
              </Box>

              {displayedMemberIds.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Users size={64} />}
                    title="No members yet"
                    description="Search and add agents above."
                  />
                </ListingTable.Container>
              ) : (
                <ListingTable.Container>
                  <ListingTable>
                    <ListingTable.Head>
                      <ListingTable.Row>
                        <ListingTable.Cell>Agent</ListingTable.Cell>
                        <ListingTable.Cell>Agent ID</ListingTable.Cell>
                        <ListingTable.Cell />
                      </ListingTable.Row>
                    </ListingTable.Head>
                    <ListingTable.Body>
                      {displayedMemberIds.map((id) => (
                        <ListingTable.Row key={id}>
                          <ListingTable.Cell>
                            <AgentNameWithProject
                              name={displayName(id)}
                              projectName={projectDisplayName(id)}
                            />
                          </ListingTable.Cell>
                          <ListingTable.Cell>{id}</ListingTable.Cell>
                          <ListingTable.Cell align="right">
                            <Tooltip title="Remove from group">
                              <IconButton
                                size="small"
                                onClick={() => handleRemoveAgent(id)}
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          </ListingTable.Cell>
                        </ListingTable.Row>
                      ))}
                    </ListingTable.Body>
                  </ListingTable>
                </ListingTable.Container>
              )}
            </>
          )}

          {/* ── Roles tab ── */}
          {activeTab === "roles" && (
            <>
              <Form.Header>Assigned Roles</Form.Header>
              {isLoadingRoles || isLoadingAllRoles ? (
                <CircularProgress size={20} />
              ) : isRolesError ? (
                <Typography variant="body2" color="error">
                  Failed to load roles. Please try again.
                </Typography>
              ) : (
                <>
                  <Typography variant="body2" color="text.secondary">
                    Search and add roles to this group.
                  </Typography>
                  {(allRolesData?.total ?? 0) > ROLES_PAGE_SIZE && (
                    <Alert severity="warning" sx={{ mt: 1 }}>
                      Showing the first {ROLES_PAGE_SIZE} of {allRolesData?.total} roles in this
                      environment. The add-role picker below only excludes roles from this page.
                    </Alert>
                  )}

                  <Box sx={{ mt: 1, mb: 2 }}>
                    <Form.ElementWrapper label="Add Role" name="addRole">
                      <Autocomplete
                        id="addRole"
                        options={availableRoles}
                        getOptionLabel={(option) => (option as ThunderRole).name}
                        onChange={handleAddRole}
                        value={null}
                        renderInput={(autocompleteParams) => (
                          <TextField {...autocompleteParams} placeholder="Search roles..." />
                        )}
                        noOptionsText="No roles available"
                      />
                    </Form.ElementWrapper>
                  </Box>

                  {displayedRoles.length === 0 ? (
                    <ListingTable.Container>
                      <ListingTable.EmptyState
                        illustration={<Shield size={64} />}
                        title="No roles assigned yet"
                        description="Search and add roles above."
                      />
                    </ListingTable.Container>
                  ) : (
                    <ListingTable.Container>
                      <ListingTable>
                        <ListingTable.Head>
                          <ListingTable.Row>
                            <ListingTable.Cell>Name</ListingTable.Cell>
                            <ListingTable.Cell>Description</ListingTable.Cell>
                            <ListingTable.Cell />
                          </ListingTable.Row>
                        </ListingTable.Head>
                        <ListingTable.Body>
                          {displayedRoles.map((role) => (
                            <ListingTable.Row key={role.id}>
                              <ListingTable.Cell>{role.name}</ListingTable.Cell>
                              <ListingTable.Cell>
                                {role.description ?? "-"}
                              </ListingTable.Cell>
                              <ListingTable.Cell align="right">
                                <Tooltip title="Remove from group">
                                  <IconButton
                                    size="small"
                                    onClick={() => handleRemoveRole(role.id)}
                                  >
                                    <Trash size={16} />
                                  </IconButton>
                                </Tooltip>
                              </ListingTable.Cell>
                            </ListingTable.Row>
                          ))}
                        </ListingTable.Body>
                      </ListingTable>
                    </ListingTable.Container>
                  )}
                </>
              )}
            </>
          )}
        </Form.Section>

        {isDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => navigate(groupsPath)}
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

export default GroupEditPage;
