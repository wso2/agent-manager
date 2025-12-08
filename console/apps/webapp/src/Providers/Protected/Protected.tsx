import { useAuthHooks } from "@agent-management-platform/auth";
import { FullPageLoader } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useNavigate, useLocation, generatePath, useParams } from "react-router-dom";
import { useListOrganizations } from "@agent-management-platform/api-client";

export const Protected = ({ children }: { children: React.ReactNode }) => {
    const { isAuthenticated, isLoadingIsAuthenticated } = useAuthHooks();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const { data: organizations } = useListOrganizations();
    const {orgId} = useParams();

    if (isLoadingIsAuthenticated) {
        return <FullPageLoader />;
    }

    if (!isAuthenticated) {
        navigate(generatePath(absoluteRouteMap.children.login.path), { state: { from: pathname } });
    } else if (organizations?.organizations?.length && !orgId) {
        navigate(generatePath(absoluteRouteMap.children.org.children.projects.path, 
            { orgId: organizations?.organizations[0].name, projectId: 'default' }), { state: { from: pathname } });
    }

    return (
        <>
            {children}
        </>
    );
};
