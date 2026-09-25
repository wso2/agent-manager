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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useQueries, type UseQueryOptions } from "@tanstack/react-query";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  FormControl,
  FormLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, ShieldX } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  listAgentIdentityRoles,
  useCreateMCPProxyScope,
  useUpdateAgentIdentityRole,
  useDialogAnalytics,
  useValidationErrorTracking,
} from "@agent-management-platform/api-client";
import {
  type AgentIdentityRoleListResponse,
  type Environment,
  type MCPProxyScopeResponse,
  type ThunderRole,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import { z } from "zod";

// Stable empty-array identity for environments whose role query hasn't
// resolved yet — a fresh `[]` literal per render would make Autocomplete
// treat `options` as changed on every parent re-render.
const EMPTY_ROLES: ThunderRole[] = [];

interface CreateScopeFormValues {
  name: string;
  description?: string;
}

// Mirrors the backend's action format constraint (mcp_proxy_scope_service.go):
// 1-100 chars, letters/digits/./_/- only.
const SCOPE_ACTION_PATTERN = /^[A-Za-z0-9._-]{1,100}$/;

const createScopeSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Name is required")
    .regex(
      SCOPE_ACTION_PATTERN,
      "Only letters, numbers, '.', '_' and '-' are allowed (max 100 characters)",
    ),
  description: z.string().trim().optional(),
});

const DEFAULT_FORM: CreateScopeFormValues = { name: "", description: "" };

const ROLES_PAGE_SIZE = 100;

// Thunder seeds this role natively in every environment's default OU (like
// "Administrator", which agent-manager-service already excludes server-side
// in ListRoles) — not a role a user should be granting scopes to here.
// Compared case-insensitively since Thunder's exact casing isn't guaranteed.
const NATIVE_DEFAULT_ROLE_NAME = "default";

interface CreateScopeDrawerProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  proxyId: string;
  /** Environments the current endpoint is deployed to with identity security enabled. */
  environments: Environment[];
  /** Tool identifiers discovered on the current endpoint, offered as scope bindings. */
  tools: string[];
  /**
   * Subset of `tools` the endpoint's Manage Tools ACL currently blocks. Listed
   * but not selectable: binding a scope to a tool the gateway already refuses
   * would enforce nothing, while hiding them outright would leave the picker
   * looking empty under a deny-all ACL and give no hint where the tool went.
   */
  blockedTools: ReadonlySet<string>;
}

