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
import { connectAgentSchema, type ConnectAgentFormValues } from "../form/schema";
import { ExternalAgentForm } from "../forms/ExternalAgentForm";
import { CreateButtons } from "./CreateButtons";
import { buildAgentCreationPayload } from "../utils/buildAgentPayload";

export const ExternalAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  const [formData, setFormData] = useState<ConnectAgentFormValues>({
    deploymentType: "existing" as const,
    name: "",
    displayName: "",
    description: "",
  });

  const { errors, validateForm, setFieldError, validateField } = 
    useFormValidation<ConnectAgentFormValues>(connectAgentSchema);

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

  const onSubmit = useCallback(
    (values: ConnectAgentFormValues) => {
      const payload = buildAgentCreationPayload(values, params);
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
          console.error("Failed to register agent:", e);
        },
      });
    },
    [createAgent, navigate, params]
  );

  const handleConnect = useCallback(() => {
    if (!validateForm(formData)) {
      return;
    }
    onSubmit(formData);
  }, [validateForm, formData, onSubmit]);

  const hasAgents = Boolean(agents?.agents?.length && agents?.agents?.length > 0);

  const backHref = useMemo(() => {
    if (!hasAgents) {
      return undefined;
    }
    return generatePath(
      absoluteRouteMap.children.org.children.projects.children.newAgent.path,
      {
        orgId: orgId ?? "",
        projectId: projectId ?? "default",
      }
    );
  }, [hasAgents, orgId, projectId]);

  const isValid = Object.keys(errors).length === 0 &&
    Boolean(formData.name) &&
    Boolean(formData.displayName);

  return (
    <PageLayout
      title="Register an Externally-Hosted Agent"
      description="Provide basic information to register your externally-hosted agent on the platform."
      disableIcon
      backHref={backHref}
      backLabel="Back to Agent Hosting Options"
    >
      <Form.Stack spacing={3}>
        <ExternalAgentForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          setFieldError={setFieldError}
          validateField={validateField}
        />

        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : "Failed to register agent"}
          </Alert>
        )}

        <CreateButtons
          isValid={isValid}
          isPending={isPending}
          onCancel={handleCancel}
          onSubmit={handleConnect}
          mode="connect"
        />
      </Form.Stack>
    </PageLayout>
  );
};
