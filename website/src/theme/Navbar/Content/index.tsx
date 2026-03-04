import React, {useEffect, useState} from 'react';
import clsx from 'clsx';
import {
  ErrorCauseBoundary,
  ThemeClassNames,
  useThemeConfig,
} from '@docusaurus/theme-common';
import {
  splitNavbarItems,
  useNavbarMobileSidebar,
} from '@docusaurus/theme-common/internal';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import AlgoliaSearchBar from '@theme/SearchBar';
import LunrSearchBar from '@theme-original/SearchBar';
import NavbarColorModeToggle from '@theme/Navbar/ColorModeToggle';
import NavbarLogo from '@theme/Navbar/Logo';
import NavbarMobileSidebarToggle from '@theme/Navbar/MobileSidebar/Toggle';
import NavbarSearch from '@theme/Navbar/Search';
import NavbarItem from '@theme/NavbarItem';
import styles from './styles.module.css';

type SearchProvider = 'algolia' | 'lunr';

async function isAlgoliaReachable(algolia: {
  appId?: string;
  apiKey?: string;
  indexName?: string;
}): Promise<boolean> {
  if (!algolia.appId || !algolia.apiKey || !algolia.indexName) {
    return false;
  }

  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), 5000);

  try {
    const response = await fetch(
      `https://${algolia.appId}-dsn.algolia.net/1/indexes/${encodeURIComponent(algolia.indexName)}/query`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Algolia-Application-Id': algolia.appId,
          'X-Algolia-API-Key': algolia.apiKey,
        },
        body: JSON.stringify({
          query: 'health-check',
          hitsPerPage: 1,
        }),
        signal: controller.signal,
      },
    );

    return response.ok;
  } catch {
    return false;
  } finally {
    window.clearTimeout(timeoutId);
  }
}

function useNavbarItems() {
  // TODO temporary casting until ThemeConfig type is improved
  return useThemeConfig().navbar.items;
}

function NavbarItems({items}) {
  return (
    <>
      {items.map((item, i) => (
        <ErrorCauseBoundary
          key={i}
          onError={(error) =>
            new Error(
              `A theme navbar item failed to render.
Please double-check the following navbar item (themeConfig.navbar.items) of your Docusaurus config:
${JSON.stringify(item, null, 2)}`,
              {cause: error},
            )
          }>
          <NavbarItem {...item} />
        </ErrorCauseBoundary>
      ))}
    </>
  );
}

function NavbarContentLayout({left, right}) {
  return (
    <div className="navbar__inner">
      <div
        className={clsx(
          ThemeClassNames.layout.navbar.containerLeft,
          'navbar__items',
        )}>
        {left}
      </div>
      <div
        className={clsx(
          ThemeClassNames.layout.navbar.containerRight,
          'navbar__items navbar__items--right',
        )}>
        {right}
      </div>
    </div>
  );
}

function NavbarSearchSection() {
  const {siteConfig} = useDocusaurusContext();
  const [provider, setProvider] = useState<SearchProvider>('algolia');

  useEffect(() => {
    const algoliaConfig = siteConfig.themeConfig?.algolia as
      | {
          appId?: string;
          apiKey?: string;
          indexName?: string;
        }
      | undefined;

    let cancelled = false;

    isAlgoliaReachable(algoliaConfig ?? {}).then((reachable) => {
      if (cancelled) {
        return;
      }
      setProvider(reachable ? 'algolia' : 'lunr');
    });

    return () => {
      cancelled = true;
    };
  }, [siteConfig.themeConfig]);

  const ActiveSearchBar = provider === 'algolia' ? AlgoliaSearchBar : LunrSearchBar;

  return (
    <NavbarSearch className={styles.navbarSearch}>
      <ActiveSearchBar key={provider} />
    </NavbarSearch>
  );
}

export default function NavbarContent() {
  const mobileSidebar = useNavbarMobileSidebar();
  const items = useNavbarItems();
  const [leftItems, rightItems] = splitNavbarItems(items);
  const rightItemsWithoutSearch = rightItems.filter((item) => item.type !== 'search');

  return (
    <NavbarContentLayout
      left={
        <>
          {!mobileSidebar.disabled && <NavbarMobileSidebarToggle />}
          <NavbarLogo />
          <NavbarItems items={leftItems} />
        </>
      }
      right={
        <>
          <NavbarItems items={rightItemsWithoutSearch} />
          <NavbarColorModeToggle className={styles.colorModeToggle} />
          <NavbarSearchSection />
        </>
      }
    />
  );
}
