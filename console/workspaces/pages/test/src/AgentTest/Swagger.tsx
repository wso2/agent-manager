import { useGetAgentEndpoints } from "@agent-management-platform/api-client";
import { Alert, Box, Skeleton } from "@wso2/oxygen-ui";
import { useParams } from "react-router-dom";
import { useMemo } from "react";
import SwaggerUI from "swagger-ui-react";
import "swagger-ui-react/swagger-ui.css";

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
      agentName: agentId ?? "",
      orgName: orgId ?? "",
      projName: projectId ?? "",
    },
    {
      environment: envId ?? "",
    }
  );

  const endpoint = useMemo(() => Object.keys(data ?? {})?.[0] ?? "", [data]);
  const requestInterceptor = useMemo(
    () => (req: any) => {
      const targetUrl = data?.[endpoint]?.url;
      if (!targetUrl) return req;
      try {
        const incoming = new URL(req.url, window.location.origin);
        const target = new URL(targetUrl);

        const targetPath = target.pathname.replace(/\/+$/, "");
        const incomingPath = incoming.pathname.replace(/^\/+/, "");
        const mergedPath = [targetPath, incomingPath].filter(Boolean).join("/");

        target.pathname = mergedPath.startsWith("/") ? mergedPath : `/${mergedPath}`;
        target.search = incoming.search;
        target.hash = incoming.hash;

        req.url = target.toString();
      } catch {
        req.url = targetUrl;
      }
      return req;
    },
    [data, endpoint]
  );


  if (isLoading || !data?.[endpoint]?.schema?.content) {
    return <Skeleton variant="rounded" height={500} />;
  }

  if (error) {
    return <Alert severity="error">{error.message}</Alert>;
  }
  return (
    <Box sx={{ "& .swagger-ui .wrapper": { padding: 0 } }}>
      <SwaggerUI
        spec={data?.[endpoint].schema.content}
        layout="BaseLayout"
        plugins={[disableAuthorizeAndInfoPluginCustomSecuritySchema]}
        docExpansion="list"
        requestInterceptor={requestInterceptor}
      />
    </Box>
  );
}
