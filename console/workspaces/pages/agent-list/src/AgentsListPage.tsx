import React, { useCallback, useMemo, useState } from 'react';
import { Box, TextField, Typography, Avatar, ButtonBase, Button, Alert, useTheme, Tooltip, Skeleton, Chip, alpha } from '@wso2/oxygen-ui';
import { Clock as AccessTimeRounded, Plus as Add, Trash2 as DeleteOutlineOutlined, Search as SearchRounded } from '@wso2/oxygen-ui-icons-react';
import { PageLayout, DataListingTable, TableColumn, BackgoundLoader, NoDataFound, FadeIn, InitialState } from '@agent-management-platform/views';
import { generatePath, Link, useNavigate, useParams } from 'react-router-dom';
import { absoluteRouteMap, AgentResponse, Provisioning } from '@agent-management-platform/types';
import { useListAgents, useDeleteAgent } from '@agent-management-platform/api-client';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import { AgentTypeSummery } from './subComponents/AgentTypeSummery';

dayjs.extend(relativeTime);


export function ListPageSkeleton() {
  return (
    <Box display="flex" flexDirection="column" gap={2} p={2}>
      <Box display="flex" flexDirection="row" justifyContent="space-between" gap={2}>
        <Box display="flex" flexDirection="column" gap={2}>
          <Skeleton variant="rounded" width={100} height={40} />
          <Skeleton variant="rounded" width={400} height={20} />
        </Box>
        <Skeleton variant="rounded" height={40} width={150} />
      </Box>
      <Box display="flex" flexDirection="column" gap={2}>
        <Skeleton variant="rounded" width="100%" height={40} />
        <Skeleton variant="rounded" width="100%" height={450} />
      </Box>
    </Box>
  );
}

export interface AgentWithHref extends AgentResponse {
  href: string;
  id: string;
  agentInfo: { agentName: string; description: string };
}

