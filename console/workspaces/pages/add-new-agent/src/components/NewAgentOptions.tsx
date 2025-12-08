import { Box } from "@wso2/oxygen-ui";
import { NewAgentTypeCard } from "./NewAgentTypeCard";
import { ImageList } from "@agent-management-platform/views";
// import  from "node_modules/@agent-management-platform/views/dist/component/Image/Image";

interface NewAgentOptionsProps {
    onSelect: (option: 'new' | 'existing') => void;
}

export const NewAgentOptions = ({ onSelect }: NewAgentOptionsProps) => {
    const handleSelect = (type: string) => {
        onSelect(type as 'new' | 'existing');
    };

    return (
        <Box display="flex" flexDirection="row" gap={3} py={2} width={1}>
            <NewAgentTypeCard
                type="existing"
                title="Externally-Hosted Agent"
                subheader="Connect an existing agent running outside the platform and enable observability and governance."
                icon={<img src={ImageList.EXTERNAL_AGENT} width={200} height={400} alt="External Agent" />}
                onClick={handleSelect}
                content={
                    <Box />
                }
            />
            <NewAgentTypeCard
                type="new"
                title="Platform-Hosted Agent"
                subheader="Platform-Hosted Agent
Description: Deploy and manage agents with full lifecycle support, including built-in CI/CD, scaling, observability, and governance."
                icon={<img src={ImageList.INTERNAL_AGENT} width={200} height={400} alt="Internal Agent" />}
                onClick={handleSelect}
                content={
                    <Box />
                }
            />
        </Box>
    );
};
