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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Form,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ArrowLeft,
  Check,
  Circle,
  Search,
  Braces,
  ShieldCheck,
} from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import {
  useLLMPoliciesCatalog,
  filterGuardrailPolicies,
  type GuardrailDefinition,
  type GuardrailsCatalogResponse,
} from "@agent-management-platform/api-client";
import { CodeBlock } from "../CodeBlock";
import PolicyParameterEditor from "../PolicyParameterEditor/PolicyParameterEditor";
import type { ParameterValues } from "../../utils/policyParameterEditor";
import {
  useConfirmIfUnsaved,
  useUnsavedChangesGuard,
} from "../../utils/useUnsavedChangesGuard";
import { resolvePolicyDetail } from "./resolvePolicyDetail";

const unavailableMessage = (policyNoun: string) =>
  `This ${policyNoun} is not available on this gateway, so its configuration is read-only. Remove it from the list if it is no longer needed.`;

const PolicyDetailView: React.FC<{
  policy: GuardrailDefinition;
  /** The catalog no longer has this policy installed; the form stays but cannot be submitted. */
  unavailable?: boolean;
  existingSettings?: Record<string, unknown>;
  policyNoun?: string;
  onBack: () => void;
  /** Explicit discard from the editor; defaults to `onBack`. */
  onCancel?: () => void;
  onDirtyChange?: (dirty: boolean) => void;
  onSubmit: (policy: GuardrailDefinition, settings: ParameterValues) => void;
}> = ({
  policy,
  unavailable = false,
  existingSettings,
  policyNoun = "policy",
  onBack,
  onCancel = onBack,
  onDirtyChange,
  onSubmit,
}) => {
  const policyDefinition = useMemo(() => resolvePolicyDetail(policy), [policy]);

  if (!policyDefinition) {
    return (
      <Stack spacing={2}>
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Braces size={64} />}
            title="No definition available"
            description={`This ${policyNoun} does not have a configuration schema.`}
          />
        </ListingTable.Container>
        <Button
          variant="text"
          startIcon={<ArrowLeft size={16} />}
          onClick={onBack}
        >
          Back
        </Button>
      </Stack>
    );
  }

  return (
    <Stack spacing={2}>
      <Box>
        <Button
          variant="text"
          size="small"
          startIcon={<ArrowLeft size={16} />}
          onClick={onBack}
        >
          Back
        </Button>
      </Box>
      {unavailable && (
        <Alert severity="warning">{unavailableMessage(policyNoun)}</Alert>
      )}
      <PolicyParameterEditor
        submitDisabled={unavailable}
        policyDefinition={policyDefinition}
        policyDisplayName={policy.displayName || policy.name}
        existingValues={existingSettings}
        isEditMode={Boolean(existingSettings)}
        onCancel={onCancel}
        onDirtyChange={onDirtyChange}
        onSubmit={(values) => onSubmit(policy, values)}
      />
    </Stack>
  );
};

/** Current parameters of an applied policy the gateway no longer offers. */
const UnavailablePolicyView: React.FC<{
  settings?: Record<string, unknown>;
  policyNoun: string;
  onClose: () => void;
}> = ({ settings, policyNoun, onClose }) => (
  <Stack spacing={2}>
    <Alert severity="warning">
      {unavailableMessage(policyNoun)}
    </Alert>
    <CodeBlock
      code={JSON.stringify(settings ?? {}, null, 2)}
      language="json"
      showCopyButton={false}
    />
    <Box>
      <Button variant="outlined" onClick={onClose}>
        Close
      </Button>
    </Box>
  </Stack>
);

