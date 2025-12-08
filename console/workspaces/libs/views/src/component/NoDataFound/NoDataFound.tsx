import { Box, Typography} from "@wso2/oxygen-ui";
import { SearchX as SearchOffOutlined } from "@wso2/oxygen-ui-icons-react";
import { FadeIn } from "../FadeIn/FadeIn";
import { ReactNode } from "react";

interface NoDataFoundProps {
    message?: string;
    action?: ReactNode;
    icon?: ReactNode;
    subtitle?: string;
}

export function NoDataFound({ 
    message = "No data found", 
    action,
    icon,
    subtitle
}: NoDataFoundProps) {    return (
        <FadeIn>
            <Box sx={{
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
                color: 'text.secondary',
                p: 2,
                gap: 1
            }}>
                {icon || (
                    <Box sx={{ fontSize: 100, mb: 2, opacity: 0.2, display: 'inline-flex' }}>
                        <SearchOffOutlined size={100} color="primary" />
                    </Box>
                )}
                <Typography variant="h6" align="center" color="textSecondary" sx={{ mb: subtitle ? 1 : 2 }}>
                    {message}
                </Typography>
                {subtitle && (
                    <Typography variant="body2" align="center" color="textSecondary" sx={{ mb: 2, opacity: 0.7 }}>
                        {subtitle}
                    </Typography>
                )}
                {action && (
                    <Box sx={{ mt: 2 }}>
                        {action}
                    </Box>
                )}
            </Box>
        </FadeIn>
    );
}
