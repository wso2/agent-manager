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

import React, { useCallback, useMemo } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type UpdateCustomEvaluatorRequest,
} from "@agent-management-platform/types";
import {
  useGetCustomEvaluator,
  useUpdateCustomEvaluator,
} from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { Skeleton, Stack } from "@wso2/oxygen-ui";
import { EvaluatorForm, type EvaluatorFormValues } from "./subComponents/EvaluatorForm";

export const EditEvaluatorComponent: React.FC = () => {
  const { agentId, orgId, projectId, evaluatorId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
    evaluatorId: string;
  }>();
  const navigate = useNavigate();

  const { data: evaluator, isLoading } = useGetCustomEvaluator({
    orgName: orgId!,
    identifier: evaluatorId!,
  });

  const {
    mutate: updateEvaluator,
    isPending,
    error,
  } = useUpdateCustomEvaluator({
    orgName: orgId!,
    identifier: evaluatorId!,
  });

  const evaluatorsRouteMap = agentId
    ? absoluteRouteMap.children.org.children.projects.children.agents
        .children.evaluation.children.evaluators
    : absoluteRouteMap.children.org.children.projects.children.evaluators;

  const routeParams = agentId
    ? { orgId, projectId, agentId }
    : { orgId, projectId };

  const backHref = generatePath(evaluatorsRouteMap.path, routeParams);

  const initialValues: EvaluatorFormValues | undefined = useMemo(() => {
    if (!evaluator) return undefined;
    return {
      displayName: evaluator.displayName,
      description: evaluator.description,
      type: (evaluator.type as "code" | "llm_judge") ?? "code",
      level: evaluator.level as "trace" | "agent" | "llm",
      source: evaluator.source ?? "",
      dependencies: evaluator.dependencies ?? "",
      tags: evaluator.tags ?? [],
    };
  }, [evaluator]);

  const handleSubmit = useCallback(
    (values: EvaluatorFormValues) => {
      const body: UpdateCustomEvaluatorRequest = {
        displayName: values.displayName,
        description: values.description,
        source: values.source,
        dependencies: values.type === "code" && values.dependencies
          ? values.dependencies
          : undefined,
        tags: values.tags,
      };
      updateEvaluator(body, {
        onSuccess: () => {
          navigate(backHref);
        },
      });
    },
    [updateEvaluator, navigate, backHref],
  );

  if (isLoading) {
    return (
      <PageLayout title="Edit Evaluator" disableIcon>
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  return (
    <PageLayout title="Edit Evaluator" disableIcon>
      {initialValues && (
        <EvaluatorForm
          onSubmit={handleSubmit}
          isSubmitting={isPending}
          serverError={error}
          backHref={backHref}
          submitLabel="Save Changes"
          initialValues={initialValues}
          isTypeEditable={false}
          isLevelEditable={false}
        />
      )}
    </PageLayout>
  );
};
