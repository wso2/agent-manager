# Oxygen UI Component Reference

Complete API reference for custom Oxygen UI components. For MUI components, refer to [Material-UI documentation](https://mui.com/material-ui/).

## Table of Contents

- [OxygenUIThemeProvider](#oxygenuitthemeprovider)
- [ListingTable](#listingtable)
- [AppShell](#appshell)
- [Header](#header)
- [Sidebar](#sidebar)
- [Form](#form)
- [NotificationPanel](#notificationpanel)
- [Other Components](#other-components)
- [MUI X Namespaces](#mui-x-namespaces)
- [Hooks](#hooks)

---

## OxygenUIThemeProvider

Theme provider that wraps your application. Required at the root level.

```tsx
import { OxygenUIThemeProvider, OxygenTheme } from '@wso2/oxygen-ui';
```

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `children` | `ReactNode` | required | App content |
| `theme` | `Theme` | `OxygenTheme` | Single theme (disables switching) |
| `themes` | `ThemeOption[]` | - | Array of themes (enables switching) |
| `initialTheme` | `string` | First theme key | Initial theme key |

### ThemeOption Type

```tsx
interface ThemeOption {
  key: string;    // Unique identifier
  label: string;  // Display name
  theme: Theme;   // MUI Theme object
}
```

### Basic Usage

```tsx
// Single theme (no switching)
<OxygenUIThemeProvider theme={OxygenTheme}>
  <App />
</OxygenUIThemeProvider>
```

### Theme Switching

```tsx
import {
  OxygenUIThemeProvider,
  OxygenTheme,
  AcrylicOrangeTheme,
  AcrylicPurpleTheme,
} from '@wso2/oxygen-ui';

const themes = [
  { key: 'default', label: 'Default', theme: OxygenTheme },
  { key: 'orange', label: 'Orange', theme: AcrylicOrangeTheme },
  { key: 'purple', label: 'Purple', theme: AcrylicPurpleTheme },
];

<OxygenUIThemeProvider themes={themes} initialTheme="default">
  <App />
</OxygenUIThemeProvider>
```

---

## ListingTable

Advanced data table with compound component pattern. Supports search, sort, pagination, selection, and bulk actions.

```tsx
import { ListingTable } from '@wso2/oxygen-ui';
```

### Sub-components

| Component | Description |
|-----------|-------------|
| `ListingTable.Provider` | Context provider for state management |
| `ListingTable.Container` | Paper wrapper with styling |
| `ListingTable.Toolbar` | Search, filters, and actions bar |
| `ListingTable.Head` | Table header wrapper |
| `ListingTable.Body` | Table body wrapper |
| `ListingTable.Footer` | Table footer (pagination) |
| `ListingTable.Row` | Table row |
| `ListingTable.Cell` | Table cell |
| `ListingTable.SortLabel` | Sortable column header |
| `ListingTable.DensityControl` | Density toggle button |
| `ListingTable.EmptyState` | Empty state display |
| `ListingTable.RowActions` | Row action buttons |
| `ListingTable.CellIcon` | Icon in cell |

### ListingTable Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `variant` | `'table' \| 'card'` | `'table'` | Display style |
| `density` | `'compact' \| 'standard' \| 'comfortable'` | `'standard'` | Row padding |
| `striped` | `boolean` | `false` | Alternating row colors |
| `bordered` | `boolean` | `false` | Cell borders |

### Provider Props (Context State)

```tsx
interface ListingTableProviderProps {
  // Search
  searchValue?: string;
  onSearchChange?: (value: string) => void;

  // Sort
  sortField?: string;
  sortDirection?: 'asc' | 'desc';
  onSortChange?: (field: string, direction: 'asc' | 'desc') => void;

  // Pagination
  page?: number;                    // 0-indexed
  rowsPerPage?: number;
  totalCount?: number;
  onPageChange?: (page: number) => void;
  onRowsPerPageChange?: (rowsPerPage: number) => void;

  // Selection
  selected?: readonly string[];
  onSelectionChange?: (selected: readonly string[]) => void;
  onSelectAll?: () => void;
  onClearSelection?: () => void;
  isSelected?: (id: string) => boolean;

  // Filters
  filters?: Record<string, unknown>;
  onFilterChange?: (key: string, value: unknown) => void;
  onClearFilters?: () => void;

  // Bulk Actions
  onBulkDelete?: (ids: readonly string[]) => void;
  onBulkAction?: (actionId: string, ids: readonly string[]) => void;

  // UI State
  density?: 'compact' | 'standard' | 'comfortable';
  onDensityChange?: (density: ListingTableDensity) => void;
  loading?: boolean;
}
```

### Toolbar Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `showSearch` | `boolean` | `false` | Show built-in search |
| `searchSlot` | `ReactNode` | - | Custom search component |
| `searchPlaceholder` | `string` | `'Search...'` | Search placeholder |
| `actions` | `ReactNode` | - | Right-side actions |
| `children` | `ReactNode` | - | Center content |

### Basic Table

```tsx
<ListingTable.Container>
  <ListingTable>
    <ListingTable.Head>
      <ListingTable.Row>
        <ListingTable.Cell>Name</ListingTable.Cell>
        <ListingTable.Cell>Status</ListingTable.Cell>
        <ListingTable.Cell>Actions</ListingTable.Cell>
      </ListingTable.Row>
    </ListingTable.Head>
    <ListingTable.Body>
      {data.map((row) => (
        <ListingTable.Row key={row.id}>
          <ListingTable.Cell>{row.name}</ListingTable.Cell>
          <ListingTable.Cell>{row.status}</ListingTable.Cell>
          <ListingTable.Cell>
            <ListingTable.RowActions
              actions={[
                { id: 'edit', label: 'Edit', onClick: () => edit(row.id) },
                { id: 'delete', label: 'Delete', onClick: () => delete(row.id) },
              ]}
            />
          </ListingTable.Cell>
        </ListingTable.Row>
      ))}
    </ListingTable.Body>
  </ListingTable>
</ListingTable.Container>
```

### Full-Featured Table with Context

```tsx
const [search, setSearch] = useState('');
const [sortField, setSortField] = useState('name');
const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
const [page, setPage] = useState(0);
const [rowsPerPage, setRowsPerPage] = useState(10);

<ListingTable.Provider
  searchValue={search}
  onSearchChange={setSearch}
  sortField={sortField}
  sortDirection={sortDirection}
  onSortChange={(field, dir) => {
    setSortField(field);
    setSortDirection(dir);
  }}
  page={page}
  rowsPerPage={rowsPerPage}
  totalCount={data.length}
  onPageChange={setPage}
  onRowsPerPageChange={setRowsPerPage}
>
  <ListingTable.Container>
    <ListingTable.Toolbar
      showSearch
      actions={<ListingTable.DensityControl />}
    />
    <ListingTable variant="card">
      <ListingTable.Head>
        <ListingTable.Row>
          <ListingTable.Cell>
            <ListingTable.SortLabel field="name">Name</ListingTable.SortLabel>
          </ListingTable.Cell>
          <ListingTable.Cell>
            <ListingTable.SortLabel field="status">Status</ListingTable.SortLabel>
          </ListingTable.Cell>
        </ListingTable.Row>
      </ListingTable.Head>
      <ListingTable.Body>
        {paginatedData.map((row) => (
          <ListingTable.Row key={row.id}>
            <ListingTable.Cell>{row.name}</ListingTable.Cell>
            <ListingTable.Cell>{row.status}</ListingTable.Cell>
          </ListingTable.Row>
        ))}
      </ListingTable.Body>
      <ListingTable.Footer />
    </ListingTable>
  </ListingTable.Container>
</ListingTable.Provider>
```

---

## AppShell

Application layout wrapper using compound component pattern.

```tsx
import { AppShell } from '@wso2/oxygen-ui';
```

### Sub-components

| Component | Description |
|-----------|-------------|
| `AppShell.Navbar` | Top navigation area (header) |
| `AppShell.Sidebar` | Left sidebar area |
| `AppShell.Main` | Main content area |
| `AppShell.Footer` | Bottom footer area |
| `AppShell.NotificationPanel` | Overlay notification panel |

### Usage

```tsx
<AppShell>
  <AppShell.Navbar>
    <Header>...</Header>
  </AppShell.Navbar>

  <AppShell.Sidebar>
    <Sidebar collapsed={collapsed}>...</Sidebar>
  </AppShell.Sidebar>

  <AppShell.Main>
    <Outlet />  {/* React Router outlet */}
  </AppShell.Main>

  <AppShell.Footer>
    <Footer companyName="WSO2 LLC" />
  </AppShell.Footer>

  <AppShell.NotificationPanel>
    <NotificationPanel open={open} onClose={onClose}>
      ...
    </NotificationPanel>
  </AppShell.NotificationPanel>
</AppShell>
```

---

## Header

Top navigation bar compound component.

```tsx
import { Header } from '@wso2/oxygen-ui';
```

### Sub-components

| Component | Description |
|-----------|-------------|
| `Header.Toggle` | Sidebar collapse toggle button |
| `Header.Brand` | Logo and title container |
| `Header.BrandLogo` | Logo image/icon |
| `Header.BrandTitle` | Application title |
| `Header.Switchers` | Context switcher container |
| `Header.Actions` | Right-side action buttons |
| `Header.Spacer` | Flexible spacer |

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `minimal` | `boolean` | `false` | Hide switchers section |
| `sx` | `SxProps` | - | Custom styles |

### Usage

```tsx
<Header>
  <Header.Toggle collapsed={collapsed} onToggle={() => setCollapsed(!collapsed)} />

  <Header.Brand>
    <Header.BrandLogo>
      <img src="/logo.svg" alt="Logo" />
    </Header.BrandLogo>
    <Header.BrandTitle>My Application</Header.BrandTitle>
  </Header.Brand>

  <Header.Switchers>
    <OrganizationSwitcher />
    <ProjectSwitcher />
  </Header.Switchers>

  <Header.Spacer />

  <Header.Actions>
    <ColorSchemeToggle />
    <NotificationButton />
    <UserMenu user={user} />
  </Header.Actions>
</Header>
```

---

## Sidebar

Collapsible navigation sidebar compound component.

```tsx
import { Sidebar, SIDEBAR_WIDTH, COLLAPSED_SIDEBAR_WIDTH } from '@wso2/oxygen-ui';
```

### Sub-components

| Component | Description |
|-----------|-------------|
| `Sidebar.Nav` | Scrollable navigation container |
| `Sidebar.Category` | Menu category group |
| `Sidebar.CategoryLabel` | Category heading text |
| `Sidebar.Item` | Navigation menu item |
| `Sidebar.ItemIcon` | Item icon |
| `Sidebar.ItemLabel` | Item text label |
| `Sidebar.ItemBadge` | Badge indicator |
| `Sidebar.Footer` | Fixed bottom section |
| `Sidebar.User` | User profile section |
| `Sidebar.UserAvatar` | User avatar |
| `Sidebar.UserName` | User name text |
| `Sidebar.UserEmail` | User email text |

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `collapsed` | `boolean` | `false` | Collapsed state |
| `activeItem` | `string` | - | Active menu item ID |
| `expandedMenus` | `Record<string, boolean>` | `{}` | Expanded submenu map |
| `onSelect` | `(id: string) => void` | - | Item selection handler |
| `onToggleExpand` | `(id: string) => void` | - | Submenu toggle handler |
| `width` | `number` | `250` | Expanded width (px) |
| `collapsedWidth` | `number` | `64` | Collapsed width (px) |

### Usage

```tsx
import { HomeIcon, SettingsIcon } from '@wso2/oxygen-ui-icons-react';

<Sidebar
  collapsed={collapsed}
  activeItem={activeId}
  expandedMenus={expandedMenus}
  onSelect={setActiveId}
  onToggleExpand={(id) => toggleExpand(id)}
>
  <Sidebar.Nav>
    <Sidebar.Category>
      <Sidebar.CategoryLabel>Main</Sidebar.CategoryLabel>

      <Sidebar.Item id="home">
        <Sidebar.ItemIcon><HomeIcon size={20} /></Sidebar.ItemIcon>
        <Sidebar.ItemLabel>Home</Sidebar.ItemLabel>
      </Sidebar.Item>

      <Sidebar.Item id="dashboard">
        <Sidebar.ItemIcon><DashboardIcon size={20} /></Sidebar.ItemIcon>
        <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
        <Sidebar.ItemBadge>5</Sidebar.ItemBadge>
      </Sidebar.Item>
    </Sidebar.Category>
  </Sidebar.Nav>

  <Sidebar.Footer>
    <Sidebar.Item id="settings">
      <Sidebar.ItemIcon><SettingsIcon size={20} /></Sidebar.ItemIcon>
      <Sidebar.ItemLabel>Settings</Sidebar.ItemLabel>
    </Sidebar.Item>
  </Sidebar.Footer>
</Sidebar>
```

---

## Form

Form components namespace with enhanced inputs.

```tsx
import { Form } from '@wso2/oxygen-ui';
```

### Components

| Component | Description |
|-----------|-------------|
| `Form.TextInput` | Enhanced text field |
| `Form.SelectInput` | Enhanced select dropdown |
| `Form.MenuItem` | Select menu item |
| `Form.AutocompleteInput` | Autocomplete input |
| `Form.CardButton` | Clickable card |
| `Form.Section` | Form section with heading |
| `Form.Header` | Section header text |
| `Form.Subheader` | Section subheader text |
| `Form.Body` | Section body text |
| `Form.Stack` | Form field stack layout |
| `Form.Wizard` | Multi-step form wizard |

### Wizard Props

```tsx
interface WizardStep {
  id: string;
  label: string;
  content: ReactNode;
  optional?: boolean;
}

interface WizardProps {
  steps: WizardStep[];
  activeStep: number;
  onStepChange: (step: number) => void;
  onComplete?: () => void;
}
```

### Usage

```tsx
<Form.Section>
  <Form.Header>User Details</Form.Header>
  <Form.Subheader>Enter your information</Form.Subheader>

  <Form.Stack spacing={2}>
    <Form.TextInput
      label="Name"
      value={name}
      onChange={(e) => setName(e.target.value)}
    />

    <Form.SelectInput
      label="Role"
      value={role}
      onChange={(e) => setRole(e.target.value)}
    >
      <Form.MenuItem value="admin">Admin</Form.MenuItem>
      <Form.MenuItem value="user">User</Form.MenuItem>
    </Form.SelectInput>
  </Form.Stack>
</Form.Section>
```

---

## NotificationPanel

Slide-out notification panel with tabs.

```tsx
import { NotificationPanel } from '@wso2/oxygen-ui';
```

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `open` | `boolean` | required | Panel visibility |
| `onClose` | `() => void` | required | Close handler |
| `title` | `string` | `'Notifications'` | Panel title |

### Usage

```tsx
<NotificationPanel open={open} onClose={() => setOpen(false)}>
  <NotificationPanel.Header>
    <NotificationPanel.Tabs>
      <NotificationPanel.Tab label="All" />
      <NotificationPanel.Tab label="Unread" badge={3} />
    </NotificationPanel.Tabs>
    <NotificationPanel.Actions>
      <Button onClick={markAllRead}>Mark all read</Button>
    </NotificationPanel.Actions>
  </NotificationPanel.Header>

  <NotificationPanel.List>
    {notifications.map((n) => (
      <NotificationPanel.Item
        key={n.id}
        title={n.title}
        message={n.message}
        time={n.timestamp}
        read={n.read}
      />
    ))}
  </NotificationPanel.List>

  <NotificationPanel.EmptyState message="No notifications" />
</NotificationPanel>
```

---

## Other Components

### ColorSchemeToggle

Dark/light mode toggle button.

```tsx
import { ColorSchemeToggle } from '@wso2/oxygen-ui';

<ColorSchemeToggle />
```

### ThemeSwitcher

Theme selection UI component.

```tsx
import { ThemeSwitcher } from '@wso2/oxygen-ui';

<ThemeSwitcher />  // Requires OxygenUIThemeProvider with themes
```

### Footer

Application footer component.

```tsx
import { Footer } from '@wso2/oxygen-ui';

<Footer
  companyName="WSO2 LLC"
  year={2025}
  links={[
    { label: 'Privacy', href: '/privacy' },
    { label: 'Terms', href: '/terms' },
  ]}
/>
```

### UserMenu

User profile dropdown menu.

```tsx
import { UserMenu } from '@wso2/oxygen-ui';

<UserMenu
  user={{ name: 'John Doe', email: 'john@example.com', avatar: '/avatar.png' }}
  onSignOut={() => signOut()}
/>
```

### PageTitle

Page header with avatar and breadcrumb support.

```tsx
import { PageTitle } from '@wso2/oxygen-ui';

<PageTitle>
  <PageTitle.Avatar><UserIcon /></PageTitle.Avatar>
  <PageTitle.Header>Dashboard</PageTitle.Header>
  <PageTitle.SubHeader>Overview of your projects</PageTitle.SubHeader>
</PageTitle>
```

### CodeBlock

Syntax-highlighted code display.

```tsx
import { CodeBlock } from '@wso2/oxygen-ui';

<CodeBlock language="typescript">
  {`const greeting = "Hello, World!";`}
</CodeBlock>
```

---

## MUI X Namespaces

MUI X components are exported as namespaces to avoid naming conflicts.

```tsx
import { DataGrid, Charts, DatePickers, TreeView, AdapterDateFns } from '@wso2/oxygen-ui';
```

### DataGrid

```tsx
<DataGrid.DataGrid
  rows={rows}
  columns={columns}
  pageSize={10}
  checkboxSelection
/>

<DataGrid.DataGridPro
  rows={rows}
  columns={columns}
  treeData
/>
```

### Charts

```tsx
<Charts.LineChart
  series={[{ data: [1, 2, 3, 4], label: 'Sales' }]}
  xAxis={[{ data: ['Jan', 'Feb', 'Mar', 'Apr'], scaleType: 'band' }]}
/>

<Charts.BarChart series={series} />
<Charts.PieChart series={series} />
<Charts.ScatterChart series={series} />
```

### DatePickers

```tsx
<DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
  <DatePickers.DatePicker
    value={date}
    onChange={setDate}
    label="Select date"
  />

  <DatePickers.DateTimePicker
    value={dateTime}
    onChange={setDateTime}
  />

  <DatePickers.DateRangePicker
    value={range}
    onChange={setRange}
  />
</DatePickers.LocalizationProvider>
```

### TreeView

```tsx
<TreeView.SimpleTreeView>
  <TreeView.TreeItem itemId="1" label="Parent">
    <TreeView.TreeItem itemId="2" label="Child 1" />
    <TreeView.TreeItem itemId="3" label="Child 2" />
  </TreeView.TreeItem>
</TreeView.SimpleTreeView>

<TreeView.RichTreeView
  items={treeData}
  multiSelect
/>
```

---

## Hooks

### useThemeSwitcher

Access theme switching context.

```tsx
import { useThemeSwitcher } from '@wso2/oxygen-ui';

function ThemeSelector() {
  const { currentTheme, themes, setTheme, isActive } = useThemeSwitcher();

  return (
    <Select value={currentTheme} onChange={(e) => setTheme(e.target.value)}>
      {themes.map((t) => (
        <MenuItem key={t.key} value={t.key}>{t.label}</MenuItem>
      ))}
    </Select>
  );
}
```

### useListingTable

Access ListingTable context (returns null outside Provider).

```tsx
import { useListingTable } from '@wso2/oxygen-ui';

function CustomFilter() {
  const context = useListingTable();
  if (!context) return null;

  return (
    <TextField
      value={context.searchValue}
      onChange={(e) => context.onSearchChange?.(e.target.value)}
    />
  );
}
```

### useListingTableRequired

Access ListingTable context (throws if outside Provider).

```tsx
import { useListingTableRequired } from '@wso2/oxygen-ui';

function TableControls() {
  const { density, onDensityChange } = useListingTableRequired();
  // ...
}
```

---

## Type Exports

Import types for TypeScript usage:

```tsx
import type {
  // Theme
  ThemeOption,
  ThemeSwitcherContextValue,

  // ListingTable
  ListingTableProps,
  ListingTableProviderProps,
  ListingTableContextValue,
  ListingTableToolbarProps,
  ListingTableDensity,
  ListingTableVariant,
  ListingTableSortDirection,

  // Layout components
  AppShellProps,
  HeaderProps,
  SidebarProps,

  // Form
  WizardProps,
  WizardStep,
} from '@wso2/oxygen-ui';
```
