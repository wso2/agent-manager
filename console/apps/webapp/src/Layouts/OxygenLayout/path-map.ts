
import { useMemo } from "react";
import { useMatch } from "react-router-dom";
import { absoluteRouteMap, relativeRouteMap } from "@agent-management-platform/types";

export function usePagePath(path: string) {
    const match = useMatch(path + "/:page/*");
    const matchWithSubPage = useMatch(path + "/:page/:subPage/*");
    return { page: match?.params.page ?? null, subPage: matchWithSubPage?.params.subPage ?? null };
}

export function useActiveOrgPage() {
    const { page } = usePagePath(absoluteRouteMap.children.org.path);

    return useMemo(() => {
        if (
            page !== "projects" &&
            page !== relativeRouteMap.children.org.children.projects.path &&
            page !== relativeRouteMap.children.org.children.newProject.path
        ) {
            return page;
        }
        return null;
    }, [page]);
}

export function useActiveProjectPage() {
    const { page } = usePagePath(absoluteRouteMap.children.org.children.projects.path);

    return useMemo(() => {
        if (
            page !== "agents" &&
            page !== relativeRouteMap.children.org.children.projects.children.agents.path &&
            page !== relativeRouteMap.children.org.children.projects.children.newAgent.path
        ) {
            return page;
        }
        return null;
    }, [page]);
}

export function useActiveAgentPage() {
    const { page, subPage } = usePagePath(
        absoluteRouteMap.children.org.children.projects.children.agents.path,
    );

    return useMemo(() => {
        if (
            page !== "environment" &&
            subPage !== "evaluation" &&
            page !==
                relativeRouteMap.children.org.children.projects.children.agents
                    .children.build.path &&
            page !==
                relativeRouteMap.children.org.children.projects.children.agents
                    .children.deployment.path &&
            page !==
                relativeRouteMap.children.org.children.projects.children.agents
                    .children.environment.path &&
            page !==
                relativeRouteMap.children.org.children.projects.children.agents
                    .children.evaluation.path
        ) {
            return page;
        }
        if (subPage && page === "evaluation") {
            return `${page}/${subPage}`;
        }
        return null;
    }, [page, subPage]);
}
