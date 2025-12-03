import { Box, CircularProgress, Skeleton } from "@mui/material";
import { StatusCard } from '@agent-management-platform/views';
import { CheckCircle as CheckCircleIcon, Error, PlayArrow, Warning } from '@mui/icons-material';
import dayjs from 'dayjs';
import duration from 'dayjs/plugin/duration';
import relativeTime from 'dayjs/plugin/relativeTime';
import { BuildStatus } from "@agent-management-platform/types";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
dayjs.extend(duration);
dayjs.extend(relativeTime);
export interface TopCardsProps {
    buildCount: number;
    successfulBuildCount: number;
    latestBuildTime: number;
    latestBuildStatus: string;
    averageBuildTime: number;
}
const getBuildIcon = (status: BuildStatus) => {
    switch (status) {
        case "Completed":
            return <CheckCircleIcon />;
        case "BuildTriggered":
            return <PlayArrow />;
        case "BuildInProgress":
            return <CircularProgress size={20} color="inherit" />;
        case "BuildFailed":
            return <Error />;
        default:
            return <Error />;
    }
}
const percIcon = (percentage: number) => {
    if (isNaN(percentage)) {
        return <CircularProgress size={20} color="inherit" />;
    }
    if (percentage >= 0.9) {
        return <CheckCircleIcon fontSize="small" color="inherit" />;
    }
    else if (percentage >= 0.5) {
        return <Warning fontSize="small" />;
    }
    else {
        return <Error fontSize="small" />;
    }
}
const percIconVariant = (percentage: number) => {
    if (isNaN(percentage)) {
        return "warning";
    }
    if (percentage >= 0.9) {
        return "success";
    }
    else if (percentage >= 0.5) {
        return "warning";
    }
    else {
        return "error";
    }
}


const getBuildIconVariant = (status: BuildStatus): 'success' | 'warning' | 'error' | 'info' => {
    switch (status) {
        case "Completed":
            return "success"; // greenish for success
        case "BuildTriggered":
            return "warning";
        case "BuildInProgress":
            return "warning";
        case "BuildFailed":
            return "error"; // red for failed
        default:
            return "info";
    }
}
const getTagVariant = (status: BuildStatus): 'success' | 'warning' | 'error' | 'info' | 'default' => {
    switch (status) {
        case "Completed":
            return "success";
        case "BuildTriggered":
            return "warning";
        case "BuildInProgress":
            return "warning";
        case "BuildFailed":
            return "error";
        default:
            return "default";
    }
}
const getTagText = (status: BuildStatus) => {
    switch (status) {
        case "Completed":
            return "Success";
        case "BuildFailed":
            return "Failed";
        case "BuildInProgress":
            return "In Progress";
        case "BuildTriggered":
            return "Triggered";
        default:
            return "Unknown";
    }
}
function TopCardsSkeleton() {
    return (
        <Box sx={{
            display: 'grid',
            gap: 2,
            gridTemplateColumns: {
                xs: '1fr',
                md: '1fr 1fr',
                lg: '1fr 1fr 1fr'
            }
        }}>
            <Skeleton variant="rectangular" height={100} />
            <Skeleton variant="rectangular" height={100} />
            <Skeleton variant="rectangular" height={100} />
        </Box>
    );
}
export const TopCards: React.FC = (
) => {
    const { agentId, projectId, orgId } = useParams();
    const { data: builds, isLoading } = useGetAgentBuilds({ orgName: orgId ?? 'default', projName: projectId ?? 'default', agentName: agentId ?? '' });

    // Latest Build
    const latestBuild = builds?.builds[0];
    const latestBuildStatus = latestBuild?.status ?? '';
    const latestBuildStartedTime = latestBuild?.startedAt ?? '';

    // Summery
    const succesfullBuildCount = builds?.builds.filter((build) => build.status === 'Completed').length ?? 0;
    const inProgressBuildCount = builds?.builds.filter((build) => build.status === 'BuildInProgress' || build.status === 'BuildTriggered').length ?? 0;
    const failedBuildCount = builds?.builds.filter((build) => build.status === 'BuildFailed').length ?? 0;


    if (isLoading) {
        return <TopCardsSkeleton />;
    }

    return (
        <Box sx={{
            display: 'grid',
            gap: 2,
            gridTemplateColumns: {
                xs: '1fr',
                md: '1fr 1fr',
                lg: '1fr 1fr 1fr'
            }
        }}>
            <StatusCard
                title="Latest Build"
                value={latestBuild?.status ?? ''}
                subtitle={dayjs(latestBuildStartedTime).fromNow()}
                icon={getBuildIcon(latestBuildStatus as BuildStatus)}
                iconVariant={getBuildIconVariant(latestBuildStatus as BuildStatus)}
                tag={getTagText(latestBuildStatus as BuildStatus)}
                tagVariant={getTagVariant(latestBuildStatus as BuildStatus)}
                minWidth="100%"
            />
            <StatusCard
                title="Build Success Rate"
                value={`${(succesfullBuildCount / Math.max(1, succesfullBuildCount + failedBuildCount) * 100).toFixed(2)}%`}
                subtitle="last 30 days"
                icon={
                    percIcon(succesfullBuildCount / (succesfullBuildCount + failedBuildCount))}
                iconVariant={
                    percIconVariant(succesfullBuildCount
                        / (succesfullBuildCount + failedBuildCount))}
                tag={`${succesfullBuildCount}/${succesfullBuildCount + failedBuildCount}`}
                tagVariant={
                    percIconVariant(succesfullBuildCount
                        / (succesfullBuildCount + failedBuildCount))}
                minWidth="100%"
            />
            <StatusCard
                title="Build Status"
                value={`${inProgressBuildCount}/${builds?.builds.length ?? 0}`}
                subtitle="in progress"
                icon={getBuildIcon(latestBuildStatus as BuildStatus)}
                iconVariant={getBuildIconVariant(latestBuildStatus as BuildStatus)}
                tag={`${inProgressBuildCount}/${builds?.builds.length ?? 0}`}
                tagVariant={getTagVariant(latestBuildStatus as BuildStatus)}
                minWidth="100%"
            />
        </Box>
    );
};
