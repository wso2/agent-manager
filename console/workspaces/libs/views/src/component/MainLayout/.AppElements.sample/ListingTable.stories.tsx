/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { Meta, StoryObj } from '@storybook/react';
import {
  ListingTable,
  Button,
  Checkbox,
  Chip,
  IconButton,
  Stack,
  TablePagination,
  Tooltip,
  Box,
  Typography,
  Autocomplete,
  TextField,
  useListingTable,
} from '@wso2/oxygen-ui';
import type { ListingTableDensity, ListingTableSortDirection, ListingTableVariant } from '@wso2/oxygen-ui';
import {
  Edit,
  Trash2,
  MoreVertical,
  Plus,
  CheckCircle,
  XCircle,
  Inbox,
  Server,
  Globe,
  Webhook,
  Shield,
  AlertCircle,
  Loader,
  ExternalLink,
} from '@wso2/oxygen-ui-icons-react';
import React, { useState, useMemo } from 'react';

// ============================================================================
// Types and Sample Data
// ============================================================================

type ComponentType = 'Service' | 'WebApp' | 'Webhook' | 'Proxy';
type ComponentStatus = 'Running' | 'Stopped' | 'Deploying' | 'Error';

interface PlatformComponent {
  id: string;
  name: string;
  type: ComponentType;
  description: string;
  status: ComponentStatus;
  environment: string;
  version: string;
  lastDeployed: string;
  createdBy: string;
}

const platformComponents: PlatformComponent[] = [
  {
    id: 'svc-001',
    name: 'user-service',
    type: 'Service',
    description: 'User authentication and management service',
    status: 'Running',
    environment: 'Production',
    version: 'v2.1.0',
    lastDeployed: '2025-12-15',
    createdBy: 'John Doe',
  },
  {
    id: 'app-001',
    name: 'dashboard-app',
    type: 'WebApp',
    description: 'Main dashboard web application',
    status: 'Running',
    environment: 'Production',
    version: 'v1.5.2',
    lastDeployed: '2025-12-14',
    createdBy: 'Jane Smith',
  },
  {
    id: 'whk-001',
    name: 'payment-webhook',
    type: 'Webhook',
    description: 'Payment gateway event handler',
    status: 'Deploying',
    environment: 'Staging',
    version: 'v1.0.0',
    lastDeployed: '2025-12-16',
    createdBy: 'Bob Johnson',
  },
  {
    id: 'prx-001',
    name: 'api-gateway',
    type: 'Proxy',
    description: 'API gateway and rate limiter',
    status: 'Running',
    environment: 'Production',
    version: 'v3.0.1',
    lastDeployed: '2025-12-10',
    createdBy: 'Alice Brown',
  },
  {
    id: 'svc-002',
    name: 'order-service',
    type: 'Service',
    description: 'Order processing and management',
    status: 'Running',
    environment: 'Production',
    version: 'v1.8.3',
    lastDeployed: '2025-12-12',
    createdBy: 'Charlie Wilson',
  },
  {
    id: 'app-002',
    name: 'admin-portal',
    type: 'WebApp',
    description: 'Administrative portal for system management',
    status: 'Stopped',
    environment: 'Development',
    version: 'v0.9.0',
    lastDeployed: '2025-12-08',
    createdBy: 'Diana Martinez',
  },
  {
    id: 'whk-002',
    name: 'notification-webhook',
    type: 'Webhook',
    description: 'Email and SMS notification handler',
    status: 'Running',
    environment: 'Production',
    version: 'v2.0.0',
    lastDeployed: '2025-12-11',
    createdBy: 'Edward Lee',
  },
  {
    id: 'prx-002',
    name: 'auth-proxy',
    type: 'Proxy',
    description: 'Authentication proxy for external services',
    status: 'Error',
    environment: 'Staging',
    version: 'v1.2.0',
    lastDeployed: '2025-12-13',
    createdBy: 'Fiona Garcia',
  },
  {
    id: 'svc-003',
    name: 'inventory-service',
    type: 'Service',
    description: 'Inventory tracking and management',
    status: 'Running',
    environment: 'Production',
    version: 'v1.4.0',
    lastDeployed: '2025-12-09',
    createdBy: 'George Taylor',
  },
  {
    id: 'app-003',
    name: 'customer-portal',
    type: 'WebApp',
    description: 'Self-service customer portal',
    status: 'Deploying',
    environment: 'Staging',
    version: 'v2.0.0-beta',
    lastDeployed: '2025-12-16',
    createdBy: 'Hannah Anderson',
  },
];

// ============================================================================
// Helper Functions
// ============================================================================

const getComponentIcon = (type: ComponentType) => {
  switch (type) {
    case 'Service':
      return <Server size={20} />;
    case 'WebApp':
      return <Globe size={20} />;
    case 'Webhook':
      return <Webhook size={20} />;
    case 'Proxy':
      return <Shield size={20} />;
    default:
      return <Server size={20} />;
  }
};

const getStatusColor = (status: ComponentStatus): 'success' | 'error' | 'warning' | 'info' => {
  switch (status) {
    case 'Running':
      return 'success';
    case 'Stopped':
      return 'error';
    case 'Deploying':
      return 'warning';
    case 'Error':
      return 'error';
    default:
      return 'info';
  }
};

const getStatusIcon = (status: ComponentStatus) => {
  switch (status) {
    case 'Running':
      return <CheckCircle size={14} />;
    case 'Stopped':
      return <XCircle size={14} />;
    case 'Deploying':
      return <Loader size={14} />;
    case 'Error':
      return <AlertCircle size={14} />;
    default:
      return null;
  }
};

// ============================================================================
// Story Args Interface
// ============================================================================

interface ListingTableStoryArgs {
  variant?: ListingTableVariant;
  density?: ListingTableDensity;
  striped?: boolean;
  bordered?: boolean;
}

// ============================================================================
// Meta Configuration
// ============================================================================

