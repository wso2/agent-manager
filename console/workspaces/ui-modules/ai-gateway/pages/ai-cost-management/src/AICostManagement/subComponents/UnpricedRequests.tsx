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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  Alert,
  Card,
  CardContent,
  Grid,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { CircleDollarSign } from "@wso2/oxygen-ui-icons-react";
import { useCosts } from "@agent-management-platform/api-client";
import { PageLayout, StateNotice } from "@agent-management-platform/views";
import { keptMonths, monthLabel } from "../costFormat";
import { CostFilterBar, scopeFilter, useCostSelection } from "./CostFilters";

interface UnpricedRequestsProps {
  orgName?: string;
  /** A project's Cost Management: only that project's requests. */
  projectName?: string;
  /** An agent's Cost Management (with its project): only that agent's requests. */
  agentName?: string;
  backHref: string;
}

function Tile({
  label,
  value,
  caption,
}: {
  label: string;
  value: number;
  caption: string;
}) {
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <Typography variant="h5" sx={{ mt: 0.5 }}>
          {value}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {caption}
        </Typography>
      </CardContent>
    </Card>
  );
}

/**
 * Requests charged $0 over the kept months, by model: what had no price or no token
 * usage. Opens with the Dashboard's selection (its URL parameters).
 */
export function UnpricedRequests({
  orgName,
  projectName,
  agentName,
  backHref,
}: UnpricedRequestsProps) {
  // The Dashboard's selection, changeable here too (it stays in the URL).
  const selection = useCostSelection(orgName, projectName, agentName);
  const filter = [
    ...(scopeFilter(projectName, agentName) ?? []),
    ...(selection.pickFilter ? [selection.pickFilter] : []),
  ];
  const { data, isLoading, error } = useCosts(
    { orgName },
    { groupBy: "model", filter: filter.length ? filter : undefined },
  );
  const entries = data?.costByGroup ?? [];
  const months = keptMonths(entries);
  const period = `${monthLabel(months[0] ?? "")}–${monthLabel(months[months.length - 1] ?? "")}`;
  const rows = entries
    .map((e) => ({
      model: e.id ?? "Unknown model",
      missing: e.unpricedRequests.missingPrice,
      unavailable: e.unpricedRequests.usageUnavailable,
    }))
    .filter((r) => r.missing + r.unavailable > 0)
    .sort(
      (a, b) =>
        b.missing + b.unavailable - (a.missing + a.unavailable) ||
        a.model.localeCompare(b.model),
    );
  const missing = rows.reduce((n, r) => n + r.missing, 0);
  const unavailable = rows.reduce((n, r) => n + r.unavailable, 0);

  return (
    <PageLayout
      title="Requests charged $0"
      description={`Requests with no cost, counted as $0. Last 3 months (${period}).`}
      backHref={backHref}
      backLabel="Back to dashboard"
      disableIcon
    >
      <Stack spacing={3}>
        <CostFilterBar selection={selection} />
        {error ? (
          <Alert severity="error">
            Couldn&apos;t load the requests charged $0.
          </Alert>
        ) : (
          <>
            <Grid container spacing={2}>
              {isLoading ? (
                [0, 1, 2].map((i) => (
                  <Grid key={i} size={{ xs: 12, sm: 4 }}>
                    <Skeleton variant="rounded" height={104} />
                  </Grid>
                ))
              ) : (
                <>
                  <Grid size={{ xs: 12, sm: 4 }}>
                    <Tile
                      label="Missing price"
                      value={missing}
                      caption="No price for the model"
                    />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 4 }}>
                    <Tile
                      label="Usage unavailable"
                      value={unavailable}
                      caption="No token usage in the response"
                    />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 4 }}>
                    <Tile
                      label="All requests charged $0"
                      value={missing + unavailable}
                      caption="Last 3 months"
                    />
                  </Grid>
                </>
              )}
            </Grid>
            {isLoading ? (
              <Skeleton variant="rounded" height={160} />
            ) : rows.length === 0 ? (
              <StateNotice
                compact
                icon={CircleDollarSign}
                title="No requests charged $0"
                description="All requests in the last 3 months were priced."
              />
            ) : (
              <Stack spacing={1.5}>
                <Stack spacing={0.25}>
                  <Typography variant="subtitle1">
                    Pricing and usage exceptions
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Models with requests that couldn&apos;t be priced, most
                    affected first.
                  </Typography>
                </Stack>
                <ListingTable>
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell>Model</ListingTable.Cell>
                      <ListingTable.Cell>Missing price</ListingTable.Cell>
                      <ListingTable.Cell>Usage unavailable</ListingTable.Cell>
                      <ListingTable.Cell>Requests charged $0</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {rows.map((r) => (
                      <ListingTable.Row key={r.model}>
                        <ListingTable.Cell>
                          <Typography variant="body2" fontWeight={600}>
                            {r.model}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>{r.missing}</ListingTable.Cell>
                        <ListingTable.Cell>{r.unavailable}</ListingTable.Cell>
                        <ListingTable.Cell>
                          {r.missing + r.unavailable}
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </Stack>
            )}
          </>
        )}
      </Stack>
    </PageLayout>
  );
}
