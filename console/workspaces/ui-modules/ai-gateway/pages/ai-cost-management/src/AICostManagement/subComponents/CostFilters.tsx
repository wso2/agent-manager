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

import { useMemo } from "react";
import {
  Autocomplete,
  Box,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useSearchParams } from "react-router-dom";
import { useCosts } from "@agent-management-platform/api-client";
import { TextInput } from "@agent-management-platform/views";
import type { CostGroupBy } from "@agent-management-platform/types";
import { dimensionsFor, groupKey, groupLabel } from "../costFormat";
import { inSentence } from "./budgetLabels";
import { useEntityNames } from "../useEntityNames";
import { TOOLBAR_FIELD_SX } from "../controlSizes";

/** The cost API filter for a project's or an agent's own Cost Management. */
export function scopeFilter(
  projectName?: string,
  agentName?: string,
): string[] | undefined {
  if (projectName && agentName) return [`agent:${projectName}/${agentName}`];
  return projectName ? [`project:${projectName}`] : undefined;
}

/**
 * The Dashboard's selection (Group by and the picked values), kept in the URL so links
 * can open it pre-filtered and other pages (Requests charged $0) can share it. Reads the
 * cost grouped by the selected dimension: the chart's data, and the picker's options.
 */
export function useCostSelection(
  orgName?: string,
  projectName?: string,
  agentName?: string,
) {
  const projectScoped = !!projectName;
  const dimensions = dimensionsFor(projectScoped, !!agentName);
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = searchParams.get("groupBy") as CostGroupBy | null;
  const groupBy: CostGroupBy = dimensions.some((d) => d.key === requested)
    ? (requested as CostGroupBy)
    : agentName
      ? "model"
      : projectScoped
        ? "agent"
        : "project";
  const picked = searchParams.getAll("pick");
  const dimension = dimensions.find((d) => d.key === groupBy) ?? dimensions[0];
  // Display names for what cost data names by handle (UX: the names used elsewhere).
  const names = useEntityNames(orgName ?? "");
  const { data, isLoading, error } = useCosts(
    { orgName },
    { groupBy, filter: scopeFilter(projectName, agentName) },
  );
  const setFilters = (nextGroupBy: CostGroupBy, nextPicked: string[]) => {
    const params = new URLSearchParams(searchParams);
    params.set("groupBy", nextGroupBy);
    params.delete("pick");
    nextPicked.forEach((p) => params.append("pick", p));
    setSearchParams(params, { replace: true });
  };
  const allGroups = useMemo(() => data?.costByGroup ?? [], [data]);
  const options = useMemo(
    () =>
      allGroups
        .map((g) => groupKey(g, groupBy))
        .sort((a, b) =>
          groupLabel(a, groupBy, projectScoped, names).localeCompare(
            groupLabel(b, groupBy, projectScoped, names),
          ),
        ),
    [allGroups, groupBy, projectScoped, names],
  );
  const scope = picked.length
    ? picked.map((p) => groupLabel(p, groupBy, projectScoped, names)).join(", ")
    : groupBy === "organization"
      ? "The organization"
      : `All ${inSentence(dimension.plural)}`;
  // The cost API's filter for the picks; agents are `<project>/<agent>` as picked.
  const pickFilter =
    groupBy !== "organization" && picked.filter(Boolean).length
      ? `${groupBy}:${picked.filter(Boolean).join(",")}`
      : undefined;
  return {
    projectName,
    agentName,
    projectScoped,
    dimensions,
    dimension,
    groupBy,
    picked,
    setFilters,
    names,
    allGroups,
    options,
    scope,
    pickFilter,
    isLoading,
    error,
    searchParams,
  };
}

export type CostSelection = ReturnType<typeof useCostSelection>;

/** Group by, the picker for that dimension, and the "Showing" line under them. */
export function CostFilterBar({ selection }: { selection: CostSelection }) {
  const {
    dimensions,
    dimension,
    groupBy,
    picked,
    setFilters,
    options,
    names,
    projectScoped,
    scope,
    projectName,
    agentName,
  } = selection;
  return (
    <>
      <Stack
        direction={{ xs: "column", md: "row" }}
        spacing={1.5}
        alignItems={{ md: "center" }}
      >
        <Box sx={{ minWidth: 220 }}>
          <TextInput
            select
            size="small"
            aria-label="Group by"
            sx={TOOLBAR_FIELD_SX}
            value={groupBy}
            onChange={(e) => setFilters(e.target.value as CostGroupBy, [])}
            slotProps={{
              select: {
                renderValue: () => `Group by: ${dimension.label}`,
              },
            }}
          >
            {dimensions.map((d) => (
              <MenuItem key={d.key} value={d.key}>
                {d.label}
              </MenuItem>
            ))}
          </TextInput>
        </Box>
        {groupBy !== "organization" && (
          <Autocomplete
            multiple
            size="small"
            sx={{ minWidth: 280, flex: 1, maxWidth: 640, ...TOOLBAR_FIELD_SX }}
            options={options}
            value={picked}
            onChange={(_, value) => setFilters(groupBy, value)}
            getOptionLabel={(o) => groupLabel(o, groupBy, projectScoped, names)}
            renderInput={(params) => (
              <TextField
                {...params}
                placeholder={
                  picked.length
                    ? `Add ${inSentence(dimension.label)}`
                    : `All ${inSentence(dimension.plural)}`
                }
                slotProps={{
                  htmlInput: {
                    ...params.inputProps,
                    "aria-label": `${dimension.plural} to show`,
                  },
                }}
              />
            )}
          />
        )}
      </Stack>
      <Typography variant="body2" color="text.secondary">
        Showing <strong>{scope}</strong>
        {agentName
          ? ` for agent ${agentName}`
          : projectName
            ? ` in ${projectName}`
            : ""}
      </Typography>
    </>
  );
}
