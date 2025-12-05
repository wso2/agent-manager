import React, { useCallback } from 'react';
import { AgentBuild } from './AgentBuild/AgentBuild';
import { FadeIn } from '@agent-management-platform/views';
import { Button, Drawer, Box, useTheme } from '@mui/material';
import { BuildOutlined } from '@mui/icons-material';
import { useParams, useSearchParams } from 'react-router-dom';
import { BuildPanel } from '@agent-management-platform/shared-component';

export const BuildComponent: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();

  const { orgId, projectId, agentId } = useParams();
  const theme = useTheme();

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
      <Box width="100%" display="flex" justifyContent="flex-end" py={1}>
        <Button
          onClick={handleBuild}
          variant="contained"
          size="small"
          color="inherit"
          startIcon={<BuildOutlined fontSize="inherit" />}>
          Build Latest
        </Button>
      </Box>
      <AgentBuild />
      <Drawer
        anchor="right"
        open={isBuildPanelOpen}
        onClose={closeBuildPanel}
        sx={{
          zIndex: 1300,
        }}
      >
        <Box
          width={theme.spacing(100)}
          p={2}
          height="100%"
          display="flex"
          flexDirection="column"
          gap={2}
          bgcolor={theme.palette.background.paper}
        >
          <BuildPanel
            onClose={closeBuildPanel}
            orgName={orgId || ''}
            projName={projectId || ''}
            agentName={agentId || ''}
          />
        </Box>
      </Drawer>
    </FadeIn >
  );
};

export default BuildComponent;