export function CreateScopeDrawer({
  open,
  onClose,
  orgName,
  proxyId,
  environments,
  tools,
  blockedTools,
}: CreateScopeDrawerProps) {
  const [formData, setFormData] = useState<CreateScopeFormValues>(DEFAULT_FORM);
  const [selectedTools, setSelectedTools] = useState<string[]>([]);
  // Keyed by `env.name` (not position in `environments`) so a reorder or
  // refetch of the environments list can't silently misapply a selection to
  // the wrong environment. Roles are picked one environment at a time rather
  // than from one combined list, since a role only ever lives in a single
  // environment and the save step already updates them per-env.
  const [selectedRolesByEnv, setSelectedRolesByEnv] = useState<Record<string, ThunderRole[]>>({});
  const [isAssigningRoles, setIsAssigningRoles] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<CreateScopeFormValues>(createScopeSchema);

  const { getToken } = useAuthHooks();
  const createScope = useCreateMCPProxyScope();
  // Opening reports intent; closing without markCompleted reports an
  // abandonment. Pair with amp.connections.manage-mcp-scope (the backend's own
  // success event) for the conversion rate.
  const { markCompleted } = useDialogAnalytics("mcp-scope", open);
  const trackValidationError = useValidationErrorTracking("mcp-scope");
  const { reset: resetCreateScope } = createScope;
  const updateRole = useUpdateAgentIdentityRole();

  useEffect(() => {
    if (!open) return;
    setFormData(DEFAULT_FORM);
    setSelectedTools([]);
    setSelectedRolesByEnv({});
    setSubmitError(null);
    clearErrors();
    resetCreateScope();
  }, [open, clearErrors, resetCreateScope]);

  const roleQueries = useQueries({
    queries: environments.map(
      (env): UseQueryOptions<AgentIdentityRoleListResponse> => ({
        queryKey: [
          "agent-identity-roles",
          { orgName, envName: env.name },
          { offset: 0, limit: ROLES_PAGE_SIZE },
        ],
        queryFn: () =>
          listAgentIdentityRoles(
            { orgName, envName: env.name },
            { offset: 0, limit: ROLES_PAGE_SIZE },
            getToken,
          ),
        enabled: open && !!orgName && !!env.name,
      }),
    ),
  });

  // Assignable roles per environment, in the same order as `environments`.
  const roleOptionsByEnv: ThunderRole[][] = useMemo(
    () =>
      environments.map((_env, index) => {
        const roles = roleQueries[index]?.data?.roles ?? [];
        return roles.filter(
          (role) =>
            !role.isReadOnly && role.name.trim().toLowerCase() !== NATIVE_DEFAULT_ROLE_NAME,
        );
      }),
    [environments, roleQueries],
  );

  const handleRolesChangeForEnv = useCallback((envName: string, roles: ThunderRole[]) => {
    setSelectedRolesByEnv((prev) => ({ ...prev, [envName]: roles }));
  }, []);

  const handleFieldChange = useCallback(
    (field: keyof CreateScopeFormValues, value: string) => {
      const error = validateField(field, value);
      if (error) {
        // The field name only — never the value, which is the user's own input.
        trackValidationError(String(field));
      }
      setFieldError(field, error);
      setFormData((prev) => ({ ...prev, [field]: value }));
    },
    [validateField, setFieldError, trackValidationError],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const formValid = validateForm(formData);
      if (!formValid) return;

      setSubmitError(null);
      let created: MCPProxyScopeResponse;
      try {
        created = await createScope.mutateAsync({
          params: { orgName, proxyId },
          body: {
            action: formData.name.trim(),
            description: formData.description?.trim() || undefined,
            tools: selectedTools,
          },
        });
      } catch {
        setSubmitError("Failed to create scope. Please try again.");
        return;
      }

      markCompleted();

      // Scope creation succeeded — close now rather than waiting on role
      // assignment below. A role-assignment failure is a separate, partial
      // failure and must not be reported through (or mistaken for) the
      // scope-creation error above.
      onClose();

      const roleAssignments = environments.flatMap((env) =>
        (selectedRolesByEnv[env.name] ?? []).map((role) => ({
          role,
          envName: env.name,
        })),
      );

      if (roleAssignments.length === 0) return;

      setIsAssigningRoles(true);
      try {
        // Roles have no incremental "grant this one scope" endpoint — each
        // update PUTs the role's full desired scope set, merged from the
        // permissions already on hand from the roles fetch. Settled (not
        // Promise.all) so one role failing to update doesn't stop the rest
        // from being assigned; each mutation reports its own failure via its
        // built-in error notification.
        await Promise.allSettled(
          roleAssignments.map(({ role, envName }) => {
            const existingScopes = role.permissions?.flatMap((p) => p.permissions) ?? [];
            const nextScopes = existingScopes.includes(created.scope)
              ? existingScopes
              : [...existingScopes, created.scope];
            return updateRole.mutateAsync({
              params: { orgName, envName, roleId: role.id },
              body: { name: role.name, description: role.description, scopes: nextScopes },
            });
          }),
        );
      } finally {
        setIsAssigningRoles(false);
      }
    },
    [
      formData,
      validateForm,
      selectedTools,
      createScope,
      orgName,
      proxyId,
      environments,
      selectedRolesByEnv,
      updateRole,
      onClose,
    ],
  );

  const isSubmitting = createScope.isPending || isAssigningRoles;
  const isValid = !errors.name && formData.name.trim().length > 0;

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Plus size={24} />} title="Create Scope" onClose={onClose} />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {submitError && (
              <Alert severity="error">
                <Typography variant="body2">{submitError}</Typography>
              </Alert>
            )}

            <Typography variant="body2" color="text.secondary">
              A scope is a named permission callers must hold to invoke the
              tools it&apos;s attached to. Leave the tools empty to declare the
              scope now and bind it later — until then it is grantable to roles
              but required by no tool.
            </Typography>

            <FormControl fullWidth error={Boolean(errors.name)}>
              <FormLabel required>Name</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                fullWidth
                size="small"
                value={formData.name}
                onChange={(e) => handleFieldChange("name", e.target.value)}
                placeholder="e.g., read-only"
                error={Boolean(errors.name)}
                helperText={errors.name}
                disabled={isSubmitting}
              />
            </FormControl>

            <FormControl fullWidth error={Boolean(errors.description)}>
              <FormLabel>Description</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.DESCRIPTION } }}
                fullWidth
                size="small"
                multiline
                minRows={2}
                value={formData.description}
                onChange={(e) => handleFieldChange("description", e.target.value)}
                error={Boolean(errors.description)}
                helperText={errors.description}
                disabled={isSubmitting}
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel>Tools</FormLabel>
              <Autocomplete
                multiple
                size="small"
                disableCloseOnSelect
                options={tools}
                value={selectedTools}
                onChange={(_e, value) => setSelectedTools(value)}
                getOptionDisabled={(tool) => blockedTools.has(tool)}
                renderOption={(optionProps, tool) => {
                  const { key, ...liProps } = optionProps;
                  if (!blockedTools.has(tool)) {
                    return (
                      <li key={key} {...liProps}>
                        {tool}
                      </li>
                    );
                  }
                  // Spelled out in the row rather than behind the tool table's
                  // tooltip: MUI gives a disabled option `pointer-events: none`,
                  // so no hover affordance on one would ever fire. The name gets
                  // its own <span> because Stack's spacing selector matches only
                  // element siblings — a bare text node would sit flush against
                  // the marker.
                  return (
                    <li key={key} {...liProps}>
                      <Stack direction="row" alignItems="center" spacing={1}>
                        <span>{tool}</span>
                        <Stack
                          color="warning.main"
                          direction="row"
                          alignItems="center"
                          spacing={0.5}
                        >
                          <ShieldX size={14} />
                          <Typography component="span" variant="caption">
                            Blocked by Manage Tools
                          </Typography>
                        </Stack>
                      </Stack>
                    </li>
                  );
                }}
                renderTags={(value, getTagProps) =>
                  value.map((tool, index) => (
                    <Chip {...getTagProps({ index })} key={tool} label={tool} size="small" />
                  ))
                }
                renderInput={(params) => (
                  <TextField {...params} placeholder="Search tools..." />
                )}
                noOptionsText="No tools discovered on this endpoint"
                disabled={isSubmitting}
              />
            </FormControl>

            <Stack spacing={1.5}>
              <FormLabel>
                {environments.length > 1
                  ? "Assigned Roles for each environment"
                  : "Assigned Roles"}
              </FormLabel>
              {environments.length === 0 ? (
                <Typography variant="body2" color="text.secondary">
                  This endpoint isn&apos;t deployed to any environment yet.
                </Typography>
              ) : (
                environments.map((env, index) => (
                  <FormControl fullWidth key={env.id ?? env.name}>
                    <FormLabel sx={{ fontSize: "0.8125rem" }}>
                      {env.displayName ?? env.name}
                    </FormLabel>
                    <Autocomplete
                      multiple
                      size="small"
                      disableCloseOnSelect
                      loading={roleQueries[index]?.isLoading}
                      options={roleOptionsByEnv[index] ?? EMPTY_ROLES}
                      value={selectedRolesByEnv[env.name] ?? EMPTY_ROLES}
                      onChange={(_e, value) => handleRolesChangeForEnv(env.name, value)}
                      getOptionLabel={(role) => role.name}
                      isOptionEqualToValue={(option, value) => option.id === value.id}
                      renderTags={(value, getTagProps) =>
                        value.map((role, tagIndex) => (
                          <Chip
                            {...getTagProps({ index: tagIndex })}
                            key={role.id}
                            label={role.name}
                            size="small"
                          />
                        ))
                      }
                      renderInput={(params) => (
                        <TextField {...params} placeholder="Search roles..." />
                      )}
                      noOptionsText={
                        roleQueries[index]?.isError
                          ? "Failed to load roles"
                          : "No roles available"
                      }
                      disabled={isSubmitting}
                    />
                  </FormControl>
                ))
              )}
              <Typography variant="caption" color="text.secondary">
                Roles to grant this scope to, per environment this MCP Server is
                deployed to.
              </Typography>
            </Stack>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isSubmitting}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={!isValid || isSubmitting}
              >
                {isSubmitting ? "Creating..." : "Create Scope"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
