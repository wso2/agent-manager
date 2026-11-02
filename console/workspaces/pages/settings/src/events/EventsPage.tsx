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
import { Alert, Box, Chip, Divider, Stack, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { Lock } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  Route,
  Routes,
  useNavigate,
  useParams,
  useResolvedPath,
  useSearchParams,
} from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import {
  useEventsEnabled,
  useListEventDestinations,
  useListEventSubscriptions,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type EventScope,
  type EventTargetParams,
} from "@agent-management-platform/types";
import { DeliveryLogTab } from "./DeliveryLogTab";
import { DestinationsTab } from "./DestinationsTab";
import { SubscriptionEditor } from "./SubscriptionEditor";
import { SubscriptionsTab } from "./SubscriptionsTab";
import { useEventsAccess, type EventsAccess } from "./useEventsAccess";

type TabKey = "subscriptions" | "destinations" | "deliveries";

function TabLabel({ label, count }: { label: string; count?: string }) {
  return (
    <Stack direction="row" spacing={1} alignItems="center">
      <span>{label}</span>
      {count !== undefined && (
        <Chip
          size="small"
          label={count}
          sx={{ height: 20, fontSize: 11, bgcolor: "action.selected" }}
        />
      )}
    </Stack>
  );
}

function descriptionOf(scope: EventScope, target: EventTargetParams) {
  if (scope === "org") {
    return (
      "Send Agent Manager events to Slack, PagerDuty and webhooks. Subscriptions created here apply " +
      "to every project and agent in the organization."
    );
  }
  const name = scope === "agent" ? target.agentName : target.projName;
  return (
    <span>
      Event subscriptions for {scope} <strong>{name}</strong>.
    </span>
  );
}

interface OverviewProps {
  scope: EventScope;
  target: EventTargetParams;
  access: EventsAccess;
}

function EventsOverview({ scope, target, access }: OverviewProps) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const isOrg = scope === "org";
  const tabs: TabKey[] = isOrg
    ? ["subscriptions", "destinations", "deliveries"]
    : ["subscriptions", "deliveries"];
  const requested = searchParams.get("tab") as TabKey | null;
  const tab: TabKey = requested && tabs.includes(requested) ? requested : "subscriptions";
  const destinationId = searchParams.get("destinationId") ?? undefined;
  const subscriptionId = searchParams.get("subscriptionId") ?? undefined;

  const subscriptions = useListEventSubscriptions(target);
  const destinations = useListEventDestinations(target.orgName, isOrg);

  const setParams = (next: Record<string, string | undefined>) => {
    const params = new URLSearchParams(searchParams);
    for (const [k, v] of Object.entries(next)) {
      if (v) params.set(k, v);
      else params.delete(k);
    }
    setSearchParams(params, { replace: true });
  };

  const own = subscriptions.data?.subscriptions.length;
  const inherited = subscriptions.data?.inherited.length ?? 0;
  const subscriptionCount =
    own === undefined
      ? undefined
      : !isOrg && inherited > 0
        ? `${own} + ${inherited} inherited`
        : String(own);
  const subscriptionName = useMemo(() => {
    const all = [
      ...(subscriptions.data?.subscriptions ?? []),
      ...(subscriptions.data?.inherited ?? []),
    ];
    return all.find((s) => s.id === subscriptionId)?.name;
  }, [subscriptions.data, subscriptionId]);

  const orgEventsHref = access.canReadOrg
    ? generatePath(absoluteRouteMap.children.org.children.events.path, { orgId: target.orgName })
    : undefined;
  const projectEventsHref =
    scope === "agent"
      ? generatePath(absoluteRouteMap.children.org.children.projects.children.events.path, {
          orgId: target.orgName,
          projectId: target.projName,
        })
      : undefined;

  return (
    <PageLayout title="Events" description={descriptionOf(scope, target)} disableIcon>
      <Stack spacing={2.5}>
        <Box>
          <Tabs value={tab} onChange={(_, v: TabKey) => setParams({ tab: v })}>
            <Tab
              value="subscriptions"
              label={<TabLabel label="Subscriptions" count={subscriptionCount} />}
            />
            {isOrg && (
              <Tab
                value="destinations"
                label={
                  <TabLabel
                    label="Destinations"
                    count={
                      destinations.data ? String(destinations.data.destinations.length) : undefined
                    }
                  />
                }
              />
            )}
            <Tab value="deliveries" label={<TabLabel label="Delivery Log" />} />
          </Tabs>
          <Divider />
        </Box>

        {tab === "subscriptions" && (
          <SubscriptionsTab
            target={target}
            data={subscriptions.data}
            isLoading={subscriptions.isLoading}
            error={subscriptions.error}
            canManage={access.canManageSubscriptions}
            levelNoun={scope === "org" ? "organization" : scope}
            projectEventsHref={projectEventsHref}
            orgEventsHref={orgEventsHref}
            onNew={() => navigate("subscriptions/new")}
            onEdit={(s) => navigate(`subscriptions/${s.id}/edit`)}
            onViewDeliveries={(id) =>
              setParams({ tab: "deliveries", subscriptionId: id, destinationId: undefined })
            }
          />
        )}
        {tab === "destinations" && isOrg && (
          <DestinationsTab
            orgName={target.orgName}
            canManage={access.canManageDestinations}
            onViewDeliveries={(id) =>
              setParams({ tab: "deliveries", destinationId: id, subscriptionId: undefined })
            }
          />
        )}
        {tab === "deliveries" && (
          <DeliveryLogTab
            target={target}
            destinationOptions={
              isOrg
                ? (destinations.data?.destinations ?? []).map((d) => ({ id: d.id, name: d.name }))
                : undefined
            }
            destinationId={destinationId}
            subscriptionId={subscriptionId}
            subscriptionName={subscriptionName}
            onDestinationChange={(id) => setParams({ destinationId: id })}
            onClearSubscription={() => setParams({ subscriptionId: undefined })}
          />
        )}
      </Stack>
    </PageLayout>
  );
}

