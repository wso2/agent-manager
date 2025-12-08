import { Box, Chip, Typography, useTheme } from "@wso2/oxygen-ui";
import { Span } from "@agent-management-platform/types";
import { InfoSection } from "./InfoSection";

interface StatusSectionProps {
    span: Span;
}

export function StatusSection({ span }: StatusSectionProps) {
    const theme = useTheme();

    return (
        <InfoSection title="Status">
            <Box>
                <Typography 
                    variant="caption" 
                    fontWeight="600" 
                    sx={{ 
                        color: theme.palette.text.secondary, 
                        display: 'block', 
                        mb: 0.5 
                    }}
                >
                    Kind
                </Typography>
                <Chip label={span.kind} size="small" />
            </Box>
            
            <Box>
                <Typography 
                    variant="caption" 
                    fontWeight="600" 
                    sx={{ 
                        color: theme.palette.text.secondary, 
                        display: 'block', 
                        mb: 0.5 
                    }}
                >
                    Status
                </Typography>
                <Chip 
                    label={span.status} 
                    size="small"
                    color={
                        span.status === 'OK' || span.status === 'UNSET' 
                            ? 'success' 
                            : 'error'
                    }
                />
            </Box>
        </InfoSection>
    );
}

