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
  ShieldAlert,
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
import PolicyParameterEditor from "../PolicyParameterEditor/PolicyParameterEditor";
import type { ParameterValues } from "../../utils/policyParameterEditor";
import {
  useConfirmIfUnsaved,
  useUnsavedChangesGuard,
} from "../../utils/useUnsavedChangesGuard";
import { resolvePolicyDetail } from "./resolvePolicyDetail";

const PolicyDetailView: React.FC<{
  policy: GuardrailDefinition;
  existingSettings?: Record<string, unknown>;
  onBack: () => void;
  /** Explicit discard from the editor; defaults to `onBack`. */
  onCancel?: () => void;
  onDirtyChange?: (dirty: boolean) => void;
  onSubmit: (policy: GuardrailDefinition, settings: ParameterValues) => void;
}> = ({
  policy,
  existingSettings,
  onBack,
  onCancel = onBack,
  onDirtyChange,
  onSubmit,
}) => {
  const policyDefinition = useMemo(() => resolvePolicyDetail(policy), [policy]);

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
      {policy.requiredSystemConfig.length > 0 && (
        <Alert severity="warning">
          This policy needs gateway configuration. Make sure these keys are set
          on the gateway it is deployed to, or the policy will not work:{" "}
          {policy.requiredSystemConfig.join(", ")}
        </Alert>
      )}
      <PolicyParameterEditor
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
  /** Policy names that are already added - disable in list (e.g. create flow) */
  disabledPolicyNames?: string[];
  /** Policy keys (name@version) that are already added - more precise than names */
  disabledPolicyKeys?: string[];
  /** Existing settings when editing (e.g. for pre-filling form) */
  existingSettings?: Record<string, unknown>;
  /** When set, skip the catalog list and go directly to editing this policy (name@version) */
  editPolicyKey?: string;
  title?: string;
  subtitle?: string;
  /**
   * @deprecated No longer read: it only labelled the removed definition-loading
   * error states. Kept so existing callers compile.
   */
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
  disabledPolicyNames = [],
  disabledPolicyKeys = [],
  existingSettings,
  editPolicyKey,
  title = "Policies",
  subtitle = "Choose a policy to configure advanced options.",
  loadingLabel = "Loading policies...",
  searchPlaceholder = "Search policies...",
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

  const isDisabled = useCallback(
    (name: string, version: string) =>
      disabledPolicyKeys.length > 0
        ? disabledPolicyKeys.includes(`${name}@${version}`)
        : disabledPolicyNames.includes(name),
    [disabledPolicyKeys, disabledPolicyNames],
  );

  const filteredPolicies = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return availablePolicies;
    return availablePolicies.filter(
      (p) =>
        (p.displayName || p.name).toLowerCase().includes(q) ||
        p.name.toLowerCase().includes(q) ||
        p.description?.toLowerCase().includes(q),
    );
  }, [availablePolicies, searchQuery]);

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

  const handleSubmit = useCallback(
    (policy: GuardrailDefinition, settings: ParameterValues) => {
      onSubmit(policy, settings);
      setSelectedPolicy(null);
    },
    [onSubmit],
  );

  useEffect(() => {
    if (!open) {
      setSelectedPolicy(null);
      setSearchQuery("");
    }
  }, [open]);

  // Auto-select policy when opening in edit mode
  useEffect(() => {
    if (open && editPolicyKey && availablePolicies.length > 0) {
      const match = availablePolicies.find(
        (p) => `${p.name}@${p.version}` === editPolicyKey,
      );
      if (match) {
        setSelectedPolicy(match);
      }
    }
  }, [open, editPolicyKey, availablePolicies]);

  const drawerTitle = selectedPolicy
    ? selectedPolicy.displayName || selectedPolicy.name
    : title;

  return (
    <DrawerWrapper
      open={open}
      onClose={handleGuardedClose}
      minWidth={minWidth}
      maxWidth={maxWidth}
    >
      <DrawerHeader
        icon={<ShieldAlert size={24} />}
        title={drawerTitle}
        onClose={handleGuardedClose}
      />
      <DrawerContent>
        {!selectedPolicy && (
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
        ) : !selectedPolicy ? (
          <Stack spacing={2}>
            <SearchBar
              placeholder={searchPlaceholder}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              size="small"
              fullWidth
            />
            <Stack spacing={1.25}>
              {filteredPolicies.map((policy) => {
                const added = isDisabled(policy.name, policy.version);
                return (
                  <Form.CardButton
                    key={`${policy.name}@${policy.version}`}
                    selected={added}
                    disabled={added}
                    onClick={() => !added && setSelectedPolicy(policy)}
                    sx={{ width: "100%", justifyContent: "flex-start" }}
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
                          {policy.requiredSystemConfig.length > 0 && (
                            <Tooltip
                              title={`Set on the gateway: ${policy.requiredSystemConfig.join(", ")}`}
                            >
                              <Chip
                                label="Needs gateway configuration"
                                size="small"
                                color="warning"
                                variant="outlined"
                              />
                            </Tooltip>
                          )}
                        </Stack>
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
                    illustration={<ShieldAlert size={64} />}
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
            existingSettings={existingSettings}
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
