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

import React, { useState } from "react";
import {
  generatePath,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  PageLayout,
  returnToState,
  useReturnTo,
} from "@agent-management-platform/views";
import { useEntityNames } from "./useEntityNames";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { CostOverview } from "./subComponents/CostOverview";
import { BudgetCreateAction, BudgetsTab } from "./subComponents/BudgetsTab";
import { BudgetDetail } from "./subComponents/BudgetDetail";
import { BudgetWizard } from "./subComponents/BudgetForm";
import { startQuery } from "./budgetForm";
import { UnpricedRequests } from "./subComponents/UnpricedRequests";
import { QuickBudgetDialog } from "../panels/QuickBudgetDialog";
import { PricingList } from "../Pricing/PricingList";

type View = "overview" | "budgets";

/** The organization's Cost Management, or a project's or an agent's when the URL has
 * one. Budgets are created and edited at organization level only. */
function useScope() {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId?: string;
    agentId?: string;
  }>();
  const org = absoluteRouteMap.children.org.children;
  const route =
    projectId && agentId
      ? org.projects.children.agents.children.costManagement
      : projectId
        ? org.projects.children.costManagement
        : org.costManagement;
  const params = { orgId, projectId, agentId };
  const budgets = route.children.budgets;
  const orgBudgets = org.costManagement.children.budgets;
  return {
    orgId,
    projectId,
    agentId,
    overview: generatePath(route.path, params),
    unpriced: generatePath(route.children.unpricedRequests.path, params),
    budgets: generatePath(budgets.path, params),
    // On a project's or agent's page, a new organization budget for it.
    // On a project's or agent's page, a new organization budget for it.
    start:
      projectId && agentId
        ? { agent: `${projectId}/${agentId}` }
        : projectId
          ? { project: projectId }
          : undefined,
    createBase: generatePath(orgBudgets.children.create.path, { orgId }),
    orgBudgetView: (budgetName: string) =>
      generatePath(orgBudgets.children.view.path, { orgId, budgetName }),
    budgetCreate:
      generatePath(orgBudgets.children.create.path, { orgId }) +
      startQuery(
        projectId && agentId
          ? { agent: `${projectId}/${agentId}` }
          : projectId
            ? { project: projectId }
            : {},
      ),
    budgetView: (budgetName: string) =>
      generatePath(budgets.children.view.path, { ...params, budgetName }),
    budgetEdit: (budgetName: string) =>
      generatePath(orgBudgets.children.view.children.edit.path, {
        orgId,
        budgetName,
      }),
  };
}

const TITLES: Record<View, string> = {
  overview: "Dashboard",
  budgets: "Budgets",
};

function describe(
  view: View,
  projectId?: string,
  projectName?: string,
  agentName?: string,
): React.ReactNode {
  const project = projectName ?? projectId;
  if (view === "overview") {
    return agentName ? (
      <span>
        Monthly LLM cost of agent <strong>{agentName}</strong>.
      </span>
    ) : projectId ? (
      <span>
        Monthly LLM cost of project <strong>{project}</strong>.
      </span>
    ) : (
      "Monthly LLM cost of the organization."
    );
  }
  return agentName ? (
    <span>
      Budgets for agent <strong>{agentName}</strong>.
    </span>
  ) : projectId ? (
    <span>
      Budgets for project <strong>{project}</strong>.
    </span>
  ) : (
    "Spend limits for LLM traffic."
  );
}

function CostManagementPage({ view }: { view: View }) {
  const navigate = useNavigate();
  const {
    orgId,
    projectId,
    agentId,
    budgetView,
    budgetCreate,
    unpriced,
    start,
    createBase,
    orgBudgetView,
  } = useScope();
  // On a project's or agent's page "Add budget" opens the quick form with it filled in;
  // its "Advanced create" opens the full form.
  const [quickOpen, setQuickOpen] = useState(false);
  // The header has the create action only while the list has budgets; an empty list
  // shows it in its empty state.
  const [budgetsEmpty, setBudgetsEmpty] = useState(false);
  const location = useLocation();
  const onCreate = () =>
    start
      ? setQuickOpen(true)
      : navigate(budgetCreate, { state: returnToState(location) });
  const createAction = (
    <BudgetCreateAction
      orgName={orgId}
      projectName={projectId}
      onCreate={onCreate}
    />
  );
  const names = useEntityNames(orgId ?? "");
  const projectName = names.project?.[projectId ?? ""];
  const agentName = agentId
    ? (names.agent?.[`${projectId}/${agentId}`] ?? agentId)
    : undefined;
  return (
    <PageLayout
      title={TITLES[view]}
      description={describe(view, projectId, projectName, agentName)}
      disableIcon
      actions={view === "budgets" && !budgetsEmpty ? createAction : undefined}
    >
      {view === "overview" ? (
        <CostOverview
          orgName={orgId}
          projectName={projectId}
          agentName={agentId}
          unpricedHref={unpriced}
        />
      ) : (
        <BudgetsTab
          orgName={orgId}
          projectName={projectId}
          agentName={agentId}
          onOpen={(name) => navigate(budgetView(name))}
          createAction={createAction}
          onEmptyChange={setBudgetsEmpty}
        />
      )}
      {quickOpen && start && orgId && (
        <QuickBudgetDialog
          open
          onClose={() => setQuickOpen(false)}
          orgName={orgId}
          start={start}
          kind={agentId ? "Agent" : "Project"}
          label={agentName ?? projectName ?? projectId ?? ""}
          createHref={createBase}
          viewHref={orgBudgetView}
        />
      )}
    </PageLayout>
  );
}

