import { Box } from '@wso2/oxygen-ui';
import { BuildCard, DeployCard } from './subComponent';
import { useParams } from 'react-router-dom';
import { useListEnvironments } from '@agent-management-platform/api-client';

export const DeployComponent = () => {
  const { orgId } = useParams();

  const { data: environments } = useListEnvironments({
    orgName: orgId ?? '',
  });
  

  return (
    <Box display="flex" gap={4} pb={4} pt={4}>
      <BuildCard />
      {
        environments?.map((env) => (
          <DeployCard key={env.name} currentEnvironment={env} />
        ))
      }
    </Box>
  );
};

export default DeployComponent;
