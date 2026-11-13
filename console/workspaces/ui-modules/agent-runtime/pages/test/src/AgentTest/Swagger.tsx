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

import {
  useGetAgent,
  useGetAgentConfigurations,
  useGetAgentEndpoints,
  useGetAgentKindVersion,
  useGetBuild,
  useListAgentDeployments,
  useTestAgentAPIKey,
} from "@agent-management-platform/api-client";
import { StateNotice } from "@agent-management-platform/views";
import { getErrorMessage } from "@agent-management-platform/shared-component";
import { Alert, Box, Button, Skeleton, Typography } from "@wso2/oxygen-ui";
import { FileQuestion, FileX, KeyRound, RefreshCw, ServerCrash } from "@wso2/oxygen-ui-icons-react";

import { useParams } from "react-router-dom";
import { useMemo, useState, lazy, Suspense } from "react";

const SwaggerUI = lazy(() => import("swagger-ui-react"));

// A freshly issued test key takes a couple of seconds to reach the gateway's
// policy engine, so one delayed retry with the same key rides out that window.
const TEST_KEY_PROPAGATION_RETRY_DELAY_MS = 2000;

const disableAuthorizeAndInfoPluginCustomSecuritySchema = {
  statePlugins: {
    spec: {
      wrapSelectors: {
        servers: () => (): any[] => [],
        schemes: () => (): any[] => [],
      },
    },
  },
  wrapComponents: {
    info: () => (): any => null,
  },
};

