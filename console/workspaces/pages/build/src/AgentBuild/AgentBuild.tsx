import React from 'react';
import { Box, Skeleton } from '@mui/material';
import { TopCards } from './subComponents/TopCards';
import { BuildTable } from './subComponents/BuildTable';
import { FadeIn } from '@agent-management-platform/views';
import { useParams } from 'react-router-dom';
import { useGetAgentBuilds } from '@agent-management-platform/api-client';


export function AgentBuildSkeleton() {
  return (
    <Box display="flex" flexDirection="column" gap={1} pt={1}>
      <Box display="flex" justifyContent="space-between" gap={2}>
        <Skeleton variant="rounded" width="100%" height={120} />
        <Skeleton variant="rounded" width="100%" height={120} />
        <Skeleton variant="rounded" width="100%" height={120} />
      </Box>
      <Skeleton variant="rounded" width="100%" height={500} />
      {/* <Skeleton variant="rounded" width="100%" height={500} /> */}
    </Box>
  );
}

export const AgentBuild: React.FC = () => {
  const { agentId, projectId, orgId } = useParams();
  const { isLoading } = useGetAgentBuilds({ orgName: orgId ?? 'default', projName: projectId ?? 'default', agentName: agentId ?? '' });

  if (isLoading) {
    return <AgentBuildSkeleton />;
  }

  return (
    <FadeIn>
      <Box pt={1} gap={2} display="flex" flexDirection="column">
        <TopCards />
        <BuildTable />
      </Box>
    </FadeIn>
  );
};
