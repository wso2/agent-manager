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

import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Grid,
  MenuItem,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { CircleCheck, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import { PageLayout, TextInput, useReturnTo } from "@agent-management-platform/views";
import {
  useCreateAgent,
  useListProjects,
  useOnboardShadowFinding,
  useSetShadowFindingHidden,
  useShadowFinding,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  INPUT_LIMITS,
  type ShadowFindingDetail,
} from "@agent-management-platform/types";
import {
  AGENT_NAME_MAX_LENGTH,
  AGENT_NAME_PATTERN,
  CodeHint,
  kindLabel,
  providerLabel,
  suggestAgentName,
} from "../shadowUi";

/** Instrumentation of a Python agent (Agent Manager docs: amp-instrumentation). */
export const INSTRUMENTATION_SHELL = [
  "pip install amp-instrumentation",
  'export AMP_OTEL_ENDPOINT="<OTel endpoint from the Setup Agent panel>"',
  'export AMP_AGENT_API_KEY="<agent API key from the Setup Agent panel>"',
  "amp-instrument python main.py",
].join("\n");

/** The LLM provider values the workload reads (Agent Manager docs: LLM configuration). */
export const LLM_GATEWAY_SHELL = [
  'export LLM_PROVIDER_URL="<Endpoint URL from the Connect to LLM Provider panel>"',
  'export LLM_PROVIDER_KEY="<API key from the same panel; it is shown once>"',
].join("\n");

/** The description a registered agent starts with: where it was found. */
export function discoveredDescription(f: ShadowFindingDetail): string {
  return `Discovered by Shadow Agent Discovery: ${providerLabel(f.provider)} ${kindLabel(
    f.kind,
    f.provider,
  ).toLowerCase()} ${f.externalId}`.slice(0, INPUT_LIMITS.DESCRIPTION);
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <Stack direction="row" spacing={2}>
      <Box
        sx={{
          width: 28,
          height: 28,
          borderRadius: "50%",
          bgcolor: "action.selected",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
        }}
      >
        <Typography variant="body2" fontWeight={600}>
          {n}
        </Typography>
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle2" gutterBottom>
          {title}
        </Typography>
        {children}
      </Box>
    </Stack>
  );
}

/**
 * Onboards an unregistered agent as an external agent: the page says what it
 * does and what follows, then registers the agent where it runs today (the
 * same request as the console's Register an agent page) and links the
 * unregistered agent to it.
 */
