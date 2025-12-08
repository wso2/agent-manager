import { Box, Card, CardContent, Typography, alpha, useTheme } from "@wso2/oxygen-ui";
import { useCallback } from "react";

interface AttributesSectionProps {
    attributes: Record<string, unknown>;
}

export function AttributesSection({ attributes }: AttributesSectionProps) {
    const theme = useTheme();

    const isJsonObject = useCallback((value: unknown): boolean => {
        if (value === null || value === undefined) return false;
        
        if (typeof value === 'object' || Array.isArray(value)) return true;
        
        if (typeof value === 'string') {
            try {
                const parsed = JSON.parse(value);
                return typeof parsed === 'object' && parsed !== null;
            } catch {
                return false;
            }
        }
        
        return false;
    }, []);

    const renderAttributeValue = useCallback((value: unknown): string => {
        if (value === null) return 'null';
        if (value === undefined) return 'undefined';
        
        if (typeof value === 'object' || Array.isArray(value)) {
            try {
                return JSON.stringify(value, null, 2);
            } catch {
                return String(value);
            }
        }
        
        if (typeof value === 'string') {
            try {
                const parsed = JSON.parse(value);
                if (typeof parsed === 'object' && parsed !== null) {
                    return JSON.stringify(parsed, null, 2);
                }
            } catch {
                // Not valid JSON, return as-is
            }
        }
        
        return String(value);
    }, []);

    if (!attributes || Object.keys(attributes).length === 0) {
        return null;
    }

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
                Attributes
            </Typography>
            <Box 
                sx={{ 
                    display: 'flex', 
                    flexDirection: 'column', 
                    gap: 2 
                }}
            >
                {Object.entries(attributes).map(([key, value]) => (
                    <Box key={key}>
                        <Typography 
                            variant="caption" 
                            fontWeight="600"
                            sx={{
                                color: theme.palette.text.secondary,
                                display: 'block',
                                mb: 0.75
                            }}
                        >
                            {key}
                        </Typography>
                        <Card
                            variant="outlined"
                            sx={{
                                maxHeight: isJsonObject(value) ? 375 : 'auto',
                                overflow: 'auto',
                                bgcolor: theme.palette.mode === 'dark' 
                                    ? alpha(theme.palette.common.black, 0.2)
                                    : alpha(theme.palette.common.black, 0.03),
                            }}
                        >
                            <CardContent sx={{ '&:last-child': { pb: 1.5 } }}>
                                <Typography 
                                    component="pre"
                                    variant="body2" 
                                    sx={{
                                        fontFamily: 'monospace',
                                        m: 0,
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        fontSize: theme.typography.caption.fontSize,
                                        lineHeight: 1.6,
                                        color: theme.palette.text.primary
                                    }}
                                >
                                    {renderAttributeValue(value)}
                                </Typography>
                            </CardContent>
                        </Card>
                    </Box>
                ))}
            </Box>
        </Box>
    );
}

