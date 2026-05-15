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
  useGetAgentEndpoints,
  useTestAgentAPIKey,
} from "@agent-management-platform/api-client";
import { getErrorMessage } from "@agent-management-platform/shared-component";
import { Alert, Box, Skeleton } from "@wso2/oxygen-ui";
import { useParams } from "react-router-dom";
import { useMemo, lazy, Suspense } from "react";

const SwaggerUI = lazy(() => import("swagger-ui-react"));

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

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const securityEnabled = !!agent?.configurations?.enableApiKeySecurity;
  const {
    data: testKey,
    isLoading: isLoadingTestKey,
    isError: isTestKeyError,
    error: testKeyError,
  } = useTestAgentAPIKey(
    { orgName: orgId, projName: projectId, agentName: agentId, envId },
    { enabled: securityEnabled },
  );
  const testApiKey = testKey?.apiKey;

  const endpoint = useMemo(() => Object.keys(data ?? {})?.[0] ?? "", [data]);
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
      return req;
    },
    [data, endpoint, securityEnabled, testApiKey]
  );

  if (isLoading || (securityEnabled && isLoadingTestKey)) {
    return <Skeleton variant="rounded" height={500} />;
  }

  if (error) {
    return <Alert severity="error">{getErrorMessage(error)}</Alert>;
  }

  if (securityEnabled && isTestKeyError) {
    return (
      <Alert severity="error">
        Failed to fetch test API key{testKeyError instanceof Error ? `: ${testKeyError.message}` : ""}.
      </Alert>
    );
  }

  if (!data?.[endpoint]?.schema?.content) {
    return (
      <Alert severity="warning">
        No API schema available for this endpoint.
      </Alert>
    );
  }

  return (
    <Suspense fallback={<Skeleton variant="rounded" height={500} />}>
      <Box sx={{ "& .swagger-ui .wrapper": { padding: 0 } }}>
        <SwaggerUI
          spec={data?.[endpoint].schema.content}
          layout="BaseLayout"
          plugins={[disableAuthorizeAndInfoPluginCustomSecuritySchema]}
          docExpansion="list"
          requestInterceptor={requestInterceptor}
        />
      </Box>
    </Suspense>
  );
}