export type PolicySelectorDrawerProps = {
  open: boolean;
  onClose: () => void;
  onSubmit: (policy: GuardrailDefinition, settings: ParameterValues) => void;
  catalogData?: GuardrailsCatalogResponse;
  isLoadingCatalog?: boolean;
  catalogError?: unknown;
  /**
   * Scopes the default catalog fetch to this LLM provider's deployed
   * gateways. Ignored when catalogData is supplied.
   */
  providerId?: string;
  filterPolicies?: (policies: GuardrailDefinition[]) => GuardrailDefinition[];
  /** Marks catalog rows that are already added; they are shown selected and disabled. */
  isAdded?: (policy: GuardrailDefinition) => boolean;
  /** False when the policy hub is unreachable; defaults to the catalog's own flag. */
  hubAvailable?: boolean;
  /** Defaults to the catalog's own scope. */
  availabilityScope?: "deployed" | "org_managed";
  /** Existing settings when editing (e.g. for pre-filling form) */
  existingSettings?: Record<string, unknown>;
  /** When set, skip the catalog list and go directly to editing this policy (name@version) */
  editPolicyKey?: string;
  /**
   * Finds the catalog item for the policy being edited; defaults to comparing
   * `name@version` with `editPolicyKey`. Only installed items are matched.
   */
  matchEdit?: (policy: GuardrailDefinition) => boolean;
  /**
   * An applied policy with no installed catalog item: its `existingSettings`
   * are shown read-only instead of an editor.
   */
  unavailablePolicy?: { name: string; displayName?: string };
  title?: string;
  subtitle?: string;
  policyNoun?: string;
  loadingLabel?: string;
  searchPlaceholder?: string;
  catalogErrorLabel?: string;
  emptySearchTitle?: string;
  emptySearchDescription?: string;
  emptyCatalogTitle?: string;
  emptyCatalogDescription?: string;
  minWidth?: number;
  maxWidth?: number;
};

