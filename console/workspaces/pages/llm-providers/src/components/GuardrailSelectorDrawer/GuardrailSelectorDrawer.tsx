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
  useGuardrailsCatalog,
  useGuardrailPolicyDefinition,
  type GuardrailDefinition,
} from "@agent-management-platform/api-client";
import PolicyParameterEditor from "../../PolicyParameterEditor/PolicyParameterEditor";
import { parsePolicyYaml } from "../../PolicyParameterEditor/yamlParser";
import type {
  PolicyDefinition,
  ParameterValues,
} from "../../PolicyParameterEditor/types";

const GuardrailDetailView: React.FC<{
  guardrail: GuardrailDefinition;
  existingSettings?: Record<string, unknown>;
  onBack: () => void;
  onSubmit: (guardrail: GuardrailDefinition, settings: ParameterValues) => void;
}> = ({ guardrail, existingSettings, onBack, onSubmit }) => {
  const {
    data: yamlText,
    isLoading,
    error,
  } = useGuardrailPolicyDefinition(guardrail.name, guardrail.version);

  const [policyDefinition, setPolicyDefinition] =
    useState<PolicyDefinition | null>(null);
  const [parseError, setParseError] = useState<string | null>(null);

  useEffect(() => {
    if (!yamlText) return;
    try {
      setPolicyDefinition(parsePolicyYaml(yamlText));
      setParseError(null);
    } catch {
      setParseError("Failed to parse policy definition.");
    }
  }, [yamlText]);

  if (isLoading) {
    return (
      <Stack spacing={2} sx={{ mt: 1 }}>
        <Typography variant="body2" color="text.secondary">
          Loading definition...
        </Typography>
        <Skeleton variant="text" width="60%" height={28} />
        <Skeleton variant="text" width="90%" height={16} />
        <Skeleton variant="rounded" width="100%" height={48} />
        <Skeleton variant="rounded" width="100%" height={48} />
      </Stack>
    );
  }

  if (error || parseError) {
    return (
      <Stack spacing={2} sx={{ py: 2 }}>
        <Alert severity="error">
          {parseError ||
            (error as Error)?.message ||
            "Failed to load guardrail definition."}
        </Alert>
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

  if (!policyDefinition) {
    return (
      <Stack spacing={2}>
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<ShieldAlert size={64} />}
            title="No definition available"
            description="This guardrail does not have a configuration schema."
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
      <PolicyParameterEditor
        policyDefinition={policyDefinition}
        policyDisplayName={guardrail.displayName || guardrail.name}
        existingValues={existingSettings}
        isEditMode={Boolean(existingSettings)}
        onCancel={onBack}
        onSubmit={(values) => onSubmit(guardrail, values)}
      />
    </Stack>
  );
};

export type GuardrailSelectorDrawerProps = {
  open: boolean;
  onClose: () => void;
  onSubmit: (guardrail: GuardrailDefinition, settings: ParameterValues) => void;
  /** Guardrail names that are already added - disable in list (e.g. create flow) */
  disabledGuardrailNames?: string[];
  /** Guardrail keys (name@version) that are already added - more precise than names */
  disabledGuardrailKeys?: string[];
  /** Existing settings when editing (e.g. for pre-filling form) */
  existingSettings?: Record<string, unknown>;
  title?: string;
  subtitle?: string;
  minWidth?: number;
  maxWidth?: number;
};

export function GuardrailSelectorDrawer({
  open,
  onClose,
  onSubmit,
  disabledGuardrailNames = [],
  disabledGuardrailKeys = [],
  existingSettings,
  title = "Guardrails",
  subtitle = "Choose a guardrail to configure advanced options.",
  minWidth = 600,
  maxWidth = 800,
}: GuardrailSelectorDrawerProps) {
  const {
    data: catalogData,
    isLoading: isLoadingCatalog,
    error: catalogError,
  } = useGuardrailsCatalog();

  const [selectedGuardrail, setSelectedGuardrail] =
    useState<GuardrailDefinition | null>(null);
  const [searchQuery, setSearchQuery] = useState("");

  const availableGuardrails = useMemo(
    () => catalogData?.data ?? [],
    [catalogData],
  );

  const isDisabled = useCallback(
    (name: string, version: string) =>
      disabledGuardrailKeys.length > 0
        ? disabledGuardrailKeys.includes(`${name}@${version}`)
        : disabledGuardrailNames.includes(name),
    [disabledGuardrailKeys, disabledGuardrailNames],
  );

  const filteredGuardrails = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return availableGuardrails;
    return availableGuardrails.filter(
      (g) =>
        (g.displayName || g.name).toLowerCase().includes(q) ||
        g.name.toLowerCase().includes(q) ||
        g.description?.toLowerCase().includes(q),
    );
  }, [availableGuardrails, searchQuery]);

  const handleClose = useCallback(() => {
    setSelectedGuardrail(null);
    setSearchQuery("");
    onClose();
  }, [onClose]);

  const handleSubmit = useCallback(
    (guardrail: GuardrailDefinition, settings: ParameterValues) => {
      onSubmit(guardrail, settings);
      setSelectedGuardrail(null);
    },
    [onSubmit],
  );

  useEffect(() => {
    if (!open) {
      setSelectedGuardrail(null);
      setSearchQuery("");
    }
  }, [open]);

  const drawerTitle = selectedGuardrail
    ? selectedGuardrail.displayName || selectedGuardrail.name
    : title;

  return (
    <DrawerWrapper
      open={open}
      onClose={handleClose}
      minWidth={minWidth}
      maxWidth={maxWidth}
    >
      <DrawerHeader
        icon={<ShieldAlert size={24} />}
        title={drawerTitle}
        onClose={handleClose}
      />
      <DrawerContent>
        {!selectedGuardrail && (
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {subtitle}
          </Typography>
        )}

        {isLoadingCatalog ? (
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Loading guardrails...
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
            Failed to load guardrails. {(catalogError as Error)?.message}
          </Alert>
        ) : !selectedGuardrail ? (
          <Stack spacing={2}>
            <SearchBar
              placeholder="Search guardrails..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              size="small"
              fullWidth
            />
            <Stack spacing={1.25}>
              {filteredGuardrails.map((guardrail) => {
                const added = isDisabled(guardrail.name, guardrail.version);
                return (
                  <Form.CardButton
                    key={guardrail.name}
                    selected={added}
                    disabled={added}
                    onClick={() => !added && setSelectedGuardrail(guardrail)}
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
                            {added ? (
                              <Check size={16} />
                            ) : (
                              <Circle size={16} />
                            )}
                          </Avatar>
                          <Typography variant="body2" fontWeight={500}>
                            {guardrail.displayName || guardrail.name}
                          </Typography>
                          <Chip
                            label={`v${guardrail.version}`}
                            size="small"
                            variant="outlined"
                          />
                        </Stack>
                        {guardrail.description && (
                          <Tooltip title={guardrail.description}>
                            <Typography variant="caption" color="text.secondary">
                              {guardrail.description.substring(0, 200)}
                              {guardrail.description.length > 200 ? "..." : ""}
                            </Typography>
                          </Tooltip>
                        )}
                      </Stack>
                    </CardContent>
                  </Form.CardButton>
                );
              })}
              {filteredGuardrails.length === 0 && searchQuery && (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Search size={64} />}
                    title="No guardrails match your search"
                    description="Try a different keyword or clear the search filter."
                  />
                </ListingTable.Container>
              )}
              {filteredGuardrails.length === 0 && !searchQuery && (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<ShieldAlert size={64} />}
                    title="No guardrails available"
                    description="No guardrail policies are available in the catalog."
                  />
                </ListingTable.Container>
              )}
            </Stack>
          </Stack>
        ) : (
          <GuardrailDetailView
            guardrail={selectedGuardrail}
            existingSettings={existingSettings}
            onBack={() => setSelectedGuardrail(null)}
            onSubmit={handleSubmit}
          />
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}
