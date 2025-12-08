import { useGetBuild, useGetBuildLogs } from "@agent-management-platform/api-client";
import { NoDataFound, DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { FileText as DescriptionOutlined, RefreshCw as RefreshOutlined, Logs } from "@wso2/oxygen-ui-icons-react";
import { Box, Typography, Alert, Collapse, Skeleton, Button} from "@wso2/oxygen-ui";
import { BuildSteps } from "./BuildSteps";

export interface BuildLogsProps {
    onClose: () => void;
    orgName: string;
    projName: string;
    agentName: string;
    buildName: string;
}

function LogsSkeleton() {
    return (
        <Box display="flex" flexDirection="column" gap={1}>
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
            <Skeleton variant="rounded" height={20} />
        </Box>)
}

const InfoLoadingSkeleton = () => (
    <Box display="flex" flexDirection="column" gap={1}>
        <Skeleton variant="rounded" height={24} width={200} />
        <Skeleton variant="rounded" height={15} width={150} />
    </Box>)

export function BuildLogs({ buildName, orgName, projName, agentName, onClose }: BuildLogsProps) {
    const { data: buildLogs, error, isLoading, refetch } = useGetBuildLogs({
        orgName,
        projName,
        agentName,
        buildName,
    });

    const { data: build, isLoading: isBuildLoading, error: buildError } = useGetBuild({
        orgName,
        projName,
        agentName,
        buildName,
    });

    const getEmptyStateMessage = () => {
        if (error) {
            return {
                title: "Unable to Load Logs",
                subtitle: "There was an error retrieving the logs. Please try refreshing. If the issue persists, contact support.",
            };
        }

        if (build?.status === "BuildInProgress" || build?.status === "BuildTriggered") {
            return {
                title: "Logs Being Generated",
                subtitle: "Build is in progress. Logs will appear shortly. Try refreshing in a few moments.",
            };
        }

        if (build?.status === "BuildFailed") {
            return {
                title: "Unable to Retrieve Logs",
                subtitle: "The build logs could not be loaded. Please try refreshing or check back later.",
            };
        }

        return {
            title: "Logs Not Loaded",
            subtitle: "Build logs are not currently available. Please try refreshing the page. If the issue persists, there may be a temporary system issue.",
        };
    };

    const emptyState = getEmptyStateMessage();

    return (
        <Box display="flex" flexDirection="column" height="100%">
            <DrawerHeader
                icon={<Logs size={24} />}
                title="Build Details"
                onClose={onClose}
            />
            <DrawerContent>
                {buildLogs?.length && (
                    <Typography variant="body2" color="text.secondary">
                        Build execution logs and output.
                    </Typography>
                )}
                <Box display="flex" flexDirection="column" gap={2}>
                    <Box>
                        {isBuildLoading && <InfoLoadingSkeleton />}
                        {
                            build && <BuildSteps build={build} />
                        }
                    </Box>
                    <Box height="calc(100vh - 200px)" display="flex" gap={1} flexDirection="column">
                        <Box display="flex" justifyContent="space-between" alignItems="center">
                            <Typography variant="h6">
                                Logs
                            </Typography>
                            
                                <Button
                                    size="small"
                                    startIcon={<RefreshOutlined size={16} />}
                                    onClick={() => refetch()}
                                    variant="outlined"
                                    disabled={isLoading}
                                >
                                    Refresh
                                </Button>
                        
                        </Box>
                        {(isLoading) && <LogsSkeleton />}
                        {!!buildLogs?.length && (
                            <Typography component="code" variant="body2" fontFamily="monospace">
                                {buildLogs?.map((log) => log.log).join('\n')}
                            </Typography>
                        )}
                        {(!buildLogs?.length && !isLoading) && (
                            <NoDataFound
                                message={emptyState.title}
                                subtitle={emptyState.subtitle}
                                icon={
                                    <Box sx={{ fontSize: 100, mb: 2, opacity: 0.2, display: 'inline-flex' }}>
                                        <DescriptionOutlined
                                            size={100}
                                            color="inherit"
                                        />
                                    </Box>
                                }
                            />
                        )}

                    </Box>
                </Box>
                <Box display="flex" flexDirection="column" gap={1}>
                    <Collapse in={!!error}>
                        <Alert severity="error">
                            {error?.message ? error.message : "Failed to load build logs. Please try refreshing."}
                        </Alert>
                    </Collapse>
                    <Collapse in={!!buildError}>
                        <Alert severity="error">
                            {buildError?.message ? buildError.message : "Failed to load build details."}
                        </Alert>
                    </Collapse>
                </Box>
            </DrawerContent>
        </Box>
    );
}


