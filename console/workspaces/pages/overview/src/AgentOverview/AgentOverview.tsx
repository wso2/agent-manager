import { useGetAgent } from "@agent-management-platform/api-client";
import { InternalAgentOverview } from "./InternalAgentOverview";
import { useParams } from "react-router-dom";
import { ExternalAgentOverview } from "./ExternalAgentOverview";

export function AgentOverview() {
    const { orgId, agentId, projectId } = useParams();
    const { data: agent } = useGetAgent({
        orgName: orgId ?? 'default',
        projName: projectId ?? 'default',
        agentName: agentId ?? ''
    });

    if (agent?.provisioning.type === 'internal') {
        return (
            <InternalAgentOverview />
        )
    }
    if (agent?.provisioning.type === 'external') {
        return (
            <ExternalAgentOverview />
        )
    }

    return null;
}