/**
 * ListingTable provides an enhanced table component using the compound component pattern.
 *
 * **Key Features:**
 * - **Two display variants**: `table` (traditional) and `card` (card-like rows with gaps)
 * - **Provider pattern**: Centralized state management with auto-binding sub-components
 * - **Search extensibility**: Built-in search, custom searchSlot, and useListingTable hook
 * - **Density control**: compact, standard, comfortable padding options
 * - **Sub-components**: Toolbar, SortLabel, CellIcon, RowActions, EmptyState, DensityControl
 *
 * **Quick Start:**
 * ```tsx
 * import { ListingTable } from '@wso2/oxygen-ui';
 *
 * <ListingTable.Container>
 *   <ListingTable.Toolbar showSearch searchPlaceholder="Search..." />
 *   <ListingTable>
 *     <ListingTable.Head>...</ListingTable.Head>
 *     <ListingTable.Body>...</ListingTable.Body>
 *   </ListingTable>
 * </ListingTable.Container>
 * ```
 *
 * **With Provider (recommended for complex tables):**
 * ```tsx
 * <ListingTable.Provider
 *   searchValue={search}
 *   onSearchChange={setSearch}
 *   sortField={sortField}
 *   onSortChange={handleSort}
 * >
 *   <ListingTable.Container>
 *     <ListingTable.Toolbar showSearch />
 *     <ListingTable>...</ListingTable>
 *   </ListingTable.Container>
 * </ListingTable.Provider>
 * ```
 */
