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
import AlgoliaSearchBar from '@theme/SearchBar';
import LunrSearchBar from '@theme-original/SearchBar';
import NavbarColorModeToggle from '@theme/Navbar/ColorModeToggle';
import NavbarLogo from '@theme/Navbar/Logo';
import NavbarMobileSidebarToggle from '@theme/Navbar/MobileSidebar/Toggle';
import NavbarSearch from '@theme/Navbar/Search';
import NavbarItem from '@theme/NavbarItem';
import styles from './styles.module.css';

type SearchProvider = 'algolia' | 'lunr';

const SEARCH_PROVIDER_STORAGE_KEY = 'amp-docs-search-provider';

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

function SearchProviderToggle({
  provider,
  onChange,
}: {
  provider: SearchProvider;
  onChange: (provider: SearchProvider) => void;
}) {
  return (
    <div className={styles.searchProviderToggle} role="group" aria-label="Search provider">
      <button
        type="button"
        className={clsx(
          styles.searchProviderButton,
          provider === 'algolia' && styles.searchProviderButtonActive,
        )}
        onClick={() => onChange('algolia')}>
        Algolia
      </button>
      <button
        type="button"
        className={clsx(
          styles.searchProviderButton,
          provider === 'lunr' && styles.searchProviderButtonActive,
        )}
        onClick={() => onChange('lunr')}>
        Lunr
      </button>
    </div>
  );
}

function NavbarSearchSection() {
  const [provider, setProvider] = useState<SearchProvider>('algolia');

  useEffect(() => {
    const storedProvider = window.localStorage.getItem(SEARCH_PROVIDER_STORAGE_KEY);
    if (storedProvider === 'algolia' || storedProvider === 'lunr') {
      setProvider(storedProvider);
    }
  }, []);

  const handleProviderChange = (nextProvider: SearchProvider) => {
    setProvider(nextProvider);
    window.localStorage.setItem(SEARCH_PROVIDER_STORAGE_KEY, nextProvider);
  };

  const ActiveSearchBar = provider === 'algolia' ? AlgoliaSearchBar : LunrSearchBar;

  return (
    <NavbarSearch className={styles.navbarSearchWithToggle}>
      <div className={styles.searchControls}>
        <SearchProviderToggle provider={provider} onChange={handleProviderChange} />
        <ActiveSearchBar key={provider} />
      </div>
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
