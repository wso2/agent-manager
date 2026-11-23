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

// The cost section the LLM provider page shows; it opens the Dashboard picked to
// that provider.
import React, { useState, type ReactNode } from "react";
import { Box, Button, Stack, Typography } from "@wso2/oxygen-ui";
import { ChevronRight, Plus } from "@wso2/oxygen-ui-icons-react";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import { generatePath, Link } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { EntityCostChart, EntityCostTiles } from "./EntityCostPanel";
import {
  QuickBudgetDialog,
  type QuickBudgetDialogProps,
} from "./QuickBudgetDialog";

const orgNav = absoluteRouteMap.children.org.children;

type AddBudgetProps = Omit<QuickBudgetDialogProps, "open" | "onClose">;

/** "Add budget" and its quick form, for users who may create budgets. */
function AddBudget(props: AddBudgetProps): ReactNode {
  const canWrite = useFeatureManagement().hasAccess("budget_create");
  const [open, setOpen] = useState(false);
  if (!canWrite) return null;
  return (
    <>
      <Button
        size="small"
        variant="outlined"
        startIcon={<Plus size={14} />}
        onClick={() => setOpen(true)}
        sx={{ borderRadius: 999, textTransform: "none", whiteSpace: "nowrap" }}
      >
        Add budget
      </Button>
      {open && (
        <QuickBudgetDialog {...props} open onClose={() => setOpen(false)} />
      )}
    </>
  );
}

/** Shared section header for the agent and LLM provider cost summaries. */
function CostSectionHeader({ add }: { add?: ReactNode }) {
  return (
    <Box
      display="flex"
      justifyContent="space-between"
      alignItems="center"
      mb={1.5}
    >
      <Typography
        variant="subtitle2"
        color="text.secondary"
        sx={{ fontWeight: 600 }}
      >
        Cost
      </Typography>
      {add}
    </Box>
  );
}

/** "View cost details", below a Cost section's figures. */
function ViewCostDetails({ href }: { href: string }) {
  return (
    <Box sx={{ mt: 1 }}>
      <Button
        component={Link}
        to={href}
        size="small"
        variant="text"
        endIcon={<ChevronRight size={14} />}
        sx={{ minWidth: 0, px: 0, fontSize: "0.75rem" }}
      >
        View cost details
      </Button>
    </Box>
  );
}

/** Budget pages at org level, or under a project. */
function budgetRoutes(orgId: string) {
  // Budgets are created and managed at organization level.
  const page = orgNav.aiCostManagement;
  return {
    createHref: generatePath(page.children.budgets.children.create.path, {
      orgId,
    }),
    viewHref: (budgetName: string) =>
      generatePath(page.children.budgets.children.view.path, {
        orgId,
        budgetName,
      }),
  };
}

/** The Dashboard, grouped by `groupBy` and picked to `pick`. */
function dashboardHref(base: string, groupBy?: string, pick?: string): string {
  if (!groupBy || !pick) return base;
  const q = new URLSearchParams({ groupBy, pick });
  return `${base}?${q.toString()}`;
}

/**
 * An LLM provider's cost, a section of its Overview tab, headed as the tab's other
 * sections (Invoke URL & API Key).
 */
export const LLMProviderCostSection: React.FC<{
  orgId: string;
  /** The provider's handle, as budgets and cost name it. */
  providerId: string;
  /** Shown to the user. */
  providerName?: string;
}> = ({ orgId, providerId, providerName }) => {
  const base = generatePath(orgNav.aiCostManagement.path, { orgId });
  return (
    <Box sx={{ width: "100%" }}>
      <CostSectionHeader
        add={
          <AddBudget
            orgName={orgId}
            start={{ provider: providerId }}
            kind="LLM provider"
            label={providerName ?? providerId}
            {...budgetRoutes(orgId)}
          />
        }
      />
      {/* The totals stacked on the left and a short chart beside them, the same height. */}
      <Stack
        direction={{ xs: "column", md: "row" }}
        spacing={1.5}
        alignItems="stretch"
      >
        {/* Wider tiles, a narrower chart with slim bars. */}
        <Box sx={{ width: { xs: "100%", md: "40%" }, flexShrink: 0 }}>
          <EntityCostTiles
            orgName={orgId}
            filter={`llmProvider:${providerId}`}
            stacked
          />
        </Box>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <EntityCostChart
            orgName={orgId}
            filter={`llmProvider:${providerId}`}
            groupBy="model"
            height={130}
            maxBarSize={56}
            fill
          />
        </Box>
      </Stack>
      <ViewCostDetails href={dashboardHref(base, "llmProvider", providerId)} />
    </Box>
  );
};