export function Swagger() {
  const { orgId, projectId, agentId, envId } = useParams();
  const { data, isLoading, error } = useGetAgentEndpoints(
    {
      agentName: agentId,
      orgName: orgId,
      projName: projectId,
    },
    {
      environment: envId ?? "",
    }
  );

  // GetAgent returns only the lowest environment's config, so read per-env here.
  const { data: envConfig } = useGetAgentConfigurations(
    {
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    },
    {
      environment: envId ?? "",
    }
  );
  // Absent means no per-env config row yet, so assume a key is required.
  const securityEnabled = envConfig?.enableApiKeySecurity ?? true;
  const oauthOnly = !!(
    envConfig?.enableOAuthSecurity && !envConfig?.enableApiKeySecurity
  );
  const {
    data: testKey,
    isLoading: isLoadingTestKey,
    isError: isTestKeyError,
    error: testKeyError,
    refetch: refetchTestKey,
  } = useTestAgentAPIKey(
    { orgName: orgId, projName: projectId, agentName: agentId, envId },
    { enabled: securityEnabled && !oauthOnly },
  );
  const testApiKey = testKey?.apiKey;
  const [keyAlert, setKeyAlert] = useState<"unauthorized" | "refreshed" | null>(
    null,
  );
  const [isRefreshingKey, setIsRefreshingKey] = useState(false);

  const gatewayOffline = testKey?.gatewayConnected === false;

  const endpoint = useMemo(() => Object.keys(data ?? {})?.[0] ?? "", [data]);

  // A kind-sourced agent runs no build of its own, so nothing ever writes an
  // OpenAPI document onto its deployed endpoint. Resolve the spec from the kind
  // version instead — kind version -> its source build — which is how the agent
  // catalogue renders a kind's API spec.
  //
  // The version read is the one deployed in THIS environment, not the agent's
  // creation-time kindVersion: redeploying on a newer version changes what the
  // endpoint actually serves, and each environment moves independently, so the
  // creation version would render a spec that no longer matches the running API.
  // kindName is what identifies a kind agent; waiting on the endpoints response
  // to notice a missing schema would just delay the lookups behind it.
  const deployedSpec = data?.[endpoint]?.schema?.content;
  const {
    data: agent,
    isLoading: isAgentLoading,
    error: agentError,
  } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });
  const deployedKindVersion = deployments?.[envId ?? ""]?.kindVersion;
  const needsKindSpec = !!agent?.kindName && !!deployedKindVersion;
  const {
    data: kindVersion,
    isLoading: isKindVersionLoading,
    error: kindVersionError,
  } = useGetAgentKindVersion({
    orgName: needsKindSpec ? orgId ?? "" : "",
    kindName: needsKindSpec ? agent?.kindName ?? "" : "",
    versionTag: needsKindSpec ? deployedKindVersion ?? "" : "",
  });
  const {
    data: kindBuild,
    isLoading: isKindBuildLoading,
    error: kindBuildError,
  } = useGetBuild({
    orgName: needsKindSpec ? orgId ?? "" : "",
    projName: kindVersion?.sourceProjectName ?? "",
    agentName: kindVersion?.sourceAgentName ?? "",
    buildName: kindVersion?.buildName ?? "",
  });
  const isKindSpecLoading =
    !!agent?.kindName &&
    (isDeploymentsLoading || (needsKindSpec && (isKindVersionLoading || isKindBuildLoading)));
  // Reading the kind's spec goes through the kind's source project, which the
  // viewer may not have access to, so these can fail on their own.
  const specLookupError = agentError ?? (needsKindSpec ? kindVersionError ?? kindBuildError : null);
  const specContent =
    deployedSpec || kindBuild?.inputInterface?.schema?.content;

  // The gateway drops CORS headers on 401s, so a cross-origin 401 rejects
  // fetch with a TypeError before swagger-ui's responseInterceptor runs.
  // Inject a custom fetch (req.userFetch) instead: retry once to ride out key
  // propagation, then surface the refresh alert on a persistent 401/TypeError.
  const requestInterceptor = useMemo(
    () => (req: any) => {
      const targetUrl = data?.[endpoint]?.url;
      if (!targetUrl) {
        return req;
      }
      const incoming = new URL(req.url, window.location.origin);
      const target = new URL(targetUrl);

      const targetPath = target.pathname.replace(/\/+$/, "");
      const incomingPath = incoming.pathname.replace(/^\/+/, "");
      const mergedPath = [targetPath, incomingPath].filter(Boolean).join("/");

      target.pathname = mergedPath.startsWith("/")
        ? mergedPath
        : `/${mergedPath}`;
      target.search = incoming.search;
      target.hash = incoming.hash;
      req.url = target.toString();
      if (securityEnabled && testApiKey) {
        req.headers = req.headers ?? {};
        req.headers["X-API-Key"] = testApiKey;
      }

      if (securityEnabled && !gatewayOffline) {
        req.userFetch = async (url: string, options: RequestInit) => {
          const sleep = () =>
            new Promise((resolve) =>
              setTimeout(resolve, TEST_KEY_PROPAGATION_RETRY_DELAY_MS),
            );
          try {
            const res = await fetch(url, options);
            if (res.status !== 401) {
              return res;
            }
            await sleep();
            const retry = await fetch(url, options);
            if (retry.status === 401) {
              setKeyAlert("unauthorized");
            }
            return retry;
          } catch (err) {
            if (!(err instanceof TypeError)) {
              throw err;
            }
            await sleep();
            try {
              return await fetch(url, options);
            } catch (retryErr) {
              setKeyAlert("unauthorized");
              throw retryErr;
            }
          }
        };
      }
      return req;
    },
    [data, endpoint, securityEnabled, testApiKey, gatewayOffline]
  );

  const handleRefreshTestKey = async () => {
    setIsRefreshingKey(true);
    try {
      await refetchTestKey();
      setKeyAlert("refreshed");
    } finally {
      setIsRefreshingKey(false);
    }
  };

  if (
    isLoading ||
    isAgentLoading ||
    isKindSpecLoading ||
    (securityEnabled && isLoadingTestKey)
  ) {
    return <Skeleton variant="rounded" height={500} />;
  }

  const reloadAction = (
    <Button variant="outlined" startIcon={<RefreshCw size={16} />} onClick={() => window.location.reload()}>
      Reload page
    </Button>
  );

  if (error) {
    return (
      <StateNotice
        icon={ServerCrash}
        tone="error"
        title="Couldn't load this agent's endpoints"
        description={getErrorMessage(error)}
        actions={reloadAction}
      />
    );
  }

  if (securityEnabled && isTestKeyError) {
    return (
      <StateNotice
        icon={KeyRound}
        tone="error"
        title="Couldn't get a test API key"
        description={`Requests from this page need a test key, and fetching one failed${
          testKeyError instanceof Error ? `: ${testKeyError.message}` : ""
        }.`}
        actions={reloadAction}
      />
    );
  }

  // A failed lookup is not the same as an agent that has no schema, so only claim
  // the latter once every lookup the spec could have come from has succeeded. A
  // schema we already have still renders — an error resolving the kind's copy is
  // irrelevant then.
  if (!specContent && specLookupError) {
    return (
      <StateNotice
        icon={FileX}
        tone="error"
        title="Couldn't load the API schema"
        description={getErrorMessage(specLookupError)}
        actions={reloadAction}
      />
    );
  }

  if (!specContent) {
    return (
      <StateNotice
        icon={FileQuestion}
        tone="warning"
        title="No API schema to show"
        description="This agent hasn't published an OpenAPI schema for this endpoint, so there is nothing to try out here."
      />
    );
  }

  return (
    <Suspense fallback={<Skeleton variant="rounded" height={500} />}>
      {oauthOnly && (
        <Alert severity="info" sx={{ mb: 2 }}>
          <Typography variant="caption">
            OAuth is enabled — test this endpoint out-of-band with an{" "}
            <code>Authorization: Bearer &lt;token&gt;</code> header.
          </Typography>
        </Alert>
      )}
      {securityEnabled && gatewayOffline && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          The gateway is not connected to the control plane right now. The
          test API key has been stored but will only work once the gateway
          reconnects.
        </Alert>
      )}
      {keyAlert === "unauthorized" && (
        <Alert
          severity="error"
          action={
            <Button
              color="inherit"
              size="small"
              onClick={handleRefreshTestKey}
              disabled={isRefreshingKey}
            >
              {isRefreshingKey ? "Refreshing..." : "Refresh test key"}
            </Button>
          }
          sx={{ mb: 2 }}
        >
          The test API key is not authorized on the gateway yet. Execute the
          request again, or refresh the test key.
        </Alert>
      )}
      {keyAlert === "refreshed" && (
        <Alert
          severity="info"
          onClose={() => setKeyAlert(null)}
          sx={{ mb: 2 }}
        >
          Test key refreshed. Execute the request again.
        </Alert>
      )}
      {/* The page's own title already covers what the spec's info block would
          repeat, so this view hides it (and the single, uninformative default
          tag header) via the shared swagger-spec-viewer classes instead of
          rendering swagger-ui's out-of-the-box chrome. */}
      <Box className="swagger-spec-viewer hide-info-section hide-servers hide-models">
        <SwaggerUI
          spec={specContent}
          layout="BaseLayout"
          plugins={[disableAuthorizeAndInfoPluginCustomSecuritySchema]}
          docExpansion="list"
          requestInterceptor={requestInterceptor}
          supportedSubmitMethods={oauthOnly ? [] : undefined}
        />
      </Box>
    </Suspense>
  );
}
