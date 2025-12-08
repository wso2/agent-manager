import type { Meta, StoryObj } from '@storybook/react';
import { DataListingTable, TableColumn, renderStatusChip, renderMetrics } from './DataListingTable';
import { Box, Typography, Chip } from '@wso2/oxygen-ui';

const meta: Meta<typeof DataListingTable> = {
  title: 'AI Agent Management/Views/DataListingTable',
  component: DataListingTable,
  parameters: {
    layout: 'padded',
    docs: {
      description: {
        component: 'A reusable data listing table component with sorting, custom rendering, and action menus. Perfect for displaying agent data with status indicators and metrics.',
      },
    },
  },
  argTypes: {
    data: {
      control: 'object',
      description: 'Array of data objects to display in the table',
    },
    columns: {
      control: 'object',
      description: 'Array of column configurations defining how data is displayed',
    },
    loading: {
      control: 'boolean',
      description: 'Shows loading spinner when true',
    },
    actions: {
      control: 'object',
      description: 'Array of action menu items for each row',
    },
    onRowAction: {
      action: 'rowAction',
      description: 'Callback function when a row action is clicked',
    },
  },
};

export default meta;
type Story = StoryObj<typeof DataListingTable>;

// Sample data with status and metrics
interface SampleData {
  id: number;
  name: string;
  category: string;
  status: { color: 'success' | 'warning' | 'error' | 'default'; label: string };
  sessions: number;
  metrics: { metricsValue: string | number; metricsColor: 'success' | 'warning' | 'error' };
  warning?: boolean;
}

const sampleData: SampleData[] = [
  {
    id: 1,
    name: 'Content Generator',
    category: 'LangChain',
    status: { color: 'error', label: 'Critical' },
    sessions: 3245,
    metrics: { metricsValue: '0.5% errors', metricsColor: 'error' },
    warning: true,
  },
  {
    id: 2,
    name: 'Customer Support Agent',
    category: 'LangChain',
    status: { color: 'success', label: 'Healthy' },
    sessions: 12453,
    metrics: { metricsValue: '0.8% errors', metricsColor: 'success' },
  },
  {
    id: 3,
    name: 'Data Analysis Agent',
    category: 'AutoGPT',
    status: { color: 'warning', label: 'Warning' },
    sessions: 5621,
    metrics: { metricsValue: '8.2% errors', metricsColor: 'warning' },
  },
  {
    id: 4,
    name: 'Financial Advisory Agent',
    category: 'CrewAI',
    status: { color: 'warning', label: 'Warning' },
    sessions: 8932,
    metrics: { metricsValue: '12.4% errors', metricsColor: 'error' },
  },
  {
    id: 5,
    name: 'Legal Document Analyzer',
    category: 'LlamaIndex',
    status: { color: 'default', label: 'Stopped' },
    sessions: 4521,
    metrics: { metricsValue: '2.1% errors', metricsColor: 'success' },
  },
  {
    id: 6,
    name: 'Sales Intelligence Agent',
    category: 'Semantic Kernel',
    status: { color: 'warning', label: 'Warning' },
    sessions: 7834,
    metrics: { metricsValue: '1.4% errors', metricsColor: 'success' },
  },
];

const sampleColumns: TableColumn<SampleData>[] = [
  {
    id: 'name',
    label: 'Name',
    sortable: true,
    render: (value, row) => (
      <Box display="flex" alignItems="center" gap={1}>
        <Box
          width={40}
          height={40}
          borderRadius={1}
          bgcolor="primary.main"
          display="flex"
          alignItems="center"
          justifyContent="center"
          color="white"
          fontWeight="bold"
          fontSize="14px"
        >
          {row.category.charAt(0).toUpperCase()}
        </Box>
        <Box>
          <Box display="flex" alignItems="center" gap={1}>
            <Typography variant="subtitle2" fontWeight="bold">
              {row.name}
            </Typography>
            {row.warning && (
              <Box
                width={16}
                height={16}
                borderRadius="50%"
                bgcolor="error.main"
                display="flex"
                alignItems="center"
                justifyContent="center"
                color="white"
                fontSize="10px"
              >
                !
              </Box>
            )}
          </Box>
          <Typography variant="caption" color="text.secondary">
            {row.category}
          </Typography>
        </Box>
      </Box>
    ),
  },
  {
    id: 'status',
    label: 'Status',
    sortable: true,
    render: (value) => renderStatusChip(value),
  },
  {
    id: 'sessions',
    label: 'Sessions',
    sortable: true,
    render: (value) => value.toLocaleString(),
  },
  {
    id: 'metrics',
    label: 'METRICS',
    sortable: false,
    render: (value) => renderMetrics(value),
  },
];

const defaultActions = [
  { label: 'View Details', value: 'view' },
  { label: 'Edit Agent', value: 'edit' },
  { label: 'Deploy', value: 'deploy' },
  { label: 'Stop', value: 'stop' },
  { label: 'Delete', value: 'delete' },
];

export const Default: Story = {
  args: {
    data: sampleData,
    columns: sampleColumns,
    actions: defaultActions,
  },
};

export const WithoutActions: Story = {
  args: {
    data: sampleData,
    columns: sampleColumns,
  },
};

export const Loading: Story = {
  args: {
    data: [],
    columns: sampleColumns,
    loading: true,
  },
};

export const Empty: Story = {
  args: {
    data: [],
    columns: sampleColumns,
    actions: defaultActions,
  },
};

// Simple data example
interface SimpleData {
  id: number;
  name: string;
  email: string;
  role: string;
  status: 'active' | 'inactive';
}

