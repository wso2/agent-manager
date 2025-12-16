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
    const { data } = useListAgents({ orgName: orgId, projName: 'default' });    const [externalCount, internalCount] = useMemo(() => {
        return [data?.agents?.filter((agent) => agent.provisioning.type === 'external')?.length ?? 0, data?.agents?.filter((agent) => agent.provisioning.type === 'internal')?.length ?? 0];
    }, [data]);

    return (
        <Card variant="outlined" sx={{ minWidth: 300, flexGrow: 1, "&.MuiCard-root": { backgroundColor: "background.paper" } }}>
            <CardContent>
                <Box display="flex" flexDirection="column" gap={1.5}>
                    <Typography variant='h6'>
                        Agent Deployments
                    </Typography>
                    <TypeLine label="External" value={externalCount} icon={<Link size={16} />} />
                    <Divider />
                    <TypeLine label="Platform" value={internalCount} icon={<RocketLaunchOutlined size={16} />} />
                    <Divider />
                    <TypeLine label="Total" bold value={data?.agents?.length ?? 0} />
                </Box>
            </CardContent>
        </Card>
    );
}