export function OnboardFinding({
  orgName,
  listPath,
}: {
  orgName: string | undefined;
  listPath: string;
}) {
  const { findingId } = useParams<{ findingId: string }>();
  const navigate = useNavigate();
  const findingPath = generatePath(
    absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.finding.path,
    { orgId: orgName ?? "", findingId: findingId ?? "" },
  );
  const returnTo = useReturnTo(findingPath || listPath);
  const { data: f, isLoading } = useShadowFinding(orgName, findingId);
  const projects = useListProjects({ orgName });
  const createAgent = useCreateAgent();
  const onboard = useOnboardShadowFinding(orgName, findingId);
  const setHidden = useSetShadowFindingHidden(orgName, findingId);
  const { addConfirmation } = useConfirmationDialog();
  const [project, setProject] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [agentName, setAgentName] = useState("");
  const [description, setDescription] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [done, setDone] = useState<{ project: string; agent: string } | null>(null);

  useEffect(() => {
    if (!f || displayName) return;
    setDisplayName(f.displayName.slice(0, INPUT_LIMITS.NAME));
    setAgentName(suggestAgentName(f.displayName));
    setDescription(discoveredDescription(f));
  }, [f, displayName]);
  useEffect(() => {
    if (!project && projects.data?.projects?.length) setProject(projects.data.projects[0].name);
  }, [project, projects.data]);

  const nameError = useMemo(() => {
    if (!agentName) return "Name is required";
    if (agentName.length > AGENT_NAME_MAX_LENGTH)
      return `At most ${AGENT_NAME_MAX_LENGTH} characters`;
    if (!AGENT_NAME_PATTERN.test(agentName)) {
      return 'Lowercase letters, digits and "-", starting with a letter and ending with a letter or digit';
    }
    return undefined;
  }, [agentName]);

  const back = { backHref: returnTo, backLabel: "Back to the agent" };
  if (isLoading || !f) {
    return (
      <PageLayout title="Onboard as external agent" disableIcon {...back}>
        <Skeleton variant="rounded" height={360} />
      </PageLayout>
    );
  }

  const agentPath = (p: string, a: string) =>
    generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, {
      orgId: orgName ?? "",
      projectId: p,
      agentId: a,
    });
  const linked =
    done ??
    (f.onboarded && f.linkedProject && f.linkedAgent
      ? { project: f.linkedProject, agent: f.linkedAgent }
      : null);

  const register = () => {
    setFormError(null);
    if (!project) return setFormError("Choose a project.");
    if (nameError) return setFormError(nameError);
    if (displayName.trim().length < 3) {
      return setFormError("Display name must be at least 3 characters.");
    }
    createAgent.mutate(
      {
        params: { orgName, projName: project },
        body: {
          name: agentName,
          displayName: displayName.trim(),
          description: description.trim(),
          provisioning: { type: "external" },
          agentType: { type: "external-agent-api" },
        },
      },
      {
        onSuccess: () =>
          onboard.mutate(
            { projectName: project, agentName },
            {
              onSuccess: () => {
                setDone({ project, agent: agentName });
                // The agent is registered now: ask whether it stays in the
                // Unregistered Agents list. Later scans keep the choice, and
                // the agent's page can change it.
                addConfirmation({
                  title: `Remove ${f.displayName} from Unregistered Agents?`,
                  description:
                    `${f.displayName} is onboarded as ${project} / ${agentName}. Remove it from the ` +
                    "Unregistered Agents list, or keep it there marked as Onboarded. Later scans keep " +
                    "your choice, and you can change it on the agent's page.",
                  confirmButtonText: "Remove from list",
                  cancelButtonText: "Keep in list",
                  onConfirm: () => setHidden.mutate(true),
                });
              },
              onError: () =>
                setFormError(
                  "The agent was registered, but it could not be linked. Try Register again.",
                ),
            },
          ),
        onError: () =>
          setFormError("The agent could not be registered. The name may be taken in this project."),
      },
    );
  };

  const nextSteps = (registered: { project: string; agent: string } | null) => (
    <Stack spacing={2.5}>
      <Step n={1} title="Set up the agent">
        <Typography variant="body2" color="text.secondary">
          In the registered agent&apos;s Setup Agent panel, generate the agent key (it is shown
          once). Where the workload runs, set these variables and start it with amp-instrument. No
          code changes are needed.
        </Typography>
        <CodeHint label="instrumentation commands" code={INSTRUMENTATION_SHELL} />
        {registered && (
          <Button
            component={RouterLink}
            to={`${agentPath(registered.project, registered.agent)}?setup=true`}
            size="small"
            variant="outlined"
            endIcon={<ExternalLink size={14} />}
          >
            Open the setup panel
          </Button>
        )}
      </Step>
      <Step n={2} title="Route model calls through Agent Manager">
        <Typography variant="body2" color="text.secondary">
          Under Configure, add an LLM configuration with an organization LLM provider. The Connect
          to LLM Provider panel then shows the Endpoint URL and an API key for the workload.
        </Typography>
        <CodeHint label="LLM provider variables" code={LLM_GATEWAY_SHELL} />
      </Step>
    </Stack>
  );

  if (linked) {
    return (
      <PageLayout
        title={`Onboard ${f.displayName}`}
        description="External agent"
        disableIcon
        {...back}
      >
        <Stack spacing={3}>
          <Alert severity="success" icon={<CircleCheck size={20} />}>
            {f.displayName} is onboarded as the external agent {linked.project} / {linked.agent}.
          </Alert>
          <Card variant="outlined">
            <CardContent>
              <Typography variant="subtitle1" gutterBottom>
                Next steps
              </Typography>
              {nextSteps(linked)}
            </CardContent>
          </Card>
          <Stack direction="row" spacing={1.5}>
            <Button
              component={RouterLink}
              to={agentPath(linked.project, linked.agent)}
              variant="contained"
              endIcon={<ExternalLink size={16} />}
            >
              Open registered agent
            </Button>
            <Button variant="outlined" onClick={() => navigate(findingPath)}>
              Back to the agent
            </Button>
          </Stack>
        </Stack>
      </PageLayout>
    );
  }

  const busy = createAgent.isPending || onboard.isPending;
  return (
    <PageLayout
      title={`Onboard ${f.displayName}`}
      description={`As an external agent · ${kindLabel(f.kind, f.provider)} on ${providerLabel(
        f.provider,
      )}`}
      disableIcon
      {...back}
    >
      <Stack spacing={3}>
        <Card variant="outlined">
          <CardContent>
            <Typography variant="subtitle1" gutterBottom>
              What onboarding does
            </Typography>
            <Stack component="ul" spacing={0.75} sx={{ m: 0, pl: 2.5 }}>
              <Typography component="li" variant="body2">
                Registers the agent in Agent Manager as an externally-hosted agent, in the project
                you choose.
              </Typography>
              <Typography component="li" variant="body2">
                {f.kind === "code_agent"
                  ? `Its code stays in the repository. Nothing changes on ${providerLabel(f.provider)}.`
                  : `The agent keeps running where it is, on ${providerLabel(f.provider)}. ` +
                    "Nothing changes on the platform."}
              </Typography>
              <Typography component="li" variant="body2">
                Agent Manager gives it an agent identity, observability and governed model access,
                and this page links to it as Onboarded.
              </Typography>
            </Stack>
          </CardContent>
        </Card>
        <Card variant="outlined">
          <CardContent>
            <Typography variant="subtitle1" gutterBottom>
              After you register
            </Typography>
            {nextSteps(null)}
          </CardContent>
        </Card>
        <Card variant="outlined">
          <CardContent>
            <Typography variant="subtitle1" gutterBottom>
              Register the agent
            </Typography>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, md: 6 }}>
                <TextInput
                  select
                  label="Project"
                  value={project}
                  onChange={(e) => setProject(e.target.value)}
                >
                  {(projects.data?.projects ?? []).map((p) => (
                    <MenuItem key={p.name} value={p.name}>
                      {p.displayName || p.name}
                    </MenuItem>
                  ))}
                </TextInput>
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <TextInput
                  label="Display name"
                  placeholder="e.g. Support assistant"
                  value={displayName}
                  maxLength={INPUT_LIMITS.NAME}
                  hideCharacterCount
                  onChange={(e) => setDisplayName(e.target.value)}
                />
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <TextInput
                  label="Name"
                  placeholder="e.g. support-assistant"
                  value={agentName}
                  maxLength={AGENT_NAME_MAX_LENGTH}
                  onChange={(e) => setAgentName(e.target.value.toLowerCase())}
                  error={!!nameError}
                  helperText={nameError ?? "Used in URLs; it cannot be changed later."}
                />
              </Grid>
              <Grid size={{ xs: 12 }}>
                <TextInput
                  label="Description (optional)"
                  value={description}
                  multiline
                  minRows={2}
                  maxLength={INPUT_LIMITS.DESCRIPTION}
                  hideCharacterCount
                  onChange={(e) => setDescription(e.target.value)}
                />
              </Grid>
            </Grid>
            {formError && (
              <Alert severity="error" sx={{ mt: 2 }}>
                {formError}
              </Alert>
            )}
            <Stack direction="row" spacing={1.5} sx={{ mt: 2.5 }}>
              <Button variant="contained" disabled={busy} onClick={register}>
                Register
              </Button>
              <Button variant="outlined" onClick={() => navigate(returnTo)}>
                Cancel
              </Button>
            </Stack>
          </CardContent>
        </Card>
      </Stack>
    </PageLayout>
  );
}

export default OnboardFinding;
