import { Box, Typography, IconButton } from "@wso2/oxygen-ui";
import { X as Close, GitBranch as Timeline } from "@wso2/oxygen-ui-icons-react";

interface SpanPanelHeaderProps {
    onClose: () => void;
}

export function SpanPanelHeader({ onClose }: SpanPanelHeaderProps) {
    return (
        <Box 
            sx={{ 
                display: 'flex', 
                justifyContent: 'space-between', 
                alignItems: 'center' 
            }}
        >
            <Typography variant="h4">
                <Timeline fontSize="inherit" />
                &nbsp;
                Span Details
            </Typography>
            <IconButton color="error" size="small" onClick={onClose}>
                <Close />
            </IconButton>
        </Box>
    );
}

