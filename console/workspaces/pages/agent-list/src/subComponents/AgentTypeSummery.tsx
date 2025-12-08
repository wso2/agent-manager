import { useListAgents } from "@agent-management-platform/api-client";
import { Link, Rocket as RocketLaunchOutlined } from "@wso2/oxygen-ui-icons-react";
import { Box, Card, CardContent, Divider, Typography} from "@wso2/oxygen-ui";
import { useMemo } from "react";
import { useParams } from "react-router-dom";

const TypeLine = (props: {
    label: string, value: number, icon?: React.ReactNode, bold?: boolean
}) => {
    return (
        <Box display="flex" flexDirection="row" justifyContent="space-between" gap={2}>
            <Typography variant='body2' fontWeight={props.bold ? 'bold' : 'normal'}>
                {props?.icon}
                &nbsp;&nbsp;
                {props.label}
            </Typography>
            <Typography variant='body2' fontWeight={props.bold ? 'bold' : 'normal'}>
                {props.value}
            </Typography>
        </Box>
    );
};
export function AgentTypeSummery() {
    const { orgId } = useParams<{ orgId: string }>();
    const { data } = useListAgents({ orgName: orgId ?? 'default', projName: 'default' });    const [externalCount, internalCount] = useMemo(() => {
        return [data?.agents?.filter((agent) => agent.provisioning.type === 'external')?.length ?? 0, data?.agents?.filter((agent) => agent.provisioning.type === 'internal')?.length ?? 0];
    }, [data]);

    return (
        <Card variant="elevation" sx={{ minWidth: 40 }}>
            <CardContent>
                <Box display="flex" flexDirection="column" gap={1.5}>
                    <Typography variant='h6'>
                        Agent Types
                    </Typography>
                    <TypeLine label="External" value={externalCount} icon={<Link fontSize="inherit" />} />
                    <Divider />
                    <TypeLine label="Internal" value={internalCount} icon={<RocketLaunchOutlined fontSize="inherit" />} />
                    <Divider />
                    <TypeLine label="Total" bold value={data?.agents?.length ?? 0} />
                </Box>
            </CardContent>
        </Card>
    );
}