const simpleData: SimpleData[] = [
  { id: 1, name: 'John Doe', email: 'john@example.com', role: 'Admin', status: 'active' },
  { id: 2, name: 'Jane Smith', email: 'jane@example.com', role: 'User', status: 'inactive' },
  { id: 3, name: 'Bob Johnson', email: 'bob@example.com', role: 'Moderator', status: 'active' },
];

const simpleColumns: TableColumn<SimpleData>[] = [
  {
    id: 'name',
    label: 'Name',
    sortable: true,
  },
  {
    id: 'email',
    label: 'Email',
    sortable: true,
  },
  {
    id: 'role',
    label: 'Role',
    sortable: true,
  },
  {
    id: 'status',
    label: 'Status',
    sortable: true,
    render: (value) => (
      <Chip 
        label={value} 
        color={value === 'active' ? 'success' : 'default'}
        size="small"
      />
    ),
  },
];

export const SimpleData: Story = {
  args: {
    data: simpleData,
    columns: simpleColumns,
    actions: [
      { label: 'Edit', value: 'edit' },
      { label: 'Delete', value: 'delete' },
    ],
  },
};

// Custom rendering example
interface ProductData {
  id: number;
  name: string;
  price: number;
  category: string;
  inStock: boolean;
  rating: number;
}

const productData: ProductData[] = [
  { id: 1, name: 'Laptop Pro', price: 1299.99, category: 'Electronics', inStock: true, rating: 4.5 },
  { id: 2, name: 'Wireless Mouse', price: 29.99, category: 'Accessories', inStock: false, rating: 4.2 },
  { id: 3, name: 'Mechanical Keyboard', price: 149.99, category: 'Accessories', inStock: true, rating: 4.8 },
];

const productColumns: TableColumn<ProductData>[] = [
  {
    id: 'name',
    label: 'Product',
    sortable: true,
  },
  {
    id: 'price',
    label: 'Price',
    sortable: true,
    render: (value) => `$${value.toFixed(2)}`,
  },
  {
    id: 'category',
    label: 'Category',
    sortable: true,
  },
  {
    id: 'inStock',
    label: 'Stock',
    sortable: true,
    render: (value) => (
      <Chip 
        label={value ? 'In Stock' : 'Out of Stock'} 
        color={value ? 'success' : 'error'}
        size="small"
      />
    ),
  },
  {
    id: 'rating',
    label: 'Rating',
    sortable: true,
    render: (value) => (
      <Box display="flex" alignItems="center" gap={0.5}>
        <Typography variant="body2">{value}</Typography>
        <Typography variant="body2" color="text.secondary">★</Typography>
      </Box>
    ),
  },
];

export const CustomRendering: Story = {
  args: {
    data: productData,
    columns: productColumns,
    actions: [
      { label: 'View Details', value: 'view' },
      { label: 'Edit Product', value: 'edit' },
      { label: 'Toggle Stock', value: 'toggle' },
    ],
  },
};

// Large dataset example
const generateLargeDataset = (count: number): SampleData[] => {
  const categories = ['LangChain', 'AutoGPT', 'CrewAI', 'LlamaIndex', 'Semantic Kernel', 'OpenAI'];
  const statuses = [
    { color: 'success' as const, label: 'Healthy' },
    { color: 'warning' as const, label: 'Warning' },
    { color: 'error' as const, label: 'Critical' },
    { color: 'default' as const, label: 'Stopped' },
  ];
  const metricsColors = ['success', 'warning', 'error'] as const;

  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    name: `Item ${i + 1}`,
    category: categories[i % categories.length],
    status: statuses[i % statuses.length],
    sessions: Math.floor(Math.random() * 20000) + 1000,
    metrics: {
      metricsValue: `${(Math.random() * 15).toFixed(1)}% errors`,
      metricsColor: metricsColors[Math.floor(Math.random() * metricsColors.length)],
    },
    warning: Math.random() > 0.8,
  }));
};

export const LargeDataset: Story = {
  args: {
    data: generateLargeDataset(50),
    columns: sampleColumns,
    actions: defaultActions,
  },
};

export const SortableDemo: Story = {
  args: {
    data: sampleData,
    columns: sampleColumns,
    actions: defaultActions,
  },
  parameters: {
    docs: {
      description: {
        story: 'Click on column headers to sort the data. Notice the arrow indicators showing sort direction.',
      },
    },
  },
};

export const StatusVariations: Story = {
  args: {
    data: [
      {
        id: 1,
        name: 'Healthy Item',
        category: 'LangChain',
        status: { color: 'success', label: 'Healthy' },
        sessions: 1000,
        metrics: { metricsValue: '0.5% errors', metricsColor: 'success' },
      },
      {
        id: 2,
        name: 'Warning Item',
        category: 'AutoGPT',
        status: { color: 'warning', label: 'Warning' },
        sessions: 2000,
        metrics: { metricsValue: '5.2% errors', metricsColor: 'warning' },
      },
      {
        id: 3,
        name: 'Critical Item',
        category: 'CrewAI',
        status: { color: 'error', label: 'Critical' },
        sessions: 500,
        metrics: { metricsValue: '15.8% errors', metricsColor: 'error' },
      },
      {
        id: 4,
        name: 'Stopped Item',
        category: 'LlamaIndex',
        status: { color: 'default', label: 'Stopped' },
        sessions: 0,
        metrics: { metricsValue: '0% errors', metricsColor: 'success' },
      },
    ],
    columns: sampleColumns,
    actions: defaultActions,
  },
  parameters: {
    docs: {
      description: {
        story: 'Different status indicators and their corresponding colors and metrics.',
      },
    },
  },
};
