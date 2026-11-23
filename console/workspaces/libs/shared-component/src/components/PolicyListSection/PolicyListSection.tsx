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

import React, { useCallback, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import {
  Box,
  Button,
  Card,
  Chip,
  Form,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, ShieldCheck, Trash } from "@wso2/oxygen-ui-icons-react";
import { NoDataFound } from "@agent-management-platform/views";
import {
  normalizePolicyVersionToMajor,
  useLLMPoliciesCatalog,
  type GuardrailDefinition,
  type GuardrailsCatalogResponse,
} from "@agent-management-platform/api-client";
import type { ParameterValues } from "../../utils/policyParameterEditor";
import { PolicySelectorDrawer } from "../PolicySelectorDrawer/PolicySelectorDrawer";

export type PolicySelection = {
  name: string;
  version: string;
  displayName?: string;
  settings?: Record<string, unknown>;
};

export interface PolicyListSectionProps {
  policies: PolicySelection[];
  onAdd: (policy: PolicySelection) => void;
  onEdit: (policy: PolicySelection) => void;
  onRemove: (name: string, version: string) => void;
  /** Called with the reordered list when the user drags a row to a new position. */
  onReorder?: (policies: PolicySelection[]) => void;
  catalogData?: GuardrailsCatalogResponse;
  isLoadingCatalog?: boolean;
  catalogError?: unknown;
  /**
   * Scopes the default catalog fetch to this LLM provider's deployed
   * gateways. Ignored when catalogData is supplied.
   */
  providerId?: string;
  filterPolicies?: (policies: GuardrailDefinition[]) => GuardrailDefinition[];
  /** False when the policy hub is unreachable; defaults to the catalog's own flag. */
  hubAvailable?: boolean;
  /** Defaults to the catalog's own scope. */
  availabilityScope?: "deployed" | "org_managed";
  /**
   * Whether a saved policy is the given catalog item. Defaults to name and
   * major version, so a saved "v1.0.0" matches an installed "v1.2.0" but not
   * "v2.0.0". The "not available" and edit lookups use this too.
   */
  matchApplied?: (applied: PolicySelection, item: GuardrailDefinition) => boolean;
  title?: string;
  description?: string;
  addButtonLabel?: string;
  drawerAddTitle?: string;
  drawerEditTitle?: string;
  drawerAddSubtitle?: string;
  drawerEditSubtitle?: string;
  policyNoun?: string;
  loadingLabel?: string;
  searchPlaceholder?: string;
  catalogErrorLabel?: string;
  emptySearchTitle?: string;
  emptySearchDescription?: string;
  emptyCatalogTitle?: string;
  emptyCatalogDescription?: string;
  /** Lists policies without add, edit, remove or reorder. */
  readOnly?: boolean;
}

// Name and major version: a saved "v1.0.0" keeps using an installed "v1.2.0"
// (same parameter schema), but not an installed "v2.0.0".
const matchByNameAndMajor = (
  applied: PolicySelection,
  item: GuardrailDefinition,
): boolean =>
  applied.name === item.name &&
  normalizePolicyVersionToMajor(applied.version) ===
    normalizePolicyVersionToMajor(item.version);

export const PolicyListSection: React.FC<PolicyListSectionProps> = ({
  policies,
  onAdd,
  onEdit,
  onRemove,
  onReorder,
  catalogData,
  isLoadingCatalog,
  catalogError,
  providerId,
  filterPolicies,
  hubAvailable,
  availabilityScope,
  matchApplied: matchAppliedProp,
  title = "Policies",
  description = "Add policies to enforce consistent protections.",
  addButtonLabel = "Add policy",
  drawerAddTitle = "Add Policy",
  drawerEditTitle = "Edit Policy",
  drawerAddSubtitle = "Choose a policy to configure advanced options.",
  drawerEditSubtitle = "Update the policy configuration.",
  policyNoun = "policy",
  loadingLabel,
  searchPlaceholder,
  catalogErrorLabel,
  emptySearchTitle,
  emptySearchDescription,
  emptyCatalogTitle,
  emptyCatalogDescription,
  readOnly = false,
}) => {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<PolicySelection | null>(
    null,
  );
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null);
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null);

  const reorderEnabled = !readOnly && !!onReorder;

  // The list needs the catalog to flag applied policies the gateway lacks, so
  // it owns the default fetch and hands the result to the drawer.
  const { orgId } = useParams<{ orgId: string }>();
  const {
    data: defaultCatalogData,
    isLoading: defaultIsLoadingCatalog,
    error: defaultCatalogError,
  } = useLLMPoliciesCatalog(
    orgId,
    !catalogData &&
      isLoadingCatalog === undefined &&
      catalogError === undefined &&
      !filterPolicies,
    providerId,
  );
  const activeCatalog = catalogData ?? defaultCatalogData;
  const activeIsLoading = isLoadingCatalog ?? defaultIsLoadingCatalog;
  const activeError = catalogError ?? defaultCatalogError;

  const matchApplied = matchAppliedProp ?? matchByNameAndMajor;
  const matchAvailable = matchApplied;

  // Judged against the whole catalog, not the picker's filtered view: a policy
  // the picker hides is still available on the gateway.
  // Nothing is judged unavailable while no gateway has reported (availability
  // is not known), so no warning and no forced read-only view.
  const availabilityKnown = activeCatalog?.availabilityKnown ?? true;
  const isUnavailable = useCallback(
    (policy: PolicySelection) =>
      Boolean(activeCatalog) &&
      availabilityKnown &&
      !activeCatalog!.data.some(
        (item) => item.status !== "not_installed" && matchAvailable(policy, item),
      ),
    [activeCatalog, availabilityKnown, matchAvailable],
  );

  // While availability is unknown an applied policy is edited against any
  // catalog entry with a schema (name and major); with none there is no form to
  // build, so it opens read-only, without the "not available" warning.
  const hasNoSchemaWhileUnknown = useCallback(
    (policy: PolicySelection) =>
      Boolean(activeCatalog) &&
      !availabilityKnown &&
      !activeCatalog!.data.some(
        (item) => item.parameters !== undefined && matchAvailable(policy, item),
      ),
    [activeCatalog, availabilityKnown, matchAvailable],
  );

  const matchEditing = useCallback(
    (item: GuardrailDefinition) =>
      editingPolicy ? matchAvailable(editingPolicy, item) : false,
    [editingPolicy, matchAvailable],
  );

  const isAdded = useMemo(
    () =>
      editingPolicy
        ? undefined
        : (item: GuardrailDefinition) =>
            policies.some((policy) => matchApplied(policy, item)),
    [editingPolicy, policies, matchApplied],
  );

  const handleAdd = useCallback(
    (policy: GuardrailDefinition, settings: ParameterValues) => {
      onAdd({
        name: policy.name,
        version: policy.version,
        displayName: policy.displayName,
        settings: settings as Record<string, unknown>,
      });
      setDrawerOpen(false);
      setEditingPolicy(null);
    },
    [onAdd],
  );

  // Editing keeps the saved version: opening v1 while v2 is installed shows the
  // installed schema but never upgrades the saved policy.
  const handleEdit = useCallback(
    (policy: GuardrailDefinition, settings: ParameterValues) => {
      onEdit({
        name: policy.name,
        version: editingPolicy?.version ?? policy.version,
        displayName: policy.displayName,
        settings: settings as Record<string, unknown>,
      });
      setDrawerOpen(false);
      setEditingPolicy(null);
    },
    [onEdit, editingPolicy],
  );

  const handleRowClick = useCallback((policy: PolicySelection) => {
    setEditingPolicy(policy);
    setDrawerOpen(true);
  }, []);

  const handleCloseDrawer = useCallback(() => {
    setDrawerOpen(false);
    setEditingPolicy(null);
  }, []);

  const handleDragStart = useCallback(
    (index: number) => (event: React.DragEvent<HTMLDivElement>) => {
      if (!reorderEnabled) return;
      setDraggingIndex(index);
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", String(index));
    },
    [reorderEnabled],
  );

  const handleDragOver = useCallback(
    (index: number) => (event: React.DragEvent<HTMLDivElement>) => {
      if (!reorderEnabled || draggingIndex === null) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = "move";
      if (dragOverIndex !== index) {
        setDragOverIndex(index);
      }
    },
    [reorderEnabled, draggingIndex, dragOverIndex],
  );

  const handleDrop = useCallback(
    (dropIndex: number) => (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      if (!reorderEnabled || draggingIndex === null) return;
      if (draggingIndex !== dropIndex) {
        const next = [...policies];
        const [moved] = next.splice(draggingIndex, 1);
        next.splice(dropIndex, 0, moved);
        onReorder?.(next);
      }
      setDraggingIndex(null);
      setDragOverIndex(null);
    },
    [reorderEnabled, draggingIndex, policies, onReorder],
  );

  const handleDragEnd = useCallback(() => {
    setDraggingIndex(null);
    setDragOverIndex(null);
  }, []);

  return (
    <>
      <Form.Section>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="flex-start"
          spacing={2}
        >
          <Stack spacing={0.5} sx={{ flex: 1, minWidth: 0 }}>
            <Form.Subheader>{title}</Form.Subheader>
            <Typography variant="body2" color="text.secondary">
              {description}
            </Typography>
          </Stack>
          {!readOnly && (
            <Button
              variant="outlined"
              size="small"
              startIcon={<Plus size={16} />}
              onClick={() => setDrawerOpen(true)}
            >
              {addButtonLabel}
            </Button>
          )}
        </Stack>

        {policies.length === 0 ? (
          <Box
            sx={{
              mt: 2,
              border: "1px dashed",
              borderColor: "divider",
              borderRadius: 1,
            }}
          >
            <NoDataFound
              disableBackground
              icon={<ShieldCheck size={48} />}
              message={`No ${title.toLowerCase()} added yet`}
            />
          </Box>
        ) : (
          <Stack spacing={1.25} mt={2}>
            {policies.map((policy, index) => {
              const key = `${policy.name}@${policy.version}`;
              const label = policy.displayName || policy.name;
              const isDragging = draggingIndex === index;
              const isDragOver =
                dragOverIndex === index && draggingIndex !== index;
              return (
                <Card
                  key={key}
                  variant="outlined"
                  draggable={reorderEnabled}
                  onDragStart={handleDragStart(index)}
                  onDragOver={handleDragOver(index)}
                  onDrop={handleDrop(index)}
                  onDragEnd={handleDragEnd}
                  onClick={readOnly ? undefined : () => handleRowClick(policy)}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    px: 1.5,
                    py: 1.25,
                    cursor: readOnly ? "default" : "pointer",
                    opacity: isDragging ? 0.5 : 1,
                    borderColor: isDragOver ? "primary.main" : undefined,
                    transition: "border-color 120ms ease, opacity 120ms ease",
                    "&:hover": readOnly ? undefined : { bgcolor: "action.hover" },
                  }}
                >
                  {reorderEnabled && (
                    // The position is the enforcement order, and the badge
                    // doubles as the drag handle.
                    <Tooltip title="Drag to reorder" placement="top" arrow>
                      <Box
                        aria-label={`Position ${index + 1}. Drag to reorder.`}
                        onClick={(event) => event.stopPropagation()}
                        sx={{
                          flexShrink: 0,
                          width: 24,
                          height: 24,
                          borderRadius: 1,
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                          bgcolor: "action.selected",
                          color: "text.secondary",
                          cursor: "grab",
                          "&:active": { cursor: "grabbing" },
                        }}
                      >
                        <Typography
                          variant="caption"
                          fontWeight={700}
                          sx={{ fontVariantNumeric: "tabular-nums" }}
                        >
                          {index + 1}
                        </Typography>
                      </Box>
                    </Tooltip>
                  )}
                  <Typography
                    variant="body2"
                    fontWeight={600}
                    sx={{
                      flex: 1,
                      minWidth: 0,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {label}
                  </Typography>
                  {isUnavailable(policy) && (
                    <Chip
                      label="Not available on this gateway"
                      size="small"
                      color="warning"
                      variant="outlined"
                    />
                  )}
                  {!readOnly && (
                  <Tooltip title="Remove" placement="top" arrow>
                    <IconButton
                      size="small"
                      color="error"
                      aria-label={`Remove ${label}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        onRemove(policy.name, policy.version);
                      }}
                    >
                      <Trash size={14} />
                    </IconButton>
                  </Tooltip>
                  )}
                </Card>
              );
            })}
          </Stack>
        )}
      </Form.Section>

      <PolicySelectorDrawer
        open={drawerOpen}
        onClose={handleCloseDrawer}
        onSubmit={editingPolicy ? handleEdit : handleAdd}
        catalogData={activeCatalog}
        isLoadingCatalog={activeIsLoading}
        catalogError={activeError}
        filterPolicies={filterPolicies}
        hubAvailable={hubAvailable}
        availabilityScope={availabilityScope}
        isAdded={isAdded}
        matchEdit={matchEditing}
        unavailablePolicy={
          editingPolicy &&
          (isUnavailable(editingPolicy) || hasNoSchemaWhileUnknown(editingPolicy))
            ? editingPolicy
            : undefined
        }
        existingSettings={
          editingPolicy
            ? (editingPolicy.settings as Record<string, unknown>)
            : undefined
        }
        editPolicyKey={
          editingPolicy
            ? `${editingPolicy.name}@${editingPolicy.version}`
            : undefined
        }
        title={editingPolicy ? drawerEditTitle : drawerAddTitle}
        subtitle={editingPolicy ? drawerEditSubtitle : drawerAddSubtitle}
        policyNoun={policyNoun}
        loadingLabel={loadingLabel}
        searchPlaceholder={searchPlaceholder}
        catalogErrorLabel={catalogErrorLabel}
        emptySearchTitle={emptySearchTitle}
        emptySearchDescription={emptySearchDescription}
        emptyCatalogTitle={emptyCatalogTitle}
        emptyCatalogDescription={emptyCatalogDescription}
        minWidth={800}
        maxWidth={800}
      />
    </>
  );
};

export default PolicyListSection;
