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
  Plus,
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
import PolicyParameterEditor from "../PolicyParameterEditor/PolicyParameterEditor";
import { parsePolicyYaml } from "../PolicyParameterEditor/yamlParser";
import type {
  PolicyDefinition,
  ParameterValues,
} from "../PolicyParameterEditor/types";

export type GuardrailSelection = {
  name: string;
  version: string;
  displayName?: string;
  settings?: Record<string, unknown>;
};

interface GuardrailsSectionProps {
  guardrails: GuardrailSelection[];
  onAddGuardrail: (guardrail: GuardrailSelection) => void;
  onRemoveGuardrail: (name: string) => void;
}

// ---------------------------------------------------------------------------
// Inner component that fetches the YAML definition for a selected guardrail
// ---------------------------------------------------------------------------

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
        <Skeleton variant="text" width="60%" height={28} />
        <Skeleton variant="text" width="90%" height={16} />
        <Skeleton variant="text" width="75%" height={16} />
        <Skeleton variant="text" width="80%" height={16} />
        <Skeleton variant="rounded" width="100%" height={48} />
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
            "Failed to load definition."}
        </Alert>
        <Button
          variant="text"
          startIcon={<ArrowLeft size={16} />}
          onClick={onBack}
        >
          Back to list
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
          Back to list
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
          Back to list
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

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export const GuardrailsSection: React.FC<GuardrailsSectionProps> = ({
  guardrails,
  onAddGuardrail,
  onRemoveGuardrail,
}) => {
  const {
    data: catalogData,
    isLoading: isLoadingCatalog,
    error: catalogError,
  } = useGuardrailsCatalog();

  const availableGuardrails = useMemo(
    () => catalogData?.data ?? [],
    [catalogData],
  );

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [selectedGuardrail, setSelectedGuardrail] =
    useState<GuardrailDefinition | null>(null);
  const [searchQuery, setSearchQuery] = useState("");

  const isAlreadyAdded = useCallback(
    (name: string) => guardrails.some((g) => g.name === name),
    [guardrails],
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

  const handleOpenDrawer = useCallback(() => {
    setSelectedGuardrail(null);
    setSearchQuery("");
    setDrawerOpen(true);
  }, []);

  const handleCloseDrawer = useCallback(() => {
    setDrawerOpen(false);
    setSelectedGuardrail(null);
    setSearchQuery("");
  }, []);

  const handleGuardrailClick = useCallback((guardrail: GuardrailDefinition) => {
    setSelectedGuardrail(guardrail);
  }, []);

  const handlePolicySubmit = useCallback(
    (guardrail: GuardrailDefinition, values: ParameterValues) => {
      onAddGuardrail({
        name: guardrail.name,
        version: guardrail.version,
        displayName: guardrail.displayName,
        settings: values as Record<string, unknown>,
      });
      setSelectedGuardrail(null);
    },
    [onAddGuardrail],
  );

  const drawerTitle = selectedGuardrail
    ? selectedGuardrail.displayName || selectedGuardrail.name
    : "Add Guardrail";

  return (
    <>
      {/* Section card on the form */}
      <Form.Section>
        <Form.Header>Guardrails</Form.Header>
        <Stack spacing={3}>
          <Box
            sx={{
              display: "flex",
              alignItems: "flex-start",
              justifyContent: "space-between",
              gap: 2,
            }}
          >
            <Box>
              <Typography variant="body2" color="text.secondary">
                Add safety policies to enforce consistent protections.
              </Typography>
            </Box>
          </Box>

          {
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              {guardrails.map((g) => (
                <Chip
                  key={g.name}
                  label={`${g.displayName || g.name} (${g.version})`}
                  color="warning"
                  variant="outlined"
                  onDelete={() => onRemoveGuardrail(g.name)}
                />
              ))}
              <Button
                variant="contained"
                size="small"
                endIcon={<Plus size={16} />}
                onClick={handleOpenDrawer}
              >
                Add Guardrail
              </Button>
            </Stack>
          }
        </Stack>
      </Form.Section>

      {/* Drawer */}
      <DrawerWrapper
        open={drawerOpen}
        onClose={handleCloseDrawer}
        minWidth={800}
        maxWidth={800}
      >
        <DrawerHeader
          icon={<ShieldAlert size={24} />}
          title={drawerTitle}
          onClose={handleCloseDrawer}
        />
        <DrawerContent>
          {isLoadingCatalog ? (
            <Stack spacing={1.5} sx={{ mt: 1 }}>
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
            /* -------- List view -------- */
            <Stack spacing={2}>
              {/* Search */}
              <SearchBar
                placeholder="Search guardrails..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                size="small"
                fullWidth
              />

              {/* Results */}
              <Stack spacing={1.25}>
                {filteredGuardrails.map((guardrail) => {
                  const added = isAlreadyAdded(guardrail.name);
                  return (
                    <Form.CardButton
                      key={guardrail.name}
                      selected={added}
                      disabled={added}
                      onClick={() => handleGuardrailClick(guardrail)}
                      sx={{ width: "100%", justifyContent: "flex-start" }}
                    >
                      <CardContent>
                        <Stack spacing={1}>
                        <Stack
                          direction="row"
                          spacing={0.5}
                          alignItems="center"
                        >
                          <Box
                            display="flex"
                            alignItems="center"
                            color={added ? "success.main" : "text.secondary"}
                          >
                            <Avatar
                              sx={{
                                height: 32,
                                width: 32,
                                backgroundColor: added ? "primary.main" : "secondary.main",
                                color: added ? "white" : "text.secondary",
                              }}
                            >
                              {added ? (
                                <Check size={16} />
                              ) : (
                                <Circle size={16} />
                              )}
                            </Avatar>
                          </Box>
                          &nbsp;
                          <Typography variant="body2" fontWeight={500}>
                            {guardrail.displayName || guardrail.name}
                          </Typography>
                          &nbsp;
                          <Chip
                            label={`v${guardrail.version}`}
                            size="small"
                            variant="outlined"
                          />
                        </Stack>
                        {/* <Divider sx={{ my: 1 }} /> */}
                        <Tooltip title={guardrail.description}>
                          <Typography variant="caption" color="text.secondary">
                            {guardrail.description.substring(0, 200)}...
                          </Typography>
                        </Tooltip>
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
                      description={`Try a different keyword or clear the search filter.`}
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
            /* -------- Detail / parameter editor view -------- */
            <GuardrailDetailView
              guardrail={selectedGuardrail}
              existingSettings={
                guardrails.find((g) => g.name === selectedGuardrail.name)
                  ?.settings
              }
              onBack={() => setSelectedGuardrail(null)}
              onSubmit={handlePolicySubmit}
            />
          )}
        </DrawerContent>
      </DrawerWrapper>
    </>
  );
};

export default GuardrailsSection;
