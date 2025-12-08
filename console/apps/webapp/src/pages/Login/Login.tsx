import { useAuthHooks } from "@agent-management-platform/auth";
import { generatePath, Navigate, useLocation } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { Button, Box, Typography } from "@wso2/oxygen-ui";

export function Login() {
    const { isAuthenticated, login, userInfo } = useAuthHooks();
    const { state } = useLocation();
    const from = state?.from?.pathname;
    if (isAuthenticated) {
        return <Navigate to={from ? from : generatePath(absoluteRouteMap.children.org.path, { orgId: userInfo?.orgHandle ?? '' })} />;
    }

    return (
        <Box display="flex" flexDirection="column" alignItems="center" justifyContent="center" minHeight="100vh">
            <Typography variant="h4" gutterBottom>
                Welcome to Agent Management Platform
            </Typography>
            <Button 
                variant="contained" 
                size="large" 
                onClick={login}
                sx={{ mt: 2 }}
            >
                Login
            </Button>
        </Box>
    );
}