export const AgentsListPage: React.FC = () => {
  const theme = useTheme();
  const [search, setSearch] = useState('');
  const [hoveredAgentId, setHoveredAgentId] = useState<string | null>(null);

  // Detect touch device for alternative interaction pattern
  const isTouchDevice = typeof window !== 'undefined' && ('ontouchstart' in window || navigator.maxTouchPoints > 0);

  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const { data, isLoading, error, isRefetching } = useListAgents({ orgName: orgId ?? 'default', projName: 'default' });
  const { mutate: deleteAgent } = useDeleteAgent();

  const handleDeleteAgent = useCallback((agentId: string) => {
    deleteAgent({ orgName: orgId ?? 'default', projName: 'default', agentName: agentId });
  }, [deleteAgent, orgId]);

  const handleRowMouseEnter = useCallback((row: AgentResponse & { id: string }) => {
    setHoveredAgentId(row.id);
  }, []);

  const handleRowMouseLeave = useCallback(() => {
    setHoveredAgentId(null);
  }, []);

  const getAgentPath = (isInternal: boolean) => {
    let path = absoluteRouteMap.children.org.children.projects.children.agents.path;
    if (isInternal) {
      path = absoluteRouteMap.children.org.children.projects.children.agents.path;
    }
    return path;
  }

  const agentsWithHref:AgentWithHref[] = useMemo(() => data?.agents?.filter(
    (agent: AgentResponse) => agent.displayName.toLowerCase()
      .includes(search.toLowerCase())).map((agent) => ({
        ...agent,
        href: generatePath(
          getAgentPath(agent.provisioning.type === 'internal'),
          {
            orgId: orgId ?? '',
            projectId: agent.projectName,
            agentId: agent.name
          }
        ),
        id: agent.name,
        agentInfo: { agentName: agent.displayName, description: agent.description },
      })) ?? [], [data?.agents, search, orgId]);

  const columns = useMemo(() => [
    {
      id: 'agentInfo',
      label: 'Agent Name',
      sortable: true,
      width: '25%',
      render: (value, row) => {
        const agentInfo = value as { agentName: string; description: string };
        return (
          <ButtonBase component={Link} to={row?.href}>
            <Box display="flex" alignItems="center" gap={1}>
              <Avatar
                variant='circular'
                sx={{
                  backgroundColor: alpha(theme.palette.primary.main, 0.1),
                  color: theme.palette.primary.main,
                  height: 40,
                  width: 40,
                }}
              >
                {agentInfo.agentName.substring(0, 1).toUpperCase()}
              </Avatar>
              <Box display="flex" alignItems="center" gap={1}>
                <Typography variant='body1'>{agentInfo.agentName}</Typography>
              </Box>
            </Box>
          </ButtonBase>
        );
      }
    },
    {
      id: 'description',
      label: 'Description',
      sortable: true,
      width: '30%',
      render: (value) => (
        <Typography variant='body2' color='text.secondary'>
          {(value as string) || ''}
        </Typography>
      ),
    },
    {
      id: 'provisioning',
      label: 'Provisioning Type',
      width: '10%',
      align: 'center',
      render: (value) => (
        <Chip
          label={(value as Provisioning).type === 'external' ? 'External' : 'Internal'}
          size="small"
          variant="outlined"
          color={(value as Provisioning).type === 'external' ? 'secondary' : 'default'}
        />
      ),
    },
    {
      id: 'createdAt',
      label: 'Last Updated',
      sortable: true,
      width: '20%',
      align: 'right',
      render: (value, row) => (
        <Box
          display="flex"
          alignItems="center"
          gap={1}
          justifyContent="flex-end"
          sx={{ minWidth: 150 }} // Prevent layout shift
        >
          {(hoveredAgentId === row?.id || isTouchDevice) ? (
            <Box display="flex" alignItems="center" gap={1} justifyContent="flex-end">
              <FadeIn>
                <Tooltip title="Delete Agent">
                  <Button
                    startIcon={<DeleteOutlineOutlined fontSize='inherit' />}
                    color='error'
                    variant='outlined'
                    size='small'
                    onClick={(e) => {
                      e.stopPropagation(); // Prevent row click if any
                      handleDeleteAgent(row.name);
                    }}
                    sx={{
                      // On touch devices, show with reduced opacity when not hovered
                      opacity: isTouchDevice && hoveredAgentId !== row?.id ? 0.7 : 1,
                    }}
                  >
                    Delete
                  </Button>
                </Tooltip>
              </FadeIn>
            </Box>
          ) : (
            <>
              <AccessTimeRounded fontSize='small' color="disabled" />
              <Typography variant='body2' color='text.secondary' noWrap>
                {dayjs(value as string).fromNow()}
              </Typography>
            </>
          )}
        </Box>
      ),
    },
  ] as TableColumn<AgentWithHref>[], [theme, handleDeleteAgent, hoveredAgentId, isTouchDevice]);

  // Define initial state for sorting - most recently updated agents first
  const tableInitialState: InitialState<AgentWithHref> = useMemo(() => ({
    sorting: {
      sortModel: [{
        field: 'createdAt',
        sort: 'desc'
      }]
    }
  }), []);

  if (isLoading) {
    return <ListPageSkeleton />;
  }

  return (
    <PageLayout
      title="Agents"
      description='Manage and monitor all your AI agents across environments'
    >
      {isRefetching && <BackgoundLoader />}
      <Box display="flex" justifyContent="space-between" gap={2}>
        <Box py={2} sx={{
          display: 'flex',
          flexGrow: 1,
          flexDirection: 'column',
          gap: 2,
        }}>

          <Box display="flex" justifyContent="flex-end" gap={1}>
            <TextField
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              slotProps={{ input: { startAdornment: <SearchRounded fontSize='small' /> } }}
              fullWidth
              size='small'
              sx={{
                m: 0,
              }}
              variant='outlined'
              placeholder='Search agents '
              disabled={!data?.agents?.length}
            />
            <Button
              variant='contained'
              color='primary'
              size='small'
              startIcon={<Add fontSize='inherit' />}
              onClick={() => navigate(generatePath(absoluteRouteMap.children.org.children.projects.children.newAgent.path, { orgId: orgId ?? '', projectId: 'default' }))
              }>
              <Typography noWrap fontSize="inherit">
                Add Agent
              </Typography>
            </Button>
          </Box>
          {error && (
            <Alert severity="error" variant='outlined'>
              {error.message}
            </Alert>
          )}

          {(!isLoading && !!data?.agents?.length) && (
            <DataListingTable
              data={agentsWithHref}
              columns={columns}
              pagination={true}
              pageSize={5}
              maxRows={agentsWithHref?.length}
              initialState={tableInitialState}
              onRowMouseEnter={handleRowMouseEnter}
              onRowMouseLeave={handleRowMouseLeave}
              onRowFocusIn={handleRowMouseEnter}
              onRowFocusOut={handleRowMouseLeave}
              onRowClick={(row) => navigate(row?.href)}
            />
          )}

          {!isLoading && !data?.agents?.length && (
            <Box sx={{
              boxShadow: theme.shadows[1],
              backgroundColor: theme.palette.background.paper,
              borderRadius: theme.shape.borderRadius,
              p: 2.5, // 20px equivalent
            }}>
              <NoDataFound
                message="No agents found"
                action={
                  <Button
                    variant='contained'
                    color='primary'
                    startIcon={<Add />}
                    onClick={() => navigate(generatePath(absoluteRouteMap.children.org.children.projects.children.newAgent.path, { orgId: orgId ?? '', projectId: 'default' }))
                    }>
                    Add New Agent
                  </Button>
                }
              />
            </Box>
          )}
        </Box>
        <Box pt={2}>
          <AgentTypeSummery />
        </Box>
      </Box>
    </PageLayout>
  );
};

export default AgentsListPage;


