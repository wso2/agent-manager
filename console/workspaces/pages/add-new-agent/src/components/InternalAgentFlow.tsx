/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useMemo, useState } from "react";
import { Alert, Form } from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap, OrgProjPathParams } from "@agent-management-platform/types";
import { useCreateAgent, useListAgents } from "@agent-management-platform/api-client";
import { createAgentSchema, type CreateAgentFormValues } from "../form/schema";
import { InternalAgentForm } from "../forms/InternalAgentForm";
import { CreateButtons } from "./CreateButtons";
import { buildAgentCreationPayload } from "../utils/buildAgentPayload";

export const InternalAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  const [formData, setFormData] = useState<CreateAgentFormValues>({
    deploymentType: "new" as const,
    enableAutoInstrumentation: true,
    name: "",
    displayName: "",
    description: "",
    repositoryUrl: "",
    branch: "main",
    appPath: "/",
    runCommand: "python main.py",
    language: "python",
    languageVersion: "3.11",
    dockerfilePath: "/Dockerfile",
    interfaceType: "DEFAULT" as const,
    port: "" as unknown as number,
    basePath: "/",
    openApiPath: "",
    env: [],
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateAgentFormValues>(createAgentSchema);

  const { mutate: createAgent, isPending, error } = useCreateAgent();
  const { data: agents } = useListAgents({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
  });

  const params = useMemo<OrgProjPathParams>(
    () => ({
      orgName: orgId ?? "default",
      projName: projectId ?? "default",
    }),
    [orgId, projectId]
  );

  const handleCancel = useCallback(() => {
    navigate(
      generatePath(absoluteRouteMap.children.org.children.projects.path, {
        orgId: orgId ?? "",
        projectId: projectId ?? "default",
      })
    );
  }, [navigate, orgId, projectId]);

  const handleDeploy = useCallback(() => {
    if (!validateForm(formData)) {
      return;
    }

    const payload = buildAgentCreationPayload(formData, params);
    createAgent(payload, {
      onSuccess: () => {
        navigate(
          generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.path,
            {
              orgId: params.orgName ?? "",
              projectId: params.projName ?? "",
              agentId: payload.body.name,
            }
          ) + "?setup=true"
        );
      },
      onError: (e: unknown) => {
        // eslint-disable-next-line no-console
        console.error("Failed to create agent:", e);
      },
    });
  }, [validateForm, formData, createAgent, navigate, params]);

  const isValid = Object.keys(errors).length === 0 &&
    formData.displayName.trim().length > 0 &&
    formData.repositoryUrl.trim().length > 0;

  const hasAgents = Boolean(agents?.agents?.length && agents?.agents?.length > 0);

  const backHref = useMemo(() => {
    if (!hasAgents) {
      return undefined;
    }
    return generatePath(absoluteRouteMap.children.org.children.projects.children.newAgent.path, {
      orgId: orgId ?? "",
      projectId: projectId ?? "default",
    });
  }, [hasAgents, orgId, projectId]);

  return (
    <PageLayout
      title="Create a Platform-Hosted Agent"
      description="Specify the source repository, select the agent type, and deploy it on the platform."
      disableIcon
      backHref={backHref}
      backLabel="Back to Agent Hosting Options"
    >
      <Form.Stack spacing={3}>
        <InternalAgentForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          setFieldError={setFieldError}
          validateField={validateField}
        />

        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : "Failed to create agent"}
          </Alert>
        )}

        <CreateButtons
          isValid={isValid}
          isPending={isPending}
          onCancel={handleCancel}
          onSubmit={handleDeploy}
          mode="deploy"
        />
      </Form.Stack>
    </PageLayout>
  );
};
