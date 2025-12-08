import { Box, Card, CardContent, Typography, useTheme } from "@wso2/oxygen-ui";
import { ReactNode } from "react";

interface InfoSectionProps {
    title: string;
    children: ReactNode;
}

export function InfoSection({ title, children }: InfoSectionProps) {
    const theme = useTheme();

    return (
        <Box>
            <Typography 
                variant="subtitle2" 
                fontWeight="bold" 
                sx={{ 
                    color: theme.palette.text.secondary, 
                    mb: 1.5 
                }}
            >
                {title}
            </Typography>
            <Card variant="outlined">
                <CardContent>
                    <Box 
                        sx={{ 
                            display: 'flex', 
                            flexDirection: 'column', 
                            gap: 2 
                        }}
                    >
                        {children}
                    </Box>
                </CardContent>
            </Card>
        </Box>
    );
}

