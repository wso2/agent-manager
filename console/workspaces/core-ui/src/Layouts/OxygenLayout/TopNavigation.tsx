import {
  orgRouteId,
  useListAgents,
  useListOrganizations,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  Header,
  HeaderBreadcrumbs,
  type HeaderBreadcrumbsItem,
  type HeaderBreadcrumbsLevel,
} from "@wso2/oxygen-ui";
import { Bot, Building2, Folder } from "@wso2/oxygen-ui-icons-react";
import {
  Component,
  createContext,
  forwardRef,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type MutableRefObject,
  type ReactNode,
} from "react";
import {
  generatePath,
  Link,
  useLocation,
  useNavigate,
  useParams,
  type LinkProps,
} from "react-router-dom";
import { useActiveAgentPage, useActiveOrgPage, useActiveProjectPage } from "./path-map";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  useExternalComponentModules,
  type OrganizationSwitcherState,
} from "@agent-management-platform/views";
import { MountPoints } from "../../types";

/**
 * Isolates a component injected at MountPoints.OrganizationSwitcher, so a render
 * error in it leaves the header with its own organization list.
 */
class OrganizationSwitcherBoundary extends Component<
  { onError: () => void; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch() {
    this.props.onError();
  }

  render() {
    return this.state.failed ? null : this.props.children;
  }
}

/**
 * Where a menu pick should land in place of the picked entry's `href`.
 *
 * Every entry's `href` is that entry's home page, so clicking a name in the trail
 * (or opening an entry in a new tab) goes to that level. Switching from a menu
 * instead keeps the current sub-page, such as an agent's Traces page, so the pick
 * handler stores that path here and RouterLink follows it on a plain click.
 */
const SwitchTargetContext =
  createContext<MutableRefObject<string | null> | null>(null);

const isPlainLeftClick = (event: MouseEvent<HTMLAnchorElement>) =>
  event.button === 0 &&
  !event.metaKey &&
  !event.ctrlKey &&
  !event.shiftKey &&
  !event.altKey;

/**
 * HeaderBreadcrumbs renders linked names and menu entries with an `href`; this
 * maps it onto a router Link so they navigate client-side and still support
 * middle-click / open-in-new-tab.
 */
const RouterLink = forwardRef<
  HTMLAnchorElement,
  Omit<LinkProps, "to"> & { href: string }
>(({ href, onClick, ...rest }, ref) => {
  const switchTarget = useContext(SwitchTargetContext);
  const navigate = useNavigate();
  return (
    <Link
      ref={ref}
      to={href}
      {...rest}
      onClick={(event) => {
        // Runs the breadcrumb's own handler first, which may set a switch target.
        onClick?.(event);
        const target = switchTarget?.current;
        if (!switchTarget || !target) {
          return;
        }
        switchTarget.current = null;
        // A new-tab click opens the entry's home page; a plain click switches.
        if (!event.defaultPrevented && isPlainLeftClick(event)) {
          event.preventDefault();
          navigate(target);
        }
      }}
    />
  );
});
RouterLink.displayName = "RouterLink";

const ORG_LEVEL = "organization";
const PROJECT_LEVEL = "project";
const AGENT_LEVEL = "agent";

const orgLabel = (
  org?: { name: string; displayName: string },
  fallbackName?: string,
) => {
  if (org?.displayName) {
    return org.displayName;
  }
  return fallbackName || org?.name;
};

