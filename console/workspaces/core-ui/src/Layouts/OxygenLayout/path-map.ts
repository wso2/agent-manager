
import { useMemo } from "react";
import { useMatch } from "react-router-dom";
import { absoluteRouteMap, relativeRouteMap } from "@agent-management-platform/types";

export function usePagePath(path: string) {
    const match = useMatch(path + "/:page/*");
    return { page: match?.params.page ?? null };
}

export function useActiveOrgPage() {
    const { page } = usePagePath(absoluteRouteMap.children.org.path);

    return useMemo(() => {
        if (
            page !== "project" &&
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
    // `agents` is both the listing and the agent-detail prefix; only the bare
    // listing is a project-level page worth keeping when switching projects.
    const onAgentListing = !!useMatch(
        absoluteRouteMap.children.org.children.projects.children.agentListing.path,
    );

    return useMemo(() => {
        if (onAgentListing) {
            return relativeRouteMap.children.org.children.projects.children
                .agentListing.path;
        }
        if (
            page !== "agents" &&
            page !== relativeRouteMap.children.org.children.projects.children.agents.path &&
            page !== relativeRouteMap.children.org.children.projects.children.newAgent.path &&
            page !== relativeRouteMap.children.org.children.projects.children.edit.path
        ) {
            return page;
        }
        return null;
    }, [page, onAgentListing]);
}

const agentPages = relativeRouteMap.children.org.children.projects.children.agents.children;
const STANDARD_AGENT_PAGES = new Set([
    agentPages.edit.path,
    agentPages.security.path,
    agentPages.tryOut.path,
    agentPages.observability.path,
    agentPages.evaluation.path,
]);

export function useActiveAgentPage() {
    const { page } = usePagePath(
        absoluteRouteMap.children.org.children.projects.children.agents.path,
    );

    return useMemo(() => {
        // Standard agent pages own their own nav highlight; everything else
        // (custom/component pages) reuses `page` as the active label.
        if (page && !STANDARD_AGENT_PAGES.has(page)) {
            return page;
        }
        return null;
    }, [page]);
}
