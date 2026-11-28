import {
  ConsoleAction,
  orgRouteId,
  useListAgents,
  useListOrganizations,
  useListProjects,
  useTrack,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  withBasePath,
} from "@agent-management-platform/types";
import {
  Box,
  Header,
  HeaderBreadcrumbs,
  type HeaderBreadcrumbsItem,
  type HeaderBreadcrumbsLevel,
  readHeaderBreadcrumbsState,
  writeHeaderBreadcrumbsState,
} from "@wso2/oxygen-ui";
import { Bot, Building2, Box as ProjectIcon } from "@wso2/oxygen-ui-icons-react";
import {
  Component,
  forwardRef,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type MouseEvent,
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
import { useLevelSwitchHref } from "./navigation/useLevelSwitchHref";
import { AGENT_LEVEL, ORG_LEVEL, PROJECT_LEVEL } from "./navigation/breadcrumbLevels";
import type { LevelGateFrom } from "./navigation/navModel";
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

const isPlainLeftClick = (event: MouseEvent) =>
  event.button === 0 &&
  !event.metaKey &&
  !event.ctrlKey &&
  !event.shiftKey &&
  !event.altKey;

/**
 * HeaderBreadcrumbs renders menu entries and actions with an `href` through this,
 * so they are real links: middle-click / Cmd-click opens them in a new tab.
 *
 * Every entry's `href` is that entry's home page. A plain click on a switch-menu
 * entry (`menuitemradio`) is left to `onSelect`, which keeps the current sub-page
 * (see handleSelect); the link then only closes the menu. A new-tab click skips
 * the breadcrumb's handler so the current tab stays where it is.
 */
const RouterLink = forwardRef<
  HTMLAnchorElement,
  Omit<LinkProps, "to"> & { href: string }
>(({ href, onClick, ...rest }, ref) => (
  <Link
    ref={ref}
    to={href}
    {...rest}
    onClick={(event) => {
      if (!isPlainLeftClick(event)) {
        return;
      }
      onClick?.(event);
      if (rest.role === "menuitemradio") {
        event.preventDefault();
      }
    }}
  />
));
RouterLink.displayName = "RouterLink";


const orgLabel = (
  org?: { name: string; displayName: string },
  fallbackName?: string,
) => {
  if (org?.displayName) {
    return org.displayName;
  }
  return fallbackName || org?.name;
};

/**
 * Names in the trail render as `<button href=…>` (oxygen-ui 0.25.0 does not apply
 * `linkComponent` to them), so they are not links. Open them in a new tab on
 * middle-click and Cmd/Ctrl/Shift-click, as a link would.
 */
const trailNameHref = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null;
  return target?.closest("button[href]")?.getAttribute("href") ?? null;
};

