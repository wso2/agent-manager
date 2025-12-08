import { Box, Button} from "@wso2/oxygen-ui";
import { Rocket as RocketOutlined, Link } from "@wso2/oxygen-ui-icons-react";

interface SummaryPanelProps {
    isValid: boolean;
    isPending: boolean;
    onCancel: () => void;
    onSubmit: () => void;
    mode?: 'deploy' | 'connect';
}

export const CreateButtons = (
    { isPending, onCancel, onSubmit, mode = 'deploy' }: SummaryPanelProps
) => {
    const isConnectMode = mode === 'connect';    
    return (
        <Box display="flex" flexDirection="column" gap={1}>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
                <Button variant="outlined" color="primary" size='medium' onClick={onCancel}>
                    Cancel
                </Button>
                <Button
                    variant="contained"
                    color="primary"
                    size='medium'
                    startIcon={isConnectMode ? 
                    <Link size={16} /> : 
                    <RocketOutlined size={16} />}
                    onClick={onSubmit}
                    disabled={isPending}
                >
                    {isConnectMode ? 'Register' : 'Deploy'}
                </Button>
            </Box>
        </Box>
    );
};

