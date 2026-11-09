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

import { useMemo, useState } from "react";
import {
  Alert,
  Button,
  Checkbox,
  CircularProgress,
  FormControlLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, Navigate, useNavigate, useParams } from "react-router-dom";
import {
  useCreateProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
} from "@agent-management-platform/api-client";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { environmentLabel } from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import { MAX_PROXY_ID_LENGTH, toProjectProxyId } from "./subComponents/projectBinding";

const PROXY_ID_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function errorMessage(error: unknown): string {
  if (error && typeof error === "object" && "message" in error) {
    return String((error as { message: unknown }).message);
  }
  return "Something went wrong. Please try again.";
}

function isHTTPURL(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:";
  } catch {
    return false;
  }
}

/**
 * Registers a project MCP server that fronts a public URL, and deploys it to the
 * environments picked. Each environment gets its own binding, secured with an API key.
 */
export function AddMCPProxyProject() {
  const { orgId = "", projectId = "" } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const { canWrite } = useResourceAccess("mcp_server");
  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const createProxy = useCreateProjectMCPProxy();
  const putBinding = usePutProjectMCPProxyEnvironment();

  const [name, setName] = useState("");
  const [proxyId, setProxyId] = useState("");
  const [proxyIdEdited, setProxyIdEdited] = useState(false);
  const [url, setUrl] = useState("");
  const [description, setDescription] = useState("");
  const [context, setContext] = useState("");
  const [selectedEnvs, setSelectedEnvs] = useState<string[]>([]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string>();

  const listPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.mcpProxies.path,
    { orgId, projectId },
  );

  const effectiveId = proxyIdEdited ? proxyId : toProjectProxyId(name);
  const idError =
    effectiveId && !PROXY_ID_RE.test(effectiveId)
      ? "Use lowercase letters, digits and single hyphens."
      : undefined;
  const urlError = url && !isHTTPURL(url.trim()) ? "Enter an http or https URL." : undefined;
  const contextError =
    context && !context.startsWith("/") ? "The context must start with /." : undefined;
  const canSubmit =
    !!name.trim() &&
    !!effectiveId &&
    !!url.trim() &&
    !idError &&
    !urlError &&
    !contextError &&
    !isSubmitting;

  const envOptions = useMemo(
    () =>
      environments.map((env) => ({
        name: env.name,
        label: environmentLabel(env) || env.name,
      })),
    [environments],
  );

  if (!canWrite) {
    return <Navigate to={listPath} replace />;
  }

  const toggleEnv = (envName: string, checked: boolean) =>
    setSelectedEnvs((current) =>
      checked ? [...current, envName] : current.filter((item) => item !== envName),
    );

  const submit = async () => {
    setIsSubmitting(true);
    setError(undefined);
    const params = { orgName: orgId, projName: projectId, proxyId: effectiveId };
    try {
      await createProxy.mutateAsync({
        params,
        body: {
          name: name.trim(),
          ...(description.trim() ? { description: description.trim() } : {}),
          ...(context.trim() ? { context: context.trim() } : {}),
          upstream: { url: url.trim() },
        },
      });
    } catch (err) {
      setError(errorMessage(err));
      setIsSubmitting(false);
      return;
    }
    // The server exists from here on; an environment that fails is reported on its page.
    for (const envName of selectedEnvs) {
      try {
        await putBinding.mutateAsync({ params: { ...params, envName }, body: {} });
      } catch {
        // The view page shows which environments the server is deployed to.
      }
    }
    setIsSubmitting(false);
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.mcpProxies.children.view
          .path,
        { orgId, projectId, proxyId: effectiveId },
      ),
    );
  };

  return (
    <PageLayout
      title="Register MCP Server"
      backHref={listPath}
      backLabel="Back to MCP servers"
      disableIcon
    >
      <Stack spacing={3} maxWidth={640}>
        <Typography variant="body2" color="text.secondary">
          Register an MCP server reachable at a public URL for this project. Agents in
          the project call it through the gateway of each environment it is deployed to.
        </Typography>
        {error && <Alert severity="error">{error}</Alert>}
        <TextField
          label="Name"
          size="small"
          required
          value={name}
          placeholder="e.g. Docs MCP Server"
          onChange={(event) => setName(event.target.value)}
        />
        <TextField
          label="Identifier"
          size="small"
          required
          value={effectiveId}
          error={!!idError}
          helperText={idError ?? "Unique in the organization. It cannot be changed later."}
          inputProps={{ maxLength: MAX_PROXY_ID_LENGTH }}
          onChange={(event) => {
            setProxyIdEdited(true);
            setProxyId(event.target.value);
          }}
        />
        <TextField
          label="MCP server URL"
          size="small"
          required
          value={url}
          placeholder="https://mcp.example.com/mcp"
          error={!!urlError}
          helperText={
            urlError ?? "Must be publicly reachable. Private and loopback addresses are refused."
          }
          onChange={(event) => setUrl(event.target.value)}
        />
        <TextField
          label="Description"
          size="small"
          multiline
          minRows={2}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
        />
        <TextField
          label="Context"
          size="small"
          value={context}
          placeholder={effectiveId ? `/${effectiveId}` : "/docs-mcp"}
          error={!!contextError}
          helperText={
            contextError ??
            `Served under the project, at /${projectId}${context || (effectiveId ? `/${effectiveId}` : "")}.`
          }
          onChange={(event) => setContext(event.target.value)}
        />
        <Stack spacing={0.5}>
          <Typography variant="subtitle2">Deploy to</Typography>
          <Typography variant="caption" color="text.secondary">
            Each environment is secured with an API key. You can add or remove
            environments later.
          </Typography>
          {envOptions.map((env) => (
            <FormControlLabel
              key={env.name}
              control={
                <Checkbox
                  checked={selectedEnvs.includes(env.name)}
                  onChange={(event) => toggleEnv(env.name, event.target.checked)}
                />
              }
              label={env.label}
            />
          ))}
        </Stack>
        <Stack direction="row" spacing={1}>
          <Button
            variant="contained"
            disabled={!canSubmit}
            onClick={() => void submit()}
            startIcon={isSubmitting ? <CircularProgress size={16} /> : undefined}
          >
            Register
          </Button>
          <Button variant="outlined" onClick={() => navigate(listPath)}>
            Cancel
          </Button>
        </Stack>
      </Stack>
    </PageLayout>
  );
}

export default AddMCPProxyProject;