export function TopNavigation() {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { userInfo } = useAuthHooks();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const commonOrgPages = useActiveOrgPage();
  const commonProjectPages = useActiveProjectPage();
  const commonAgentPages = useActiveAgentPage();

  // Get all organizations
  const { data: organizations } = useListOrganizations();
  const selectedOrganization = useMemo(() => {
    return organizations?.organizations?.find(
      (organization) => orgRouteId(organization) === orgId,
    );
  }, [organizations, orgId]);
  const hasOtherOrgs = (organizations?.total ?? 0) > 1;
  // With a single org, its displayName may not be set yet — fall back to the
  // org name/handle from the user's own IDP claims rather than the raw org id.
  const soloOrgFallbackName = !hasOtherOrgs
    ? userInfo?.ouName || userInfo?.ouHandle
    : undefined;

  // A deployment whose sign-in is bound to one organization can inject the
  // organizations the user can move to (MountPoints.OrganizationSwitcher). They
  // are used only while the injected component reports the current organization
  // among more than one; otherwise the list above is.
  const [organizationSwitcherModule] = useExternalComponentModules(
    MountPoints.OrganizationSwitcher,
  );
  const [reportedSwitcher, setReportedSwitcher] =
    useState<OrganizationSwitcherState | null>(null);
  const externalSwitcher =
    organizationSwitcherModule &&
    reportedSwitcher &&
    reportedSwitcher.organizations.length > 1 &&
    reportedSwitcher.organizations.some(
      (organization) => organization.id === reportedSwitcher.currentId,
    )
      ? reportedSwitcher
      : null;
  // While a switch runs the trail names the organization the console is moving to.
  const switchingToName = externalSwitcher?.organizations.find(
    (organization) => organization.id === externalSwitcher.switchingId,
  )?.name;

  // Get all projects for the organization
  const { data: projects, isLoading: projectsLoading } = useListProjects({
    orgName: orgId,
  });

  // Get all agents for the project
  const { data: agents, isLoading: agentsLoading } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });

  const levels = useMemo<HeaderBreadcrumbsLevel[]>(() => {
    if (!orgId || !organizations?.organizations || !selectedOrganization) {
      return [];
    }

    const orgHome = (id: string) =>
      generatePath(absoluteRouteMap.children.org.path, { orgId: id });

    const orgItems: HeaderBreadcrumbsItem[] = externalSwitcher
      ? externalSwitcher.organizations.map((organization) => ({
          id: organization.id,
          name: organization.name,
          disabled: !!externalSwitcher.switchingId,
        }))
      : organizations.organizations.map((organization) => ({
          id: orgRouteId(organization),
          name:
            orgLabel(
              organization,
              orgRouteId(organization) === orgId ? soloOrgFallbackName : undefined,
            ) ?? orgRouteId(organization),
          href: orgHome(orgRouteId(organization)),
        }));

    const result: HeaderBreadcrumbsLevel[] = [
      {
        id: ORG_LEVEL,
        label: "Organization",
        icon: <Building2 size={16} />,
        items: orgItems,
        selectedId: externalSwitcher ? externalSwitcher.currentId : orgId,
        // Keeps the trail pointing at the console's own org page, and shows the
        // target name while an external switch is in progress.
        selectedItem: {
          id: externalSwitcher ? externalSwitcher.currentId : orgId,
          name:
            switchingToName ??
            orgLabel(selectedOrganization, soloOrgFallbackName) ??
            orgId,
          href: orgHome(orgRouteId(selectedOrganization)),
        },
      },
    ];


    result.push({
      id: PROJECT_LEVEL,
      label: "Project",
      icon: <Folder size={16} />,
      loading: projectsLoading,
      items: (projects?.projects ?? []).map((project) => ({
        id: project.name,
        name: project.displayName,
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          { orgId, projectId: project.name },
        ),
      })),
      selectedId: projectId ?? null,
      createAction: {
        label: "Create a Project",
        href: generatePath(
          absoluteRouteMap.children.org.children.newProject.path,
          { orgId },
        ),
      },
    });

    if (projectId) {
      result.push({
        id: AGENT_LEVEL,
        label: "Agent",
        icon: <Bot size={16} />,
        loading: agentsLoading,
        items: (agents?.agents ?? []).map((agent) => ({
          id: agent.name,
          name: agent.displayName,
          href: generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents
              .path,
            { orgId, projectId, agentId: agent.name },
          ),
        })),
        selectedId: agentId ?? null,
        createAction: {
          label: "Create an Agent",
          href: generatePath(
            absoluteRouteMap.children.org.children.projects.children.newAgent
              .path,
            { orgId, projectId },
          ),
        },
      });
    }

    return result;
  }, [
    orgId,
    projectId,
    agentId,
    organizations,
    selectedOrganization,
    externalSwitcher,
    switchingToName,
    soloOrgFallbackName,
    projects,
    projectsLoading,
    agents,
    agentsLoading,
  ]);

  const switchTarget = useRef<string | null>(null);

  const handleSelect = useCallback(
    (levelId: string, item: HeaderBreadcrumbsItem) => {
      if (levelId === ORG_LEVEL && externalSwitcher) {
        externalSwitcher.onSwitch(item.id);
        return;
      }
      if (!item.href) {
        return;
      }
      // Switching keeps the page the user is on within the current level.
      const subPage =
        levelId === ORG_LEVEL
          ? commonOrgPages
          : levelId === PROJECT_LEVEL
            ? commonProjectPages
            : commonAgentPages;
      const target = subPage ? `${item.href}/${subPage}` : item.href;
      // A click on the entry's link follows this target (see RouterLink). A pick
      // from the search field with Enter has no link click, so navigate here if
      // no link has taken it by the next tick.
      switchTarget.current = target;
      setTimeout(() => {
        if (switchTarget.current === target) {
          switchTarget.current = null;
          navigate(target);
        }
      });
    },
    [externalSwitcher, navigate, commonOrgPages, commonProjectPages, commonAgentPages],
  );

  // Clicking a name in the trail goes to that level's home page, such as the
  // project page from an agent page. HeaderBreadcrumbs renders the name as a
  // button (its `linkComponent` is not applied there), so navigate here.
  const handleNavigate = useCallback(
    (_levelId: string, item: HeaderBreadcrumbsItem) => {
      if (item.href && item.href !== pathname) {
        navigate(item.href);
      }
    },
    [navigate, pathname],
  );

  return (
    <>
      {organizationSwitcherModule && (
        <OrganizationSwitcherBoundary onError={() => setReportedSwitcher(null)}>
          <organizationSwitcherModule.component
            onSwitcherChange={setReportedSwitcher}
          />
        </OrganizationSwitcherBoundary>
      )}
      <Header.Switchers showDivider={false}>
        {levels.length > 0 && (
          <SwitchTargetContext.Provider value={switchTarget}>
            <HeaderBreadcrumbs
              levels={levels}
              linkComponent={RouterLink}
              onSelect={handleSelect}
              onNavigate={handleNavigate}

            />
          </SwitchTargetContext.Provider>
        )}
      </Header.Switchers>
    </>
  );
}
