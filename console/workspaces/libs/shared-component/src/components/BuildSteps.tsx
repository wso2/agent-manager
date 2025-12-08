import { BuildDetailsResponse, BuildStatus, BuildStep } from "@agent-management-platform/types";
import { HelpCircle as QuestionMarkOutlined, XCircle as ErrorOutlined, CheckCircle, ChevronRight as ArrowRight } from "@wso2/oxygen-ui-icons-react";
import { alpha, Box, CircularProgress, Divider, Tooltip, Typography, useTheme } from "@wso2/oxygen-ui";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(relativeTime);

export interface BuildStepsProps {
    build: BuildDetailsResponse
}

const getIcon = (step: BuildStep) => {
    switch (step.status) {
        case "True":
            return <CheckCircle size={16} />;
        case "False":
            return <ErrorOutlined size={16} />;
        default:
            return <QuestionMarkOutlined size={16} />;
    }
}

const getDisplayName = (step: BuildStep) => {
    switch (step.type) {
        case "BuildCompleted":
            return "Build Image";
        case "BuildInitiated":
            return "Initiated";
        case "BuildTriggered":
            return "Triggered";
        case "WorkloadUpdated":
            return "Workload Updated";
        default:
            return step.type;
    }
}


function Step(props: { step: BuildStep, index: number, buildStatus: BuildStatus | undefined }) {
    const { step, index, buildStatus } = props;
    const theme = useTheme();
    const getColor = (isLoading: boolean) => {
        if (isLoading) {
            return theme.palette.warning.main;
        }
        if (step.status === "True") {
            return theme.palette.success.main;
        }
        return theme.palette.error.main;
    }
    const isLoading = !(buildStatus === "Completed" || buildStatus === "BuildFailed") && step.status !== "True";
    const color = getColor(isLoading);
    return (
        <>
            <Tooltip title={step.message}>
                <Box sx={{
                    display: 'flex',
                    gap: 1,
                    px: 2,
                    py: 1,
                    alignItems: 'center',
                    backgroundColor: alpha(color, 0.1),
                    justifyContent: 'space-between',
                    color: color,
                }}>
                    {index > 0 && <ArrowRight size={16}/>}
                    <Box display="flex" gap={1} alignItems="center">
                        {isLoading && <CircularProgress size={12} color="inherit" />}
                        {!isLoading && getIcon(step)}
                        <Typography variant="caption" noWrap>{getDisplayName(step)}</Typography>
                    </Box>
                </Box>
            </Tooltip>
        </>
    )
}

export function BuildSteps(props: BuildStepsProps) {
    const { build } = props;
    return (
        <Box flexDirection="column" gap={2} display="flex">
            <Box display="flex" gap={1} alignItems="center">
                <Typography variant="h6">Pipeline Status</Typography>
                <Divider orientation="vertical" flexItem />
                <Tooltip title={dayjs(build.startedAt).format('DD/MM/YYYY HH:mm:ss')}>
                    <Typography variant="body2" color="text.secondary">
                       Triggered {dayjs(build.startedAt).fromNow()}
                    </Typography>
                </Tooltip>
            </Box>
            <Box sx={{
                display: 'flex', 
                alignItems: 'center',
                borderRadius: 2,
                width: 'fit-content',
                overflow: 'hidden'
            }}>
                {build.steps?.map((step, index) => <Step
                    step={step}
                    key={step.type}
                    index={index}
                    buildStatus={build.status}
                />)}
            </Box>
        </Box >
    )
}
