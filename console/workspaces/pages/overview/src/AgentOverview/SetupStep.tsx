import { Box, Typography } from "@wso2/oxygen-ui";
import { CodeBlock } from "@agent-management-platform/shared-component";

interface SetupStepProps {
    stepNumber: number;
    title: string;
    description?: string;
    code: string;
    language?: string;
    fieldId?: string;
}

export const SetupStep = ({ 
    stepNumber, 
    title, 
    description,
    code, 
    language = "bash",
    fieldId 
}: SetupStepProps) => {
    return (
        <Box display="flex" gap={1} flexDirection="column">
            <Box display="flex" alignItems="center" gap={1}>
                <Box
                    sx={{
                        gap: 2,
                        width: 20,
                        height: 20,
                        borderRadius: '50%',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        bgcolor: (theme) =>
                           theme.palette.primary.main,
                        color: 'primary.contrastText',
                        fontWeight: 600,
                    }}
                >
                    <Typography variant="body2" fontWeight={600}>
                        {stepNumber}
                    </Typography>
                </Box>
                <Typography variant="body1">
                    {title}
                </Typography>
            </Box>
            <Box>
                <CodeBlock
                    code={code}
                    language={language}
                    fieldId={fieldId}
                />
            </Box>
            {description && (
                <Typography variant="body2" color="textSecondary">
                    {description}
                </Typography>
            )}
        </Box>
    );
};

