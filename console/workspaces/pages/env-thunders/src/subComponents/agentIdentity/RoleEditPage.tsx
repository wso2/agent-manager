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

import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
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
import { Folder, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  useListAgentIdentityAgents,
  useListAgentIdentityGroups,
  useGetAgentIdentityRole,
  useGetAgentIdentityRoleAssignments,
  useAddAgentIdentityRoleAssignees,
  useRemoveAgentIdentityRoleAssignees,
  useUpdateAgentIdentityRole,
  useListAgentIdentityScopes,
  useOrgAgentDisplayNames,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type AgentIdentityAgentResponse,
  type ThunderGroup,
} from "@agent-management-platform/types";
import {
  EditFormSkeleton,
  PermissionTree,
  type PermissionTreeItem,
} from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { AgentNameWithProject } from "./AgentNameWithProject";
import { useAgentLookup } from "./useAgentLookup";
import { useAssignmentDelta } from "./useAssignmentDelta";
import type { ScopeChoice } from "./scopeChoice";
import { withSearchParams } from "../../utils/withSearchParams";

type ActiveTab = "permissions" | "agents" | "groups";

// Groups assigned to a role are picked from one generous page rather than a
// dedicated "fetch all" hook — mirrors the convention used elsewhere in this
// feature area (see GroupEditPage's members picker).
const GROUPS_PAGE_SIZE = 100;

