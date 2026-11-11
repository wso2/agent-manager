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

import { Fragment, useMemo } from "react";
import { Box, Stack } from "@wso2/oxygen-ui";
import { BuildCard, DeployCard } from "./subComponent";
import { useParams } from "react-router-dom";
import {
  useGetProject,
  useListDeploymentPipelines,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { type Environment } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";

export const DeployComponent = () => {
  const { orgId, projectId } = useParams();

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const { data: project } = useGetProject({ orgName: orgId, projName: projectId });
  const { data: pipelinesData } = useListDeploymentPipelines({ orgName: orgId });

  const pipelineEnvironments = useMemo(() => {
    if (!environments) return [];

    const paths = pipelinesData?.deploymentPipelines
      ?.find((p) => p.name === project?.deploymentPipeline)?.promotionPaths ?? [];

    if (!paths.length) return environments;

    const allTargets = new Set(
      paths.flatMap((p) => p.targetEnvironmentRefs.map((t) => t.name)),
    );
    const adjacency = new Map(
      paths.map((p) => [p.sourceEnvironmentRef, p.targetEnvironmentRefs.map((t) => t.name)]),
    );
    const roots = [...new Set(paths.map((p) => p.sourceEnvironmentRef))]
      .filter((s) => !allTargets.has(s));

    const chain: string[] = [];
    const visited = new Set<string>();
    let current: string | undefined = roots[0];
    while (current && !visited.has(current)) {
      chain.push(current);
      visited.add(current);
      current = (adjacency.get(current) ?? [])[0];
    }
    allTargets.forEach((t) => { if (!visited.has(t)) chain.push(t); });

    return chain
      .map((name) => environments.find((e) => e.name === name))
      .filter(Boolean) as Environment[];
  }, [environments, pipelinesData, project?.deploymentPipeline]);

  return (
    <PageLayout title="Deploy" disableIcon>
      <Stack direction="row" pb={4} width="100%" minHeight="calc(100vh - 300px)" overflow="auto">
        <BuildCard initialEnvironment={pipelineEnvironments[0]} />
        {pipelineEnvironments.map((env) => (
          <Fragment key={env.name}>
            <Box
              sx={(theme) => ({
                width: theme.spacing(4),
                height: theme.spacing(0.5),
                minWidth: theme.spacing(4),
                mt: theme.spacing(14),
                bgcolor: "divider",
              })}
            />
            <DeployCard currentEnvironment={env} />
          </Fragment>
        ))}
      </Stack>
    </PageLayout>
  );
};

export default DeployComponent;