const meta: Meta<typeof ListingTable> = {
  title: 'Tables/ListingTable',
  component: ListingTable,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'radio',
      options: ['table', 'card'],
      description: 'Display variant - table (traditional) or card (card-like rows with rounded corners and gaps)',
      table: {
        type: { summary: "'table' | 'card'" },
        defaultValue: { summary: 'table' },
      },
    },
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
      description: 'Row density affects cell padding',
      table: {
        type: { summary: "'compact' | 'standard' | 'comfortable'" },
        defaultValue: { summary: 'standard' },
      },
    },
    striped: {
      control: 'boolean',
      description: 'Enable alternating row backgrounds',
      table: {
        defaultValue: { summary: 'false' },
      },
    },
    bordered: {
      control: 'boolean',
      description: 'Enable cell borders',
      table: {
        defaultValue: { summary: 'false' },
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ListingTable & ListingTableStoryArgs>;

// ============================================================================
// HERO STORIES
// ============================================================================

/**
 * **Get started in 2 minutes!** This minimal example shows the essential ListingTable pattern.
 *
 * Features included:
 * - Basic compound component structure
 * - Toolbar with built-in search
 * - CellIcon for rich cell content
 * - Row hover effects
 *
 * This example works without the Provider pattern - perfect for simple use cases.
 */
export const QuickStart: Story = {
  render: () => {
    const [searchValue, setSearchValue] = useState('');

    const filteredComponents = useMemo(() => {
      if (!searchValue) return platformComponents.slice(0, 5);
      const lower = searchValue.toLowerCase();
      return platformComponents
        .filter(
          (c) =>
            c.name.toLowerCase().includes(lower) ||
            c.description.toLowerCase().includes(lower) ||
            c.type.toLowerCase().includes(lower),
        )
        .slice(0, 5);
    }, [searchValue]);

    return (
      <ListingTable.Container sx={{ minWidth: 700 }}>
        <ListingTable.Toolbar
          showSearch
          searchValue={searchValue}
          onSearchChange={setSearchValue}
          searchPlaceholder="Search components..."
          actions={
            <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
              New Component
            </Button>
          }
        />
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Type</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Environment</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {filteredComponents.map((component) => (
              <ListingTable.Row key={component.id} hover>
                <ListingTable.Cell>
                  <ListingTable.CellIcon
                    icon={getComponentIcon(component.type)}
                    primary={component.name}
                    secondary={component.description}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip label={component.type} size="small" variant="outlined" />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip
                    icon={getStatusIcon(component.status) || undefined}
                    label={component.status}
                    size="small"
                    color={getStatusColor(component.status)}
                    variant="outlined"
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>{component.environment}</ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  },
};

/**
 * **Full-featured production implementation** using the Provider pattern.
 *
 * This example demonstrates:
 * - **ListingTable.Provider** for centralized state management
 * - **Auto-binding**: SortLabel with `field` prop, DensityControl without explicit props
 * - Search, sort, pagination, and selection
 * - Bulk actions with selection counter
 * - Row actions with hover visibility
 *
 * Use this pattern when building complex data tables with multiple interactive features.
 */
export const ProductionExample: Story = {
  args: {
    variant: 'table',
    density: 'standard',
  },
  render: (args) => {
    const [searchValue, setSearchValue] = useState('');
    const [sortField, setSortField] = useState<string>('name');
    const [sortDirection, setSortDirection] = useState<ListingTableSortDirection>('asc');
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(5);
    const [selected, setSelected] = useState<readonly string[]>([]);
    const [density, setDensity] = useState<ListingTableDensity>(args.density || 'standard');

    const handleSortChange = (field: string, direction: ListingTableSortDirection) => {
      setSortField(field);
      setSortDirection(direction);
    };

    const isSelected = (id: string) => selected.includes(id);

    const handleSelectAll = () => {
      if (selected.length === platformComponents.length) {
        setSelected([]);
      } else {
        setSelected(platformComponents.map((c) => c.id));
      }
    };

    const handleRowSelect = (id: string) => {
      const selectedIndex = selected.indexOf(id);
      let newSelected: readonly string[] = [];

      if (selectedIndex === -1) {
        newSelected = [...selected, id];
      } else {
        newSelected = selected.filter((s) => s !== id);
      }

      setSelected(newSelected);
    };

    const handleBulkDelete = (ids: readonly string[]) => {
      console.log('Bulk delete:', ids);
      setSelected([]);
    };

    const processedComponents = useMemo(() => {
      let result = [...platformComponents];

      if (searchValue) {
        const lower = searchValue.toLowerCase();
        result = result.filter(
          (c) =>
            c.name.toLowerCase().includes(lower) ||
            c.description.toLowerCase().includes(lower) ||
            c.type.toLowerCase().includes(lower),
        );
      }

      result.sort((a, b) => {
        const aVal = a[sortField as keyof PlatformComponent];
        const bVal = b[sortField as keyof PlatformComponent];
        const comparison = String(aVal).localeCompare(String(bVal));
        return sortDirection === 'asc' ? comparison : -comparison;
      });

      return result;
    }, [searchValue, sortField, sortDirection]);

    const paginatedComponents = processedComponents.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

    return (
      <ListingTable.Provider
        searchValue={searchValue}
        onSearchChange={(value) => {
          setSearchValue(value);
          setPage(0);
        }}
        sortField={sortField}
        sortDirection={sortDirection}
        onSortChange={handleSortChange}
        page={page}
        rowsPerPage={rowsPerPage}
        totalCount={processedComponents.length}
        onPageChange={setPage}
        onRowsPerPageChange={(rpp) => {
          setRowsPerPage(rpp);
          setPage(0);
        }}
        selected={selected}
        onSelectionChange={setSelected}
        onSelectAll={handleSelectAll}
        onClearSelection={() => setSelected([])}
        isSelected={isSelected}
        density={density}
        onDensityChange={setDensity}
        onBulkDelete={handleBulkDelete}
        variant={args.variant}
      >
        <ListingTable.Container sx={{ minWidth: 900 }} disablePaper={args.variant === 'card'}>
          <ListingTable.Toolbar
            showSearch
            searchPlaceholder="Search components..."
            actions={
              <>
                {selected.length > 0 && (
                  <Button
                    variant="outlined"
                    color="error"
                    size="small"
                    startIcon={<Trash2 size={16} />}
                    onClick={() => handleBulkDelete(selected)}
                  >
                    Delete ({selected.length})
                  </Button>
                )}
                <ListingTable.DensityControl />
                <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
                  New Component
                </Button>
              </>
            }
          />
          <ListingTable density={density} variant={args.variant}>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell padding="checkbox">
                  <Checkbox
                    color="primary"
                    indeterminate={selected.length > 0 && selected.length < platformComponents.length}
                    checked={platformComponents.length > 0 && selected.length === platformComponents.length}
                    onChange={handleSelectAll}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="name">Name</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="type">Type</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="status">Status</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="environment">Environment</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell align="right">Actions</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {paginatedComponents.map((component) => {
                const isItemSelected = isSelected(component.id);

                return (
                  <ListingTable.Row
                    key={component.id}
                    hover
                    selected={isItemSelected}
                    onClick={() => handleRowSelect(component.id)}
                    clickable
                    variant={args.variant}
                  >
                    <ListingTable.Cell padding="checkbox">
                      <Checkbox color="primary" checked={isItemSelected} />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <ListingTable.CellIcon
                        icon={getComponentIcon(component.type)}
                        primary={component.name}
                        secondary={component.description}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip label={component.type} size="small" variant="outlined" />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        icon={getStatusIcon(component.status) || undefined}
                        label={component.status}
                        size="small"
                        color={getStatusColor(component.status)}
                        variant="outlined"
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                    <ListingTable.Cell align="right">
                      <ListingTable.RowActions visibility="hover">
                        <Tooltip title="Edit">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              console.log('Edit:', component.id);
                            }}
                          >
                            <Edit size={16} />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Delete">
                          <IconButton
                            size="small"
                            color="error"
                            onClick={(e) => {
                              e.stopPropagation();
                              console.log('Delete:', component.id);
                            }}
                          >
                            <Trash2 size={16} />
                          </IconButton>
                        </Tooltip>
                      </ListingTable.RowActions>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                );
              })}
            </ListingTable.Body>
          </ListingTable>
          <TablePagination
            rowsPerPageOptions={[5, 10, 25]}
            component="div"
            count={processedComponents.length}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_, newPage) => setPage(newPage)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
          />
        </ListingTable.Container>
      </ListingTable.Provider>
    );
  },
};

// ============================================================================
// BASICS SECTION
// ============================================================================

/**
 * The fundamental structure of ListingTable using the compound component pattern.
 *
 * Components used:
 * - `ListingTable.Container` - Wrapper with optional Paper elevation
 * - `ListingTable` - The table element
 * - `ListingTable.Head` / `ListingTable.Body` - Table sections
 * - `ListingTable.Row` / `ListingTable.Cell` - Row and cell elements
 */
export const BaseStructure: Story = {
  name: 'Base Structure',
  parameters: {
    docs: {
      story: {
        height: '300px',
      },
    },
  },
  render: () => (
    <ListingTable.Container sx={{ minWidth: 650 }}>
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell>Type</ListingTable.Cell>
            <ListingTable.Cell>Status</ListingTable.Cell>
            <ListingTable.Cell>Environment</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {platformComponents.slice(0, 5).map((component) => (
            <ListingTable.Row key={component.id}>
              <ListingTable.Cell>{component.name}</ListingTable.Cell>
              <ListingTable.Cell>{component.type}</ListingTable.Cell>
              <ListingTable.Cell>{component.status}</ListingTable.Cell>
              <ListingTable.Cell>{component.environment}</ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
    </ListingTable.Container>
  ),
};

/**
 * Control row density with three options: compact, standard, and comfortable.
 * Use the controls panel to toggle between density levels.
 */
export const Density: Story = {
  args: {
    density: 'standard',
  },
  argTypes: {
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
    },
  },
  render: (args) => (
    <ListingTable.Container sx={{ minWidth: 600 }}>
      <ListingTable density={args.density}>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell>Type</ListingTable.Cell>
            <ListingTable.Cell>Status</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {platformComponents.slice(0, 4).map((component) => (
            <ListingTable.Row key={component.id}>
              <ListingTable.Cell>{component.name}</ListingTable.Cell>
              <ListingTable.Cell>{component.type}</ListingTable.Cell>
              <ListingTable.Cell>{component.status}</ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
    </ListingTable.Container>
  ),
};

/**
 * Display a meaningful empty state when there's no data.
 *
 * The EmptyState component supports:
 * - Custom illustration
 * - Title and description
 * - Call-to-action button
 */
export const EmptyState: Story = {
  render: () => (
    <ListingTable.Container sx={{ minWidth: 700 }}>
      <ListingTable.EmptyState
        illustration={<Inbox size={64} />}
        title="No components yet"
        description="Get started by deploying your first component. Components can be services, web apps, webhooks, or proxies."
        action={
          <Button variant="contained" startIcon={<Plus size={16} />}>
            Create Component
          </Button>
        }
      />
    </ListingTable.Container>
  ),
};

// ============================================================================
// VARIANTS SECTION
// ============================================================================

/**
 * The default table variant with traditional table appearance.
 * Use controls to toggle striped rows and bordered cells.
 */
export const TableVariant: Story = {
  name: 'Table Variant',
  args: {
    density: 'standard',
    striped: false,
    bordered: false,
  },
  argTypes: {
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
    },
    striped: {
      control: 'boolean',
    },
    bordered: {
      control: 'boolean',
    },
  },
  render: (args) => (
    <ListingTable.Container sx={{ minWidth: 700 }}>
      <ListingTable variant="table" density={args.density} striped={args.striped} bordered={args.bordered}>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell>Type</ListingTable.Cell>
            <ListingTable.Cell>Status</ListingTable.Cell>
            <ListingTable.Cell>Environment</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {platformComponents.slice(0, 5).map((component) => (
            <ListingTable.Row key={component.id} hover>
              <ListingTable.Cell>
                <ListingTable.CellIcon icon={getComponentIcon(component.type)} primary={component.name} />
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Chip label={component.type} size="small" variant="outlined" />
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Chip
                  icon={getStatusIcon(component.status) || undefined}
                  label={component.status}
                  size="small"
                  color={getStatusColor(component.status)}
                  variant="outlined"
                />
              </ListingTable.Cell>
              <ListingTable.Cell>{component.environment}</ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
    </ListingTable.Container>
  ),
};

/**
 * The card variant displays each row as a card with:
 * - Rounded corners
 * - Subtle borders
 * - 12px gaps between rows
 * - Hover and click effects
 *
 * Ideal for displaying items in a more visual, card-based layout.
 */
export const CardVariant: Story = {
  name: 'Card Variant',
  args: {
    density: 'standard',
  },
  argTypes: {
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
    },
  },
  render: (args) => (
    <ListingTable.Container sx={{ minWidth: 700 }} disablePaper>
      <ListingTable variant="card" density={args.density}>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell>Description</ListingTable.Cell>
            <ListingTable.Cell>Type</ListingTable.Cell>
            <ListingTable.Cell>Status</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {platformComponents.slice(0, 5).map((component) => (
            <ListingTable.Row key={component.id} variant="card" hover clickable>
              <ListingTable.Cell>
                <ListingTable.CellIcon icon={getComponentIcon(component.type)} primary={component.name} />
              </ListingTable.Cell>
              <ListingTable.Cell>{component.description}</ListingTable.Cell>
              <ListingTable.Cell>
                <Chip label={component.type} size="small" variant="outlined" />
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Chip
                  icon={getStatusIcon(component.status) || undefined}
                  label={component.status}
                  size="small"
                  color={getStatusColor(component.status)}
                  variant="outlined"
                />
              </ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
    </ListingTable.Container>
  ),
};

/**
 * Side-by-side comparison of table and card variants.
 */
export const VariantComparison: Story = {
  name: 'Variant Comparison',
  render: () => (
    <Stack spacing={4}>
      <div>
        <Typography variant="h6" sx={{ mb: 2 }}>
          Table Variant (Default)
        </Typography>
        <ListingTable.Container sx={{ minWidth: 600 }}>
          <ListingTable variant="table">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Type</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {platformComponents.slice(0, 3).map((component) => (
                <ListingTable.Row key={component.id} hover>
                  <ListingTable.Cell>
                    <ListingTable.CellIcon icon={getComponentIcon(component.type)} primary={component.name} />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip label={component.type} size="small" variant="outlined" />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      label={component.status}
                      size="small"
                      color={getStatusColor(component.status)}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </div>

      <div>
        <Typography variant="h6" sx={{ mb: 2 }}>
          Card Variant
        </Typography>
        <ListingTable.Container sx={{ minWidth: 600 }} disablePaper>
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Type</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {platformComponents.slice(0, 3).map((component) => (
                <ListingTable.Row key={component.id} variant="card" hover clickable>
                  <ListingTable.Cell>
                    <ListingTable.CellIcon icon={getComponentIcon(component.type)} primary={component.name} />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip label={component.type} size="small" variant="outlined" />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      label={component.status}
                      size="small"
                      color={getStatusColor(component.status)}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </div>
    </Stack>
  ),
};

// ============================================================================
// FEATURES SECTION
// ============================================================================

/**
 * The Toolbar component provides search, actions, and other controls.
 *
 * Features:
 * - Built-in search with `showSearch` prop
 * - Custom actions on the right side
 * - DensityControl integration
 */
export const Toolbar: Story = {
  render: () => {
    const [density, setDensity] = useState<ListingTableDensity>('standard');

    return (
      <ListingTable.Container sx={{ minWidth: 700 }}>
        <ListingTable.Toolbar
          showSearch
          searchPlaceholder="Search components..."
          actions={
            <>
              <ListingTable.DensityControl value={density} onChange={setDensity} />
              <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
                New Component
              </Button>
            </>
          }
        />
        <ListingTable density={density}>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Type</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Environment</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {platformComponents.slice(0, 5).map((component) => (
              <ListingTable.Row key={component.id} hover>
                <ListingTable.Cell>
                  <ListingTable.CellIcon
                    icon={getComponentIcon(component.type)}
                    primary={component.name}
                    secondary={component.description}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip label={component.type} size="small" variant="outlined" />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip
                    icon={getStatusIcon(component.status) || undefined}
                    label={component.status}
                    size="small"
                    color={getStatusColor(component.status)}
                    variant="outlined"
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>{component.environment}</ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  },
};

/**
 * Sortable columns using `ListingTable.SortLabel`.
 *
 * Click column headers to toggle sort direction.
 */
export const Sorting: Story = {
  args: {
    variant: 'table',
    density: 'standard',
  },
  argTypes: {
    variant: {
      control: 'radio',
      options: ['table', 'card'],
    },
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
    },
  },
  render: (args) => {
    const [sortField, setSortField] = useState<keyof PlatformComponent>('name');
    const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');

    const sortedComponents = useMemo(() => {
      return [...platformComponents].sort((a, b) => {
        const aVal = a[sortField];
        const bVal = b[sortField];
        const comparison = String(aVal).localeCompare(String(bVal));
        return sortDirection === 'asc' ? comparison : -comparison;
      });
    }, [sortField, sortDirection]);

    const handleSort = (field: keyof PlatformComponent) => {
      if (sortField === field) {
        setSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'));
      } else {
        setSortField(field);
        setSortDirection('asc');
      }
    };

    return (
      <ListingTable.Container sx={{ minWidth: 700 }} disablePaper={args.variant === 'card'}>
        <ListingTable variant={args.variant} density={args.density}>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>
                <ListingTable.SortLabel
                  active={sortField === 'name'}
                  direction={sortField === 'name' ? sortDirection : 'asc'}
                  onClick={() => handleSort('name')}
                >
                  Name
                </ListingTable.SortLabel>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <ListingTable.SortLabel
                  active={sortField === 'type'}
                  direction={sortField === 'type' ? sortDirection : 'asc'}
                  onClick={() => handleSort('type')}
                >
                  Type
                </ListingTable.SortLabel>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <ListingTable.SortLabel
                  active={sortField === 'status'}
                  direction={sortField === 'status' ? sortDirection : 'asc'}
                  onClick={() => handleSort('status')}
                >
                  Status
                </ListingTable.SortLabel>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <ListingTable.SortLabel
                  active={sortField === 'version'}
                  direction={sortField === 'version' ? sortDirection : 'asc'}
                  onClick={() => handleSort('version')}
                >
                  Version
                </ListingTable.SortLabel>
              </ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {sortedComponents.slice(0, 5).map((component) => (
              <ListingTable.Row key={component.id} hover variant={args.variant}>
                <ListingTable.Cell>
                  <ListingTable.CellIcon icon={getComponentIcon(component.type)} primary={component.name} />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip label={component.type} size="small" variant="outlined" />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip
                    icon={getStatusIcon(component.status) || undefined}
                    label={component.status}
                    size="small"
                    color={getStatusColor(component.status)}
                    variant="outlined"
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2" fontFamily="monospace">
                    {component.version}
                  </Typography>
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  },
};

/**
 * Row selection with checkboxes.
 *
 * Features:
 * - Select all / deselect all
 * - Indeterminate state for partial selection
 * - Selected row highlighting
 */
export const Selection: Story = {
  args: {
    variant: 'table',
    density: 'standard',
  },
  argTypes: {
    variant: {
      control: 'radio',
      options: ['table', 'card'],
    },
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
    },
  },
  render: (args) => {
    const [selected, setSelected] = useState<readonly string[]>([]);

    const handleSelectAllClick = (event: React.ChangeEvent<HTMLInputElement>) => {
      if (event.target.checked) {
        const newSelected = platformComponents.map((c) => c.id);
        setSelected(newSelected);
        return;
      }
      setSelected([]);
    };

    const handleClick = (id: string) => {
      const selectedIndex = selected.indexOf(id);
      let newSelected: readonly string[] = [];

      if (selectedIndex === -1) {
        newSelected = newSelected.concat(selected, id);
      } else if (selectedIndex === 0) {
        newSelected = newSelected.concat(selected.slice(1));
      } else if (selectedIndex === selected.length - 1) {
        newSelected = newSelected.concat(selected.slice(0, -1));
      } else if (selectedIndex > 0) {
        newSelected = newSelected.concat(selected.slice(0, selectedIndex), selected.slice(selectedIndex + 1));
      }

      setSelected(newSelected);
    };

    const isSelected = (id: string) => selected.indexOf(id) !== -1;

    return (
      <ListingTable.Container sx={{ minWidth: 700 }} disablePaper={args.variant === 'card'}>
        <ListingTable variant={args.variant} density={args.density}>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell padding="checkbox">
                <Checkbox
                  color="primary"
                  indeterminate={selected.length > 0 && selected.length < platformComponents.length}
                  checked={platformComponents.length > 0 && selected.length === platformComponents.length}
                  onChange={handleSelectAllClick}
                />
              </ListingTable.Cell>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Type</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Environment</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {platformComponents.map((component) => {
              const isItemSelected = isSelected(component.id);

              return (
                <ListingTable.Row
                  key={component.id}
                  hover
                  onClick={() => handleClick(component.id)}
                  role="checkbox"
                  aria-checked={isItemSelected}
                  selected={isItemSelected}
                  clickable
                  variant={args.variant}
                >
                  <ListingTable.Cell padding="checkbox">
                    <Checkbox color="primary" checked={isItemSelected} />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <ListingTable.CellIcon icon={getComponentIcon(component.type)} primary={component.name} />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip label={component.type} size="small" variant="outlined" />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      icon={getStatusIcon(component.status) || undefined}
                      label={component.status}
                      size="small"
                      color={getStatusColor(component.status)}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                </ListingTable.Row>
              );
            })}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  },
};

/**
 * Pagination using MUI TablePagination.
 */
export const Pagination: Story = {
  args: {
    variant: 'table',
    density: 'standard',
  },
  argTypes: {
    variant: {
      control: 'radio',
      options: ['table', 'card'],
    },
    density: {
      control: 'select',
      options: ['compact', 'standard', 'comfortable'],
    },
  },
  render: (args) => {
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(3);

    const handleChangePage = (_event: unknown, newPage: number) => {
      setPage(newPage);
    };

    const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
      setRowsPerPage(parseInt(event.target.value, 10));
      setPage(0);
    };

    return (
      <ListingTable.Container sx={{ minWidth: 700 }} disablePaper={args.variant === 'card'}>
        <ListingTable variant={args.variant} density={args.density}>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Type</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Environment</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {platformComponents.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage).map((component) => (
              <ListingTable.Row key={component.id} hover variant={args.variant}>
                <ListingTable.Cell>
                  <ListingTable.CellIcon
                    icon={getComponentIcon(component.type)}
                    primary={component.name}
                    secondary={component.description}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip label={component.type} size="small" variant="outlined" />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip
                    icon={getStatusIcon(component.status) || undefined}
                    label={component.status}
                    size="small"
                    color={getStatusColor(component.status)}
                    variant="outlined"
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>{component.environment}</ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
        <TablePagination
          rowsPerPageOptions={[3, 5, 10]}
          component="div"
          count={platformComponents.length}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
        />
      </ListingTable.Container>
    );
  },
};

/**
 * Row actions that appear on hover.
 *
 * Use `ListingTable.RowActions` with `visibility="hover"` to show action buttons only when hovering over a row.
 */
export const RowActions: Story = {
  name: 'Row Actions',
  render: () => {
    const handleAction = (action: string, id: string, e: React.MouseEvent) => {
      e.stopPropagation();
      console.log(`${action}:`, id);
    };

    return (
      <ListingTable.Container sx={{ minWidth: 800 }}>
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Type</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Environment</ListingTable.Cell>
              <ListingTable.Cell align="right">Actions</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {platformComponents.slice(0, 5).map((component) => (
              <ListingTable.Row
                key={component.id}
                hover
                clickable
                onClick={() => console.log('Row clicked:', component.id)}
              >
                <ListingTable.Cell>
                  <ListingTable.CellIcon
                    icon={getComponentIcon(component.type)}
                    primary={component.name}
                    secondary={component.description}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip label={component.type} size="small" variant="outlined" />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip
                    icon={getStatusIcon(component.status) || undefined}
                    label={component.status}
                    size="small"
                    color={getStatusColor(component.status)}
                    variant="outlined"
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                <ListingTable.Cell align="right">
                  <ListingTable.RowActions visibility="hover">
                    <Tooltip title="View Details">
                      <IconButton size="small" onClick={(e) => handleAction('View', component.id, e)}>
                        <ExternalLink size={16} />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Edit">
                      <IconButton size="small" onClick={(e) => handleAction('Edit', component.id, e)}>
                        <Edit size={16} />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete">
                      <IconButton size="small" color="error" onClick={(e) => handleAction('Delete', component.id, e)}>
                        <Trash2 size={16} />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="More Options">
                      <IconButton size="small" onClick={(e) => handleAction('More', component.id, e)}>
                        <MoreVertical size={16} />
                      </IconButton>
                    </Tooltip>
                  </ListingTable.RowActions>
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  },
};

// ============================================================================
// PROVIDER PATTERN SECTION
// ============================================================================

/**
 * The Provider pattern enables centralized state management for ListingTable.
 *
 * **Benefits:**
 * - Sub-components auto-connect to context (no prop drilling)
 * - `SortLabel` with `field` prop auto-handles sort state
 * - `DensityControl` auto-binds without explicit props
 * - `Toolbar` search auto-syncs with context
 *
 * **When to use:**
 * - Tables with search + sort + pagination
 * - Need to share state between components
 * - Building reusable table patterns
 *
 * This example shows `SortLabel` auto-connecting to the Provider's sort state.
 */
export const ProviderIntroduction: Story = {
  name: 'Introduction',
  render: () => {
    const [sortField, setSortField] = useState<string>('name');
    const [sortDirection, setSortDirection] = useState<ListingTableSortDirection>('asc');

    const handleSortChange = (field: string, direction: ListingTableSortDirection) => {
      setSortField(field);
      setSortDirection(direction);
    };

    const sortedComponents = useMemo(() => {
      return [...platformComponents].sort((a, b) => {
        const aVal = a[sortField as keyof PlatformComponent];
        const bVal = b[sortField as keyof PlatformComponent];
        const comparison = String(aVal).localeCompare(String(bVal));
        return sortDirection === 'asc' ? comparison : -comparison;
      });
    }, [sortField, sortDirection]);

    return (
      <ListingTable.Provider sortField={sortField} sortDirection={sortDirection} onSortChange={handleSortChange}>
        <ListingTable.Container sx={{ minWidth: 700 }}>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>
                  {/* SortLabel with field prop auto-connects to context */}
                  <ListingTable.SortLabel field="name">Name</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="type">Type</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="status">Status</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="environment">Environment</ListingTable.SortLabel>
                </ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {sortedComponents.slice(0, 5).map((component) => (
                <ListingTable.Row key={component.id} hover>
                  <ListingTable.Cell>
                    <ListingTable.CellIcon
                      icon={getComponentIcon(component.type)}
                      primary={component.name}
                      secondary={component.description}
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip label={component.type} size="small" variant="outlined" />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      icon={getStatusIcon(component.status) || undefined}
                      label={component.status}
                      size="small"
                      color={getStatusColor(component.status)}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </ListingTable.Provider>
    );
  },
};

/**
 * Demonstrates context auto-binding for multiple components.
 *
 * Notice how:
 * - `Toolbar` with `showSearch` automatically uses Provider's search state
 * - `SortLabel` with `field` prop auto-connects to sort state
 * - `DensityControl` without props auto-binds to density state
 *
 * No prop drilling required!
 */
export const ContextAutoBinding: Story = {
  name: 'Context Auto-Binding',
  render: () => {
    const [searchValue, setSearchValue] = useState('');
    const [sortField, setSortField] = useState<string>('name');
    const [sortDirection, setSortDirection] = useState<ListingTableSortDirection>('asc');
    const [density, setDensity] = useState<ListingTableDensity>('standard');

    const handleSortChange = (field: string, direction: ListingTableSortDirection) => {
      setSortField(field);
      setSortDirection(direction);
    };

    const filteredComponents = useMemo(() => {
      let result = [...platformComponents];

      if (searchValue) {
        const lower = searchValue.toLowerCase();
        result = result.filter(
          (c) =>
            c.name.toLowerCase().includes(lower) ||
            c.description.toLowerCase().includes(lower) ||
            c.type.toLowerCase().includes(lower),
        );
      }

      result.sort((a, b) => {
        const aVal = a[sortField as keyof PlatformComponent];
        const bVal = b[sortField as keyof PlatformComponent];
        const comparison = String(aVal).localeCompare(String(bVal));
        return sortDirection === 'asc' ? comparison : -comparison;
      });

      return result;
    }, [searchValue, sortField, sortDirection]);

    return (
      <ListingTable.Provider
        searchValue={searchValue}
        onSearchChange={setSearchValue}
        sortField={sortField}
        sortDirection={sortDirection}
        onSortChange={handleSortChange}
        density={density}
        onDensityChange={setDensity}
      >
        <ListingTable.Container sx={{ minWidth: 700 }}>
          {/* Toolbar auto-binds to search state */}
          <ListingTable.Toolbar
            showSearch
            searchPlaceholder="Search components..."
            actions={
              <>
                {/* DensityControl auto-binds to density state - no props needed! */}
                <ListingTable.DensityControl />
                <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
                  New Component
                </Button>
              </>
            }
          />
          <ListingTable density={density}>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>
                  {/* SortLabel auto-binds to sort state via field prop */}
                  <ListingTable.SortLabel field="name">Name</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="type">Type</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="status">Status</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="environment">Environment</ListingTable.SortLabel>
                </ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {filteredComponents.slice(0, 5).map((component) => (
                <ListingTable.Row key={component.id} hover>
                  <ListingTable.Cell>
                    <ListingTable.CellIcon
                      icon={getComponentIcon(component.type)}
                      primary={component.name}
                      secondary={component.description}
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip label={component.type} size="small" variant="outlined" />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      icon={getStatusIcon(component.status) || undefined}
                      label={component.status}
                      size="small"
                      color={getStatusColor(component.status)}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </ListingTable.Provider>
    );
  },
};

/**
 * Complete Provider example with all features: search, sort, pagination, selection, density, and bulk actions.
 *
 * This is the recommended pattern for production tables with complex interactivity.
 */
export const ProviderFullFeatured: Story = {
  name: 'Full Featured',
  render: () => {
    const [searchValue, setSearchValue] = useState('');
    const [sortField, setSortField] = useState<string>('name');
    const [sortDirection, setSortDirection] = useState<ListingTableSortDirection>('asc');
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(5);
    const [selected, setSelected] = useState<readonly string[]>([]);
    const [density, setDensity] = useState<ListingTableDensity>('standard');

    const handleSortChange = (field: string, direction: ListingTableSortDirection) => {
      setSortField(field);
      setSortDirection(direction);
    };

    const isSelected = (id: string) => selected.includes(id);

    const handleSelectAll = () => {
      if (selected.length === platformComponents.length) {
        setSelected([]);
      } else {
        setSelected(platformComponents.map((c) => c.id));
      }
    };

    const handleRowSelect = (id: string) => {
      const selectedIndex = selected.indexOf(id);
      let newSelected: readonly string[] = [];

      if (selectedIndex === -1) {
        newSelected = [...selected, id];
      } else {
        newSelected = selected.filter((s) => s !== id);
      }

      setSelected(newSelected);
    };

    const handleBulkDelete = (ids: readonly string[]) => {
      console.log('Bulk delete:', ids);
      setSelected([]);
    };

    const processedComponents = useMemo(() => {
      let result = [...platformComponents];

      if (searchValue) {
        const lower = searchValue.toLowerCase();
        result = result.filter(
          (c) =>
            c.name.toLowerCase().includes(lower) ||
            c.description.toLowerCase().includes(lower) ||
            c.type.toLowerCase().includes(lower),
        );
      }

      result.sort((a, b) => {
        const aVal = a[sortField as keyof PlatformComponent];
        const bVal = b[sortField as keyof PlatformComponent];
        const comparison = String(aVal).localeCompare(String(bVal));
        return sortDirection === 'asc' ? comparison : -comparison;
      });

      return result;
    }, [searchValue, sortField, sortDirection]);

    const paginatedComponents = processedComponents.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

    return (
      <ListingTable.Provider
        searchValue={searchValue}
        onSearchChange={(value) => {
          setSearchValue(value);
          setPage(0);
        }}
        sortField={sortField}
        sortDirection={sortDirection}
        onSortChange={handleSortChange}
        page={page}
        rowsPerPage={rowsPerPage}
        totalCount={processedComponents.length}
        onPageChange={setPage}
        onRowsPerPageChange={(rpp) => {
          setRowsPerPage(rpp);
          setPage(0);
        }}
        selected={selected}
        onSelectionChange={setSelected}
        onSelectAll={handleSelectAll}
        onClearSelection={() => setSelected([])}
        isSelected={isSelected}
        density={density}
        onDensityChange={setDensity}
        onBulkDelete={handleBulkDelete}
      >
        <ListingTable.Container sx={{ minWidth: 800 }}>
          <ListingTable.Toolbar
            showSearch
            searchPlaceholder="Search components..."
            actions={
              <>
                {selected.length > 0 && (
                  <Button
                    variant="outlined"
                    color="error"
                    size="small"
                    startIcon={<Trash2 size={16} />}
                    onClick={() => handleBulkDelete(selected)}
                  >
                    Delete ({selected.length})
                  </Button>
                )}
                <ListingTable.DensityControl />
                <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
                  New Component
                </Button>
              </>
            }
          />
          <ListingTable density={density}>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell padding="checkbox">
                  <Checkbox
                    color="primary"
                    indeterminate={selected.length > 0 && selected.length < platformComponents.length}
                    checked={platformComponents.length > 0 && selected.length === platformComponents.length}
                    onChange={handleSelectAll}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="name">Name</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="type">Type</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="status">Status</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ListingTable.SortLabel field="environment">Environment</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell align="right">Actions</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {paginatedComponents.map((component) => {
                const isItemSelected = isSelected(component.id);

                return (
                  <ListingTable.Row
                    key={component.id}
                    hover
                    selected={isItemSelected}
                    onClick={() => handleRowSelect(component.id)}
                    clickable
                  >
                    <ListingTable.Cell padding="checkbox">
                      <Checkbox color="primary" checked={isItemSelected} />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <ListingTable.CellIcon
                        icon={getComponentIcon(component.type)}
                        primary={component.name}
                        secondary={component.description}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip label={component.type} size="small" variant="outlined" />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        icon={getStatusIcon(component.status) || undefined}
                        label={component.status}
                        size="small"
                        color={getStatusColor(component.status)}
                        variant="outlined"
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                    <ListingTable.Cell align="right">
                      <ListingTable.RowActions visibility="hover">
                        <Tooltip title="Edit">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              console.log('Edit:', component.id);
                            }}
                          >
                            <Edit size={16} />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Delete">
                          <IconButton
                            size="small"
                            color="error"
                            onClick={(e) => {
                              e.stopPropagation();
                              console.log('Delete:', component.id);
                            }}
                          >
                            <Trash2 size={16} />
                          </IconButton>
                        </Tooltip>
                      </ListingTable.RowActions>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                );
              })}
            </ListingTable.Body>
          </ListingTable>
          <TablePagination
            rowsPerPageOptions={[5, 10, 25]}
            component="div"
            count={processedComponents.length}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_, newPage) => setPage(newPage)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
          />
        </ListingTable.Container>
      </ListingTable.Provider>
    );
  },
};

// ============================================================================
// SEARCH EXTENSIBILITY SECTION
// ============================================================================

/**
 * Built-in search using the `showSearch` prop on Toolbar.
 *
 * Two ways to use:
 * 1. **Direct props**: Pass `searchValue` and `onSearchChange` to Toolbar
 * 2. **With Provider**: Just use `showSearch`, context handles the rest
 */
export const BuiltInSearch: Story = {
  name: 'Built-in Search',
  render: () => {
    const [searchValue, setSearchValue] = useState('');

    const filteredComponents = useMemo(() => {
      if (!searchValue) return platformComponents;
      const lower = searchValue.toLowerCase();
      return platformComponents.filter(
        (c) =>
          c.name.toLowerCase().includes(lower) ||
          c.description.toLowerCase().includes(lower) ||
          c.type.toLowerCase().includes(lower),
      );
    }, [searchValue]);

    return (
      <ListingTable.Container sx={{ minWidth: 700 }}>
        <ListingTable.Toolbar
          showSearch
          searchValue={searchValue}
          onSearchChange={setSearchValue}
          searchPlaceholder="Search by name, description, or type..."
        />
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Type</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Environment</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {filteredComponents.length > 0 ? (
              filteredComponents.map((component) => (
                <ListingTable.Row key={component.id} hover>
                  <ListingTable.Cell>
                    <ListingTable.CellIcon
                      icon={getComponentIcon(component.type)}
                      primary={component.name}
                      secondary={component.description}
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip label={component.type} size="small" variant="outlined" />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      icon={getStatusIcon(component.status) || undefined}
                      label={component.status}
                      size="small"
                      color={getStatusColor(component.status)}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                </ListingTable.Row>
              ))
            ) : (
              <ListingTable.Row>
                <ListingTable.Cell colSpan={4}>
                  <Box sx={{ textAlign: 'center', py: 4 }}>
                    <Typography color="text.secondary">No components match your search criteria.</Typography>
                  </Box>
                </ListingTable.Cell>
              </ListingTable.Row>
            )}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    );
  },
};

/**
 * Custom search component that connects to the table context via `useListingTable` hook.
 */
const CustomSearchAutocomplete = () => {
  const tableContext = useListingTable();

  const componentNames = platformComponents.map((c) => c.name);

  return (
    <Autocomplete
      freeSolo
      options={componentNames}
      inputValue={tableContext?.searchValue ?? ''}
      onInputChange={(_, value) => tableContext?.onSearchChange?.(value)}
      renderInput={(params) => (
        <TextField {...params} size="small" placeholder="Search or select component..." sx={{ minWidth: 280 }} />
      )}
      sx={{ minWidth: 280 }}
    />
  );
};

/**
 * Replace the default search with a custom component using `searchSlot`.
 *
 * This example uses an Autocomplete that:
 * - Connects to Provider via `useListingTable()` hook
 * - Provides dropdown suggestions for component names
 * - Supports free-form text input
 */
export const CustomSearchComponent: Story = {
  name: 'Custom Search Component',
  render: () => {
    const [searchValue, setSearchValue] = useState('');

    const filteredComponents = useMemo(() => {
      if (!searchValue) return platformComponents;
      const lower = searchValue.toLowerCase();
      return platformComponents.filter(
        (c) =>
          c.name.toLowerCase().includes(lower) ||
          c.description.toLowerCase().includes(lower) ||
          c.type.toLowerCase().includes(lower),
      );
    }, [searchValue]);

    return (
      <ListingTable.Provider searchValue={searchValue} onSearchChange={setSearchValue}>
        <ListingTable.Container sx={{ minWidth: 700 }}>
          <ListingTable.Toolbar
            searchSlot={<CustomSearchAutocomplete />}
            actions={
              <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
                New Component
              </Button>
            }
          />
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Type</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
                <ListingTable.Cell>Environment</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {filteredComponents.length > 0 ? (
                filteredComponents.slice(0, 5).map((component) => (
                  <ListingTable.Row key={component.id} hover>
                    <ListingTable.Cell>
                      <ListingTable.CellIcon
                        icon={getComponentIcon(component.type)}
                        primary={component.name}
                        secondary={component.description}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip label={component.type} size="small" variant="outlined" />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        icon={getStatusIcon(component.status) || undefined}
                        label={component.status}
                        size="small"
                        color={getStatusColor(component.status)}
                        variant="outlined"
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                  </ListingTable.Row>
                ))
              ) : (
                <ListingTable.Row>
                  <ListingTable.Cell colSpan={4}>
                    <Box sx={{ textAlign: 'center', py: 4 }}>
                      <Typography color="text.secondary">No components match your search criteria.</Typography>
                    </Box>
                  </ListingTable.Cell>
                </ListingTable.Row>
              )}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </ListingTable.Provider>
    );
  },
};

/**
 * Custom filter component that uses the `useListingTable()` hook.
 */
const TypeFilterChips = () => {
  const tableContext = useListingTable();
  const currentFilter = tableContext?.searchValue ?? '';

  const types: ComponentType[] = ['Service', 'WebApp', 'Webhook', 'Proxy'];

  const handleFilterClick = (type: ComponentType) => {
    if (currentFilter === type) {
      tableContext?.onSearchChange?.('');
    } else {
      tableContext?.onSearchChange?.(type);
    }
  };

  return (
    <Stack direction="row" spacing={1}>
      {types.map((type) => (
        <Chip
          key={type}
          label={type}
          size="small"
          variant={currentFilter === type ? 'filled' : 'outlined'}
          color={currentFilter === type ? 'primary' : 'default'}
          onClick={() => handleFilterClick(type)}
          icon={getComponentIcon(type)}
        />
      ))}
    </Stack>
  );
};

/**
 * Build custom components that integrate with ListingTable context using the `useListingTable()` hook.
 *
 * **Hook returns:**
 * - `searchValue`, `onSearchChange` - Search state
 * - `sortField`, `sortDirection`, `onSortChange` - Sort state
 * - `density`, `onDensityChange` - Density state
 * - `selected`, `isSelected`, `onSelectionChange` - Selection state
 * - And more...
 *
 * **Note:** `useListingTable()` returns `null` outside of Provider (safe for optional usage).
 * Use `useListingTableRequired()` for strict mode that throws if no Provider.
 *
 * This example shows custom filter chips that use the hook to filter by component type.
 */
export const UseListingTableHook: Story = {
  name: 'useListingTable Hook',
  render: () => {
    const [searchValue, setSearchValue] = useState('');

    const filteredComponents = useMemo(() => {
      if (!searchValue) return platformComponents;
      const lower = searchValue.toLowerCase();
      return platformComponents.filter(
        (c) =>
          c.name.toLowerCase().includes(lower) ||
          c.description.toLowerCase().includes(lower) ||
          c.type.toLowerCase().includes(lower),
      );
    }, [searchValue]);

    return (
      <ListingTable.Provider searchValue={searchValue} onSearchChange={setSearchValue}>
        <ListingTable.Container sx={{ minWidth: 700 }}>
          <ListingTable.Toolbar
            actions={
              <Button variant="contained" size="small" startIcon={<Plus size={16} />}>
                New Component
              </Button>
            }
          >
            {/* Custom filter chips using useListingTable hook */}
            <TypeFilterChips />
          </ListingTable.Toolbar>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Type</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
                <ListingTable.Cell>Environment</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {filteredComponents.length > 0 ? (
                filteredComponents.slice(0, 5).map((component) => (
                  <ListingTable.Row key={component.id} hover>
                    <ListingTable.Cell>
                      <ListingTable.CellIcon
                        icon={getComponentIcon(component.type)}
                        primary={component.name}
                        secondary={component.description}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip label={component.type} size="small" variant="outlined" />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        icon={getStatusIcon(component.status) || undefined}
                        label={component.status}
                        size="small"
                        color={getStatusColor(component.status)}
                        variant="outlined"
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>{component.environment}</ListingTable.Cell>
                  </ListingTable.Row>
                ))
              ) : (
                <ListingTable.Row>
                  <ListingTable.Cell colSpan={4}>
                    <Box sx={{ textAlign: 'center', py: 4 }}>
                      <Typography color="text.secondary">No components match your filter.</Typography>
                    </Box>
                  </ListingTable.Cell>
                </ListingTable.Row>
              )}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </ListingTable.Provider>
    );
  },
};