export const RoleEditPage: React.FC = () => {
  const { orgId, roleId } = useParams<{
    orgId: string;
    roleId: string;
  }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<ActiveTab>("permissions");
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const params = { orgName: orgId, envName, roleId: roleId ?? "" };

  const { data: roleData, isLoading: isLoadingRole } = useGetAgentIdentityRole(params);
  const isPermissionsReadOnly = roleData?.isReadOnly ?? false;
  const { data: assignmentsData, isLoading: isLoadingAssignments } =
    useGetAgentIdentityRoleAssignments(params);
  const { data: agentsData, isLoading: isLoadingAgents } = useListAgentIdentityAgents({
    orgName: orgId,
    envName,
  });
  const { data: groupsData, isLoading: isLoadingGroups } = useListAgentIdentityGroups(
    { orgName: orgId, envName },
    { offset: 0, limit: GROUPS_PAGE_SIZE },
  );
  const { data: scopesData, isLoading: isLoadingScopes } = useListAgentIdentityScopes({
    orgName: orgId,
    envName,
  });

  const { mutateAsync: addAssignees } = useAddAgentIdentityRoleAssignees();
  const { mutateAsync: removeAssignees } = useRemoveAgentIdentityRoleAssignees();
  const { mutateAsync: updateRole } = useUpdateAgentIdentityRole();

  // --- Derived server state ---
  const agentDisplayResolver = useOrgAgentDisplayNames({ orgName: orgId });
  const {
    agents,
    displayName,
    displayNameForAgent,
    projectDisplayName,
    projectDisplayNameForAgent,
  } = useAgentLookup(agentsData?.agents ?? [], agentDisplayResolver);
  const allGroups: ThunderGroup[] = useMemo(() => groupsData?.groups ?? [], [groupsData]);
  const catalogScopes: ScopeChoice[] = useMemo(() => scopesData?.scopes ?? [], [scopesData]);

  const initialAgentIds: string[] = useMemo(
    () => (assignmentsData?.agents ?? []).map((a) => a.id),
    [assignmentsData],
  );
  const initialGroups: ThunderGroup[] = useMemo(
    () => assignmentsData?.groups ?? [],
    [assignmentsData],
  );
  const initialScopeNames: string[] = useMemo(
    () => roleData?.permissions?.flatMap((rp) => rp.permissions) ?? [],
    [roleData],
  );

  // --- Agent tab delta tracking ---
  const agentDelta = useAssignmentDelta<AgentIdentityAgentResponse>(
    initialAgentIds,
    (a) => a.thunderAgentId as string,
  );

  // --- Group tab delta tracking ---
  const initialGroupIds = useMemo(() => initialGroups.map((g) => g.id), [initialGroups]);
  const groupDelta = useAssignmentDelta<ThunderGroup>(initialGroupIds, (g) => g.id);

  // --- Permissions tab: full selected-state approach ---
  // Held as bare scope names (not ScopeChoice objects) since every consumer
  // below — the tree, the dirty check, and the save payload — only ever
  // needs the name. A scope assigned to this role may no longer be in the
  // environment's catalog (its owning proxy may no longer be deployed here);
  // keeping just the name means it isn't silently dropped on load and still
  // round-trips correctly if the role is saved.
  const [selectedScopeIds, setSelectedScopeIds] = useState<string[]>([]);
  const hasEditedScopes = useRef(false);

  const scopeTreeItems: PermissionTreeItem[] = useMemo(
    () =>
      catalogScopes.map((s) => ({
        id: s.scope,
        path: s.scope.split(":"),
        description: s.description,
      })),
    [catalogScopes],
  );

  useEffect(() => {
    if (!hasEditedScopes.current && !isLoadingScopes) {
      setSelectedScopeIds(initialScopeNames);
    }
  }, [initialScopeNames, isLoadingScopes]);

  const rolesNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.roles;
  const rolesPath = orgId
    ? withSearchParams(generatePath(rolesNode.path, { orgId }), searchParams)
    : "#";

  // --- Derived displayed lists ---
  const displayedAgentIds = useMemo(
    () => [...agentDelta.activeIds, ...agentDelta.pendingAddIds],
    [agentDelta.activeIds, agentDelta.pendingAddIds],
  );

  const displayedGroups = useMemo(
    () => [
      ...initialGroups.filter((g) => !groupDelta.removedIds.has(g.id)),
      ...groupDelta.pendingAdds,
    ],
    [initialGroups, groupDelta.removedIds, groupDelta.pendingAdds],
  );

  const availableAgents = useMemo(
    () => agents.filter((a) => !agentDelta.excludedIds.has(a.thunderAgentId as string)),
    [agents, agentDelta.excludedIds],
  );
  const availableGroups = useMemo(
    () => allGroups.filter((g) => !groupDelta.excludedIds.has(g.id)),
    [allGroups, groupDelta.excludedIds],
  );

  const handleAddAgent = agentDelta.handleAdd;
  const handleRemoveAgent = agentDelta.handleRemove;
  const handleAddGroup = groupDelta.handleAdd;
  const handleRemoveGroup = groupDelta.handleRemove;

  // --- Permissions handler ---
  const handleScopeSelectionChange = (ids: string[]) => {
    hasEditedScopes.current = true;
    setSelectedScopeIds(ids);
  };

  // --- Save ---
  const handleSave = async () => {
    if (!orgId || !envName || !roleId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const addAgentIds = agentDelta.pendingAdds.map((a) => a.thunderAgentId as string);
      const removeAgentIdList = [...agentDelta.removedIds];
      const addGroupIds = groupDelta.pendingAdds.map((g) => g.id);
      const removeGroupIdList = [...groupDelta.removedIds];

      // None of these calls depends on another's result, so they run
      // concurrently rather than paying for round-trips one at a time.
      await Promise.all([
        addAgentIds.length > 0
          ? addAssignees({
              params,
              body: { assignments: addAgentIds.map((id) => ({ id, type: "agent" as const })) },
            })
          : null,
        removeAgentIdList.length > 0
          ? removeAssignees({
              params,
              body: {
                assignments: removeAgentIdList.map((id) => ({ id, type: "agent" as const })),
              },
            })
          : null,
        addGroupIds.length > 0
          ? addAssignees({
              params,
              body: { assignments: addGroupIds.map((id) => ({ id, type: "group" as const })) },
            })
          : null,
        removeGroupIdList.length > 0
          ? removeAssignees({
              params,
              body: {
                assignments: removeGroupIdList.map((id) => ({ id, type: "group" as const })),
              },
            })
          : null,
        // The backend reconciles add/remove scope permissions server-side from
        // the full desired set, so a single update call is enough here.
        hasEditedScopes.current && !isPermissionsReadOnly && roleData
          ? updateRole({
              params,
              body: {
                name: roleData.name,
                description: roleData.description,
                scopes: selectedScopeIds,
              },
            })
          : null,
      ]);

      setSaveSuccess(true);
      agentDelta.reset();
      groupDelta.reset();
      hasEditedScopes.current = false;
    } catch {
      setSaveError("Failed to update role. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading = isLoadingRole || isLoadingAssignments || isLoadingScopes;

  const scopesDirty = useMemo(() => {
    if (isPermissionsReadOnly) return false;
    const initial = new Set(initialScopeNames);
    return (
      initial.size !== selectedScopeIds.length ||
      selectedScopeIds.some((id) => !initial.has(id))
    );
  }, [isPermissionsReadOnly, initialScopeNames, selectedScopeIds]);

  const isDirty = scopesDirty || agentDelta.isDirty || groupDelta.isDirty;

  if (isLoading) {
    return (
      <PageLayout docs={["authorizeAgentMcpTools", "agentId", "authorization"]} title="Role" backHref={rolesPath} backLabel="Back to Roles" disableIcon>
        <EditFormSkeleton tabs={3} />
      </PageLayout>
    );
  }

  return (
    <PageLayout docs={["authorizeAgentMcpTools", "agentId", "authorization"]}
      title={roleData?.name || "Role"}
      backHref={rolesPath}
      backLabel="Back to Roles"
      description={roleData?.description}
      disableIcon
      titleTail={isPermissionsReadOnly ? <Chip label="Read-only" size="small" /> : undefined}
    >
      <Stack spacing={3}>
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && <Alert severity="success">Role updated successfully.</Alert>}

        <Form.Section>
          <Tabs
            value={activeTab}
            onChange={(_e, v) => setActiveTab(v as ActiveTab)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Permissions" value="permissions" />
            <Tab label="Agents" value="agents" />
            <Tab label="Groups" value="groups" />
          </Tabs>

          {/* ── Permissions tab ── */}
          {activeTab === "permissions" && (
            <>
              <Form.Header>Permissions</Form.Header>
              <Typography variant="body2" color="text.secondary">
                {isPermissionsReadOnly
                  ? "Permissions for predefined roles cannot be modified."
                  : "Check the scopes to assign to this role."}
              </Typography>

              <Box sx={{ mt: 1 }}>
                <PermissionTree
                  items={scopeTreeItems}
                  selectedIds={selectedScopeIds}
                  onChange={handleScopeSelectionChange}
                  readOnly={isPermissionsReadOnly}
                  emptyMessage="No scopes in the catalog."
                />
              </Box>
            </>
          )}

          {/* ── Agents tab ── */}
          {activeTab === "agents" && (
            <>
              <Form.Header>Assigned Agents</Form.Header>
              {isLoadingAgents ? (
                <CircularProgress size={20} />
              ) : (
                <>
                  <Typography variant="body2" color="text.secondary">
                    Search and add agents to this role.
                  </Typography>

                  <Box sx={{ mt: 1, mb: 2 }}>
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

                  {displayedAgentIds.length === 0 ? (
                    <ListingTable.Container>
                      <ListingTable.EmptyState
                        illustration={<Users size={64} />}
                        title="No agents assigned yet"
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
                          {displayedAgentIds.map((id) => (
                            <ListingTable.Row key={id}>
                              <ListingTable.Cell>
                                <AgentNameWithProject
                                  name={displayName(id)}
                                  projectName={projectDisplayName(id)}
                                />
                              </ListingTable.Cell>
                              <ListingTable.Cell>{id}</ListingTable.Cell>
                              <ListingTable.Cell align="right">
                                <Tooltip title="Remove from role">
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
            </>
          )}

          {/* ── Groups tab ── */}
          {activeTab === "groups" && (
            <>
              <Form.Header>Assigned Groups</Form.Header>
              {isLoadingGroups ? (
                <CircularProgress size={20} />
              ) : (
                <>
                  <Typography variant="body2" color="text.secondary">
                    Search and add groups to this role.
                  </Typography>
                  {(groupsData?.total ?? 0) > GROUPS_PAGE_SIZE && (
                    <Alert severity="warning" sx={{ mt: 1 }}>
                      Showing the first {GROUPS_PAGE_SIZE} of {groupsData?.total} groups in this
                      environment. The add-group picker below only excludes groups from this page.
                    </Alert>
                  )}

                  <Box sx={{ mt: 1, mb: 2 }}>
                    <Form.ElementWrapper label="Add Group" name="addGroup">
                      <Autocomplete
                        id="addGroup"
                        options={availableGroups}
                        getOptionLabel={(option) => (option as ThunderGroup).name}
                        onChange={handleAddGroup}
                        value={null}
                        renderInput={(autocompleteParams) => (
                          <TextField {...autocompleteParams} placeholder="Search groups..." />
                        )}
                        noOptionsText="No groups available"
                      />
                    </Form.ElementWrapper>
                  </Box>

                  {displayedGroups.length === 0 ? (
                    <ListingTable.Container>
                      <ListingTable.EmptyState
                        illustration={<Folder size={64} />}
                        title="No groups assigned yet"
                        description="Search and add groups above."
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
                          {displayedGroups.map((group) => (
                            <ListingTable.Row key={group.id}>
                              <ListingTable.Cell>{group.name}</ListingTable.Cell>
                              <ListingTable.Cell>
                                {group.description ?? "-"}
                              </ListingTable.Cell>
                              <ListingTable.Cell align="right">
                                <Tooltip title="Remove from role">
                                  <IconButton
                                    size="small"
                                    onClick={() => handleRemoveGroup(group.id)}
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
              onClick={() => navigate(rolesPath)}
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

export default RoleEditPage;
