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

import React, { useCallback } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type CreateCustomEvaluatorRequest,
} from "@agent-management-platform/types";
import { useCreateCustomEvaluator } from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { EvaluatorForm, type EvaluatorFormValues } from "./subComponents/EvaluatorForm";
import { SectionErrorBoundary } from "./subComponents/SectionErrorBoundary";

export const CreateEvaluatorComponent: React.FC = () => {
  const { agentId, orgId, projectId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();

  const {
    mutate: createEvaluator,
    isPending,
    error,
  } = useCreateCustomEvaluator({
    orgName: orgId!,
  });

  const evaluatorsRouteMap = agentId
    ? absoluteRouteMap.children.org.children.projects.children.agents
        .children.evaluation.children.evaluators
    : absoluteRouteMap.children.org.children.projects.children.evaluators;

  const routeParams = agentId
    ? { orgId, projectId, agentId }
    : { orgId, projectId };

  const backHref = generatePath(evaluatorsRouteMap.path, routeParams);

  const handleSubmit = useCallback(
    (values: EvaluatorFormValues) => {
      const body: CreateCustomEvaluatorRequest = {
        displayName: values.displayName,
        description: values.description,
        type: values.type,
        level: values.level,
        source: values.source,
        tags: values.tags,
        configSchema: values.configSchema,
      };
      createEvaluator(body, {
        onSuccess: () => {
          navigate(backHref);
        },
      });
    },
    [createEvaluator, navigate, backHref],
  );

  return (
    <PageLayout title="Create Evaluator" disableIcon>
      <SectionErrorBoundary fallbackMessage="The evaluator form failed to render. Click Retry to try again.">
        <EvaluatorForm
          onSubmit={handleSubmit}
          isSubmitting={isPending}
          serverError={error}
          backHref={backHref}
          submitLabel="Create Evaluator"
        />
      </SectionErrorBoundary>
    </PageLayout>
  );
};