export function PolicySelectorDrawer({
  open,
  onClose,
  onSubmit,
  catalogData,
  isLoadingCatalog: controlledIsLoadingCatalog,
  catalogError: controlledCatalogError,
  providerId,
  filterPolicies,
  isAdded,
  hubAvailable: hubAvailableProp,
  availabilityScope: availabilityScopeProp,
  existingSettings,
  editPolicyKey,
  matchEdit,
  unavailablePolicy,
  title = "Policies",
  subtitle = "Choose a policy to configure advanced options.",
  policyNoun = "policy",
  loadingLabel = "Loading policies...",
  searchPlaceholder = "Search policies",
  catalogErrorLabel = "Failed to load policies.",
  emptySearchTitle = "No policies match your search",
  emptySearchDescription = "Try a different keyword or clear the search filter.",
  emptyCatalogTitle = "No policies available",
  emptyCatalogDescription = "No policies are available in the catalog.",
  minWidth = 600,
  maxWidth = 800,
}: PolicySelectorDrawerProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const {
    data: defaultCatalogData,
    isLoading: defaultIsLoadingCatalog,
    error: defaultCatalogError,
  } = useLLMPoliciesCatalog(
    orgId,
    !catalogData &&
      controlledIsLoadingCatalog === undefined &&
      controlledCatalogError === undefined &&
      !filterPolicies,
    providerId,
  );

  const [selectedPolicy, setSelectedPolicy] =
    useState<GuardrailDefinition | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  // Parameter values live inside PolicyParameterEditor, which reports whether
  // they differ from the defaults (create) or the saved settings (edit).
  const [isEditorDirty, setIsEditorDirty] = useState(false);
  const isFormDirty = open && selectedPolicy !== null && isEditorDirty;
  useUnsavedChangesGuard(isFormDirty);
  const confirmIfUnsaved = useConfirmIfUnsaved();

  const activeCatalogData = catalogData ?? defaultCatalogData;
  const isLoadingCatalog =
    controlledIsLoadingCatalog ?? defaultIsLoadingCatalog;
  const catalogError = controlledCatalogError ?? defaultCatalogError;

  const availablePolicies = useMemo(() => {
    const policies = activeCatalogData?.data ?? [];
    return filterPolicies
      ? filterPolicies(policies)
      : filterGuardrailPolicies(policies);
  }, [activeCatalogData, filterPolicies]);

  const hubAvailable = hubAvailableProp ?? activeCatalogData?.hubAvailable ?? true;
  const availabilityScope =
    availabilityScopeProp ?? activeCatalogData?.availabilityScope ?? "deployed";

  // Installed rows first; the sort is stable, so catalog order holds within each.
  const orderedPolicies = useMemo(
    () => [
      ...availablePolicies.filter((p) => p.status !== "not_installed"),
      ...availablePolicies.filter((p) => p.status === "not_installed"),
    ],
    [availablePolicies],
  );

  const filteredPolicies = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return orderedPolicies;
    return orderedPolicies.filter(
      (p) =>
        (p.displayName || p.name).toLowerCase().includes(q) ||
        p.name.toLowerCase().includes(q) ||
        p.description?.toLowerCase().includes(q),
    );
  }, [orderedPolicies, searchQuery]);

  const handleClose = useCallback(() => {
    setSelectedPolicy(null);
    setSearchQuery("");
    onClose();
  }, [onClose]);

  // Backdrop, header X and Back discard the picked policy's parameters, so
  // they confirm first; the editor's Cancel stays an explicit discard.
  const handleGuardedClose = useCallback(
    () => confirmIfUnsaved(handleClose, isFormDirty),
    [confirmIfUnsaved, handleClose, isFormDirty],
  );
  const handleGuardedBack = useCallback(
    () => confirmIfUnsaved(() => setSelectedPolicy(null), isFormDirty),
    [confirmIfUnsaved, isFormDirty],
  );

  // Revalidated on every catalog change: a selected policy with no installed
  // catalog entry keeps its form but cannot be submitted. Edit mode matches the
  // way the policy was opened; add mode matches the exact picked item.
  const selectedUnavailable = useMemo(() => {
    if (!selectedPolicy || !activeCatalogData) return false;
    return !activeCatalogData.data.some(
      (p) =>
        p.status !== "not_installed" &&
        (editPolicyKey
          ? matchEdit
            ? matchEdit(p)
            : `${p.name}@${p.version}` === editPolicyKey
          : p.name === selectedPolicy.name &&
            p.version === selectedPolicy.version),
    );
  }, [selectedPolicy, activeCatalogData, editPolicyKey, matchEdit]);

  const handleSubmit = useCallback(
    (policy: GuardrailDefinition, settings: ParameterValues) => {
      if (selectedUnavailable) return;
      onSubmit(policy, settings);
      setSelectedPolicy(null);
    },
    [onSubmit, selectedUnavailable],
  );

  useEffect(() => {
    if (!open) {
      setSelectedPolicy(null);
      setSearchQuery("");
    }
  }, [open]);

  // Auto-select policy when opening in edit mode. Looks at the whole catalog,
  // not just the picker's filtered list, so a policy the picker hides still
  // opens for editing.
  useEffect(() => {
    if (!open || !editPolicyKey) return;
    const match = (activeCatalogData?.data ?? []).find(
      (p) =>
        p.status !== "not_installed" &&
        (matchEdit
          ? matchEdit(p)
          : `${p.name}@${p.version}` === editPolicyKey),
    );
    if (match) {
      // A catalog refresh that leaves the schema unchanged keeps the current
      // object, so the editor does not reset the user's unsaved edits.
      setSelectedPolicy((prev) =>
        prev &&
        prev.name === match.name &&
        JSON.stringify([prev.parameters, prev.systemParameters]) ===
          JSON.stringify([match.parameters, match.systemParameters])
          ? prev
          : match,
      );
    }
  }, [open, editPolicyKey, matchEdit, activeCatalogData]);

  const drawerTitle = selectedPolicy
    ? selectedPolicy.displayName || selectedPolicy.name
    : unavailablePolicy
      ? unavailablePolicy.displayName || unavailablePolicy.name
      : title;

  return (
    <DrawerWrapper
      open={open}
      onClose={handleGuardedClose}
      minWidth={minWidth}
      maxWidth={maxWidth}
    >
      <DrawerHeader
        icon={<ShieldCheck size={24} />}
        title={drawerTitle}
        onClose={handleGuardedClose}
      />
      <DrawerContent>
        {!selectedPolicy && !unavailablePolicy && (
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {subtitle}
          </Typography>
        )}

        {isLoadingCatalog ? (
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              {loadingLabel}
            </Typography>
            {Array.from({ length: 5 }).map((_, i) => (
              <Card key={i} variant="outlined">
                <Box sx={{ p: 1.5 }}>
                  <Stack spacing={0.75}>
                    <Skeleton variant="text" width="45%" height={20} />
                    <Skeleton variant="text" width="85%" height={16} />
                    <Skeleton variant="text" width="65%" height={16} />
                  </Stack>
                </Box>
              </Card>
            ))}
          </Stack>
        ) : catalogError ? (
          <Alert severity="error" sx={{ mt: 1 }}>
            {catalogErrorLabel} {(catalogError as Error)?.message}
          </Alert>
        ) : unavailablePolicy && !selectedPolicy ? (
          <UnavailablePolicyView
            settings={existingSettings}
            policyNoun={policyNoun}
            onClose={handleClose}
          />
        ) : !selectedPolicy ? (
          <Stack spacing={2}>
            {availabilityScope === "org_managed" && (
              <Alert severity="info">
                {"Availability checked against managed gateways; will be rechecked on deploy."}
              </Alert>
            )}
            {!hubAvailable && (
              <Alert severity="info">
                Policy hub unreachable — showing installed policies only.
              </Alert>
            )}
            <SearchBar
              placeholder={searchPlaceholder}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              size="small"
              fullWidth
            />
            <Stack spacing={1.25}>
              {filteredPolicies.map((policy) => {
                const notInstalled = policy.status === "not_installed";
                const added = !notInstalled && Boolean(isAdded?.(policy));
                const unselectable = added || notInstalled;
                return (
                  <Form.CardButton
                    key={`${policy.name}@${policy.version}`}
                    selected={added}
                    disabled={unselectable}
                    aria-disabled={unselectable}
                    onClick={() => !unselectable && setSelectedPolicy(policy)}
                    sx={{
                      width: "100%",
                      justifyContent: "flex-start",
                      opacity: notInstalled ? 0.6 : 1,
                      cursor: unselectable ? "not-allowed" : undefined,
                    }}
                  >
                    <CardContent>
                      <Stack spacing={1}>
                        <Stack
                          direction="row"
                          spacing={0.5}
                          alignItems="center"
                        >
                          <Avatar
                            sx={{
                              height: 32,
                              width: 32,
                              backgroundColor: added
                                ? "primary.main"
                                : "secondary.main",
                              color: added ? "common.white" : "text.secondary",
                            }}
                          >
                            {added ? <Check size={16} /> : <Circle size={16} />}
                          </Avatar>
                          <Typography variant="body2" fontWeight={500}>
                            {policy.displayName || policy.name}
                          </Typography>
                          <Chip
                            label={policy.version}
                            size="small"
                            variant="outlined"
                          />
                          {policy.source === "custom" && (
                            <Chip label="Custom" size="small" color="info" />
                          )}
                        </Stack>
                        {notInstalled && (
                          <Typography variant="caption" color="text.secondary">
                            {policy.missingOn?.length
                              ? `Missing on ${policy.missingOn
                                  .map((g) => g.name)
                                  .join(", ")}`
                              : "Not installed on this gateway"}
                          </Typography>
                        )}
                        {policy.description && (
                          <Tooltip title={policy.description}>
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              {policy.description.substring(0, 200)}
                              {policy.description.length > 200 ? "..." : ""}
                            </Typography>
                          </Tooltip>
                        )}
                      </Stack>
                    </CardContent>
                  </Form.CardButton>
                );
              })}
              {filteredPolicies.length === 0 && searchQuery && (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Search size={64} />}
                    title={emptySearchTitle}
                    description={emptySearchDescription}
                  />
                </ListingTable.Container>
              )}
              {filteredPolicies.length === 0 && !searchQuery && (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<ShieldCheck size={64} />}
                    title={emptyCatalogTitle}
                    description={emptyCatalogDescription}
                  />
                </ListingTable.Container>
              )}
            </Stack>
          </Stack>
        ) : (
          <PolicyDetailView
            policy={selectedPolicy}
            unavailable={selectedUnavailable}
            existingSettings={existingSettings}
            policyNoun={policyNoun}
            onBack={editPolicyKey ? handleGuardedClose : handleGuardedBack}
            onDirtyChange={setIsEditorDirty}
            onCancel={
              editPolicyKey ? handleClose : () => setSelectedPolicy(null)
            }
            onSubmit={handleSubmit}
          />
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}