/** A page shown only with its access; otherwise the page the user may open instead. */
function Guard({
  allowed,
  fallback,
  children,
}: {
  allowed: boolean;
  fallback: string;
  children: React.ReactElement;
}) {
  return allowed ? children : <Navigate to={fallback} replace />;
}

function UnpricedRequestsPage() {
  const { orgId, projectId, agentId, overview } = useScope();
  const [searchParams] = useSearchParams();
  const query = searchParams.toString();
  return (
    <UnpricedRequests
      orgName={orgId}
      projectName={projectId}
      agentName={agentId}
      backHref={query ? `${overview}?${query}` : overview}
    />
  );
}

/** The model prices the gateways use: organization level only. */
function PricingPage() {
  const { orgId } = useScope();
  return (
    <PageLayout
      title="Pricing Catalog"
      description="Model prices the gateways use."
      disableIcon
    >
      <PricingList orgName={orgId} />
    </PageLayout>
  );
}

function BudgetDetailPage() {
  const { budgetName = "" } = useParams<{ budgetName: string }>();
  const { orgId, projectId, budgets, budgetEdit, unpriced } = useScope();
  return (
    <BudgetDetail
      orgName={orgId}
      budgetName={budgetName}
      backHref={budgets}
      editHref={budgetEdit(budgetName)}
      readOnly={!!projectId}
      unpricedHref={unpriced}
    />
  );
}

/** The create wizard, or with `editing` the edit wizard of the budget in the URL. */
function BudgetFormRoute({ editing }: { editing?: boolean }) {
  const navigate = useNavigate();
  const { budgetName } = useParams<{ budgetName: string }>();
  const { orgId, budgets, budgetView } = useScope();
  // Back to the page that opened it; the budget, or the list, on a direct visit.
  const backHref = useReturnTo(
    editing && budgetName ? budgetView(budgetName) : budgets,
  );
  return (
    <BudgetWizard
      orgName={orgId}
      budgetName={editing ? budgetName : undefined}
      backHref={backHref}
      backLabel={editing ? "Back to the budget" : "Back to budgets"}
      onSaved={(name) => navigate(budgetView(name))}
    />
  );
}

/** Cost Management for an organization or one of its projects; the left
 * navigation's Cost Management section opens each view. */
export const AICostManagement: React.FC = () => {
  const { overview, budgets, projectId } = useScope();
  const { hasAccess } = useFeatureManagement();
  // The module is active only with costs.read (plugin.ts), which reads every page;
  // creating and editing check their own access.
  return (
    <Routes>
      <Route index element={<CostManagementPage view="overview" />} />
      <Route path="unpriced-requests" element={<UnpricedRequestsPage />} />
      <Route path="budgets" element={<CostManagementPage view="budgets" />} />
      <Route
        path="pricing"
        element={
          <Guard allowed={!projectId} fallback={overview}>
            <PricingPage />
          </Guard>
        }
      />
      <Route
        path="budgets/create"
        element={
          <Guard
            allowed={hasAccess("cost_write") && !projectId}
            fallback={budgets}
          >
            <BudgetFormRoute />
          </Guard>
        }
      />
      <Route path="budgets/view/:budgetName" element={<BudgetDetailPage />} />
      <Route
        path="budgets/view/:budgetName/edit"
        element={<BudgetEditRoute />}
      />
      <Route path="*" element={<Navigate to={overview} replace />} />
    </Routes>
  );
};

/** Edit, for users who may update budgets; others go to the budget's page. */
function BudgetEditRoute() {
  const { budgetName = "" } = useParams<{ budgetName: string }>();
  const { budgetView, projectId } = useScope();
  // Budgets are managed at organization level only.
  const allowed =
    useFeatureManagement().hasAccess("cost_write") && !projectId;
  return (
    <Guard allowed={allowed} fallback={budgetView(budgetName)}>
      <BudgetFormRoute editing />
    </Guard>
  );
}

export default AICostManagement;
