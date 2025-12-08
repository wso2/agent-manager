import React, { useCallback } from 'react';
import { AgentBuild } from './AgentBuild/AgentBuild';
import { FadeIn, DrawerWrapper } from '@agent-management-platform/views';
import { Button, Box } from '@wso2/oxygen-ui';
import { Wrench as BuildOutlined } from '@wso2/oxygen-ui-icons-react';
import { useParams, useSearchParams } from 'react-router-dom';
import { BuildPanel } from '@agent-management-platform/shared-component';

export const BuildComponent: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();

  const { orgId, projectId, agentId } = useParams();

  const isBuildPanelOpen = searchParams.get('buildPanel') === 'open';

  const closeBuildPanel = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete('buildPanel');
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleBuild = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set('buildPanel', 'open');
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  return (
    <FadeIn>
      <Box width="100%" display="flex" justifyContent="flex-end">
        <Button
          onClick={handleBuild}
          variant="contained"
          color="primary"
          startIcon={<BuildOutlined size={16} />}>
          Trigger a Build
        </Button>
      </Box>
      <AgentBuild />
      <DrawerWrapper open={isBuildPanelOpen} onClose={closeBuildPanel}>
        <BuildPanel
          onClose={closeBuildPanel}
          orgName={orgId || ''}
          projName={projectId || ''}
          agentName={agentId || ''}
        />
      </DrawerWrapper>
    </FadeIn >
  );
};

export default BuildComponent;