/**
 * A level's Events: its subscriptions (with the ones it inherits), the
 * organization's destinations at org level, and the delivery log. The
 * subscription editor lives at "subscriptions/new" and
 * "subscriptions/:subscriptionId/edit" below it; mount it on a "/*" route.
 */
export function EventsPage({ scope }: { scope: EventScope }) {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const listPath = useResolvedPath(".").pathname;
  const enabled = useEventsEnabled();
  const access = useEventsAccess(scope);
  const target = useMemo<EventTargetParams>(
    () => ({
      scope,
      orgName: orgId,
      projName: scope === "org" ? undefined : projectId,
      agentName: scope === "agent" ? agentId : undefined,
    }),
    [scope, orgId, projectId, agentId],
  );

  if (!enabled) {
    return (
      <PageLayout title="Events" disableIcon>
        <Alert severity="info">
          Events are not enabled on this deployment. An administrator can turn them on by running
          Agent Manager with an event bus.
        </Alert>
      </PageLayout>
    );
  }
  if (scope === "org" && access.resolved && !access.canReadOrg) {
    return (
      <Stack alignItems="center" justifyContent="center" spacing={2} sx={{ py: 10, px: 4 }}>
        <Box sx={{ color: "text.secondary", display: "flex" }}>
          <Lock size={48} />
        </Box>
        <Typography variant="h6">You don&apos;t have access to organization events</Typography>
        <Typography variant="body2" color="text.secondary" textAlign="center" maxWidth={440}>
          Destinations and organization subscriptions are available to organization administrators.
          Projects and agents have Events pages of their own.
        </Typography>
      </Stack>
    );
  }

  const levelName =
    scope === "agent" ? (agentId ?? "") : scope === "project" ? (projectId ?? "") : (orgId ?? "");
  return (
    <Routes>
      <Route index element={<EventsOverview scope={scope} target={target} access={access} />} />
      <Route
        path="subscriptions/new"
        element={
          <SubscriptionEditor
            target={target}
            access={access}
            listPath={listPath}
            levelName={levelName}
          />
        }
      />
      <Route
        path="subscriptions/:subscriptionId/edit"
        element={
          <SubscriptionEditor
            target={target}
            access={access}
            listPath={listPath}
            levelName={levelName}
          />
        }
      />
    </Routes>
  );
}

export const OrgEvents: React.FC = () => <EventsPage scope="org" />;
export const ProjectEvents: React.FC = () => <EventsPage scope="project" />;
export const AgentEvents: React.FC = () => <EventsPage scope="agent" />;

export default EventsPage;
