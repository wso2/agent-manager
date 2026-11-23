/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useEffect, useMemo, useState, type ComponentType } from "react";
import { Footer } from "@wso2/oxygen-ui";
import {
  useExternalComponentModules,
  type FooterLink,
  type FooterLinksProps,
} from "@agent-management-platform/views";
import { MountPoints } from "../../types";

/**
 * The links components injected at MountPoints.FooterLinks report, as footer link
 * elements, and those components to mount (outside the footer: it would drop them).
 * Each link is built here from this copy of Oxygen, so Footer keeps it.
 */
export function useFooterLinkModules() {
  useWarnUnsupportedFooterModules();
  const modules = useExternalComponentModules(MountPoints.FooterLinks);
  const [reported, setReported] = useState<Record<string, FooterLink[]>>({});

  // Memoized so the injected components get a stable onLinksChange (an effect that
  // reports doesn't re-run) and aren't re-rendered on every render of the layout.
  const sources = useMemo(
    () =>
      modules.map((module) => {
        const Source = module.component as unknown as ComponentType<FooterLinksProps>;
        const onLinksChange = (links: FooterLink[] | null) =>
          setReported((current) => ({ ...current, [module.moduleName]: links ?? [] }));
        return <Source key={module.moduleName} onLinksChange={onLinksChange} />;
      }),
    [modules],
  );

  // In module order, then each module's own order. A plain array, so Footer's
  // React.Children.toArray() flattens it and sees each Footer.Link.
  const links = useMemo(
    () =>
      modules.flatMap((module) =>
        (reported[module.moduleName] ?? []).map((link) =>
          renderFooterLink(link, `${module.moduleName}:${link.id}`),
        ),
      ),
    [modules, reported],
  );

  return { links, sources };
}

/**
 * Component modules at the old footer mount points were never shown; the footer drops
 * them. Warn about any still registered there instead of failing silently.
 */
function useWarnUnsupportedFooterModules() {
  const names = [
    ...useExternalComponentModules(MountPoints.BottomLeftPanel),
    ...useExternalComponentModules(MountPoints.BottomRightPanel),
  ]
    .map((module) => module.moduleName)
    .join(", ");
  useEffect(() => {
    if (names) {
      // eslint-disable-next-line no-console
      console.warn(
        `Footer component modules are not rendered (${names}). ` +
          `Use the "${MountPoints.FooterLinks}" mount point (FooterLinksProps) instead.`,
      );
    }
  }, [names]);
}

/**
 * An injected link as a Footer.Link: it navigates to `href`, or without one renders as a
 * button that runs `onClick`.
 */
function renderFooterLink(link: FooterLink, key: string) {
  const newTab = link.newTab ?? /^https?:\/\//i.test(link.href ?? "");
  return (
    <Footer.Link
      key={key}
      href={link.href}
      onClick={link.onClick}
      {...(newTab ? { target: "_blank", rel: "noopener noreferrer" } : {})}
    >
      {link.label}
    </Footer.Link>
  );
}