export function TopNavigation() {
  const navigate = useNavigate();
  const { track } = useTrack();
  const { pathname } = useLocation();
  const { userInfo } = useAuthHooks();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  // Moving to another org/project/agent keeps the left-menu item in view.
  const switchHref = useLevelSwitchHref();

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

  // Remember the org and project in the URL in the shared breadcrumb storage
  // (oxygen-ui's HeaderBreadcrumbs persistence), so the next sign-in — or another
  // app on this origin — starts in the same context. On an org-level page the
  // last project of that org is kept rather than cleared.
  useEffect(() => {
    if (!orgId) {
      return;
    }
    const stored = readHeaderBreadcrumbsState()?.value;
    if (projectId) {
      if (stored?.[ORG_LEVEL] !== orgId || stored?.[PROJECT_LEVEL] !== projectId) {
        writeHeaderBreadcrumbsState({ [ORG_LEVEL]: orgId, [PROJECT_LEVEL]: projectId });
      }
    } else if (stored?.[ORG_LEVEL] !== orgId) {
      writeHeaderBreadcrumbsState({ [ORG_LEVEL]: orgId });
    }
  }, [orgId, projectId]);

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
    if (!orgId) {
      return [];
    }

    const orgHome = (id: string) => switchHref({ orgId: id });
    // The console's own org, even when the org list failed or does not include
    // it, so the project and agent levels still render.
    const currentOrgItem: HeaderBreadcrumbsItem = {
      id: externalSwitcher ? externalSwitcher.currentId : orgId,
      name:
        switchingToName ??
        orgLabel(selectedOrganization, soloOrgFallbackName) ??
        orgId,
      href: orgHome(selectedOrganization ? orgRouteId(selectedOrganization) : orgId),
    };

    let orgItems: HeaderBreadcrumbsItem[];
    if (externalSwitcher) {
      // External entries switch through the injected component, so they carry no
      // link. The current one does, so its name still goes to the org page.
      orgItems = externalSwitcher.organizations.map((organization) =>
        organization.id === externalSwitcher.currentId
          ? currentOrgItem
          : {
              id: organization.id,
              name: organization.name,
              disabled: !!externalSwitcher.switchingId,
            },
      );
    } else {
      orgItems = (organizations?.organizations ?? []).map((organization) =>
        orgRouteId(organization) === orgId
          ? currentOrgItem
          : {
              id: orgRouteId(organization),
              name: orgLabel(organization) ?? orgRouteId(organization),
              href: orgHome(orgRouteId(organization)),
            },
      );
    }
    // Always list the current org, even when it is the only one or the org list
    // failed, so the switcher never opens empty.
    if (!orgItems.some((item) => item.id === currentOrgItem.id)) {
      orgItems = [currentOrgItem, ...orgItems];
    }

    const result: HeaderBreadcrumbsLevel[] = [
      {
        id: ORG_LEVEL,
        label: "Organization",
        icon: <Building2 size={16} />,
        items: orgItems,
        selectedId: currentOrgItem.id,
        selectedItem: currentOrgItem,
      },
      {
        id: PROJECT_LEVEL,
        label: "Project",
        icon: <ProjectIcon size={16} />,
        loading: projectsLoading,
        items: (projects?.projects ?? []).map((project) => ({
          id: project.name,
          name: project.displayName,
          href: switchHref({ orgId, projectId: project.name }),
        })),
        selectedId: projectId ?? null,
        createAction: {
          label: "New project",
          href: generatePath(
            absoluteRouteMap.children.org.children.newProject.path,
            { orgId },
          ),
        },
      },
    ];

    if (projectId) {
      result.push({
        id: AGENT_LEVEL,
        label: "Agent",
        icon: <Bot size={16} />,
        loading: agentsLoading,
        items: (agents?.agents ?? []).map((agent) => ({
          id: agent.name,
          name: agent.displayName,
          href: switchHref({ orgId, projectId, agentId: agent.name }),
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
    switchHref,
  ]);

  // Picking another entry from a level's menu (by click, or Enter in its search
  // field) keeps the left-menu item the user is on, such as switching agents
  // while on Traces. The item hrefs already point there (see switchHref).
  const handleSelect = useCallback(
    (levelId: string, item: HeaderBreadcrumbsItem) => {
      if (levelId === ORG_LEVEL && externalSwitcher) {
        if (item.id !== externalSwitcher.currentId) {
          track(ConsoleAction.ContextSwitch, { switch_type: "organization" });
        }
        externalSwitcher.onSwitch(item.id);
        return;
      }
      if (!item.href) {
        return;
      }
      if (levelId === ORG_LEVEL && item.id !== orgId) {
        track(ConsoleAction.ContextSwitch, { switch_type: "organization" });
      } else if (levelId === PROJECT_LEVEL && item.id !== projectId) {
        track(ConsoleAction.ContextSwitch, { switch_type: "project" });
      }
      navigate(item.href);
    },
    [externalSwitcher, navigate, orgId, projectId, track],
  );

  // Clicking a name in the trail moves up to that level and keeps the left-menu
  // item: from an agent's Traces, the project name opens the Traces agent
  // picker for that project. The state lets that picker preselect where the
  // user came from.
  const handleNavigate = useCallback(
    (_levelId: string, item: HeaderBreadcrumbsItem) => {
      if (item.href && item.href !== pathname) {
        const from: LevelGateFrom = { projectId, agentId };
        navigate(item.href, { state: { from } });
      }
    },
    [navigate, pathname, projectId, agentId],
  );

  const openTrailNameInNewTab = (event: MouseEvent) => {
    const href = trailNameHref(event);
    if (!href) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    window.open(withBasePath(href), "_blank", "noopener");
  };

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
          <Box
            onClickCapture={(event: MouseEvent) => {
              if (event.metaKey || event.ctrlKey || event.shiftKey) {
                openTrailNameInNewTab(event);
              }
            }}
            onAuxClick={(event: MouseEvent) => {
              if (event.button === 1) {
                openTrailNameInNewTab(event);
              }
            }}
            sx={{ display: "contents" }}
          >
            <HeaderBreadcrumbs
              levels={levels}
              linkComponent={RouterLink}
              onSelect={handleSelect}
              onNavigate={handleNavigate}
            />
          </Box>
        )}
      </Header.Switchers>
    </>
  );
}
