# Oxygen UI Common Patterns

Ready-to-use UI patterns for common application scenarios.

## Table of Contents

- [App Shell Layout](#app-shell-layout)
- [Authentication Forms](#authentication-forms)
- [Dashboard Layout](#dashboard-layout)
- [Data Tables](#data-tables)
- [Modal Dialogs](#modal-dialogs)
- [Error States](#error-states)
- [Form Patterns](#form-patterns)
- [Navigation Patterns](#navigation-patterns)
- [Theme Switching](#theme-switching)

---

## App Shell Layout

Complete application shell with header, sidebar, and content area.

```tsx
import { useState } from 'react';
import {
  OxygenUIThemeProvider,
  OxygenTheme,
  AppShell,
  Header,
  Sidebar,
  Footer,
  ColorSchemeToggle,
  UserMenu,
} from '@wso2/oxygen-ui';
import { HomeIcon, DashboardIcon, SettingsIcon } from '@wso2/oxygen-ui-icons-react';

function App() {
  const [collapsed, setCollapsed] = useState(false);
  const [activeItem, setActiveItem] = useState('home');

  return (
    <OxygenUIThemeProvider theme={OxygenTheme}>
      <AppShell>
        <AppShell.Navbar>
          <Header>
            <Header.Toggle
              collapsed={collapsed}
              onToggle={() => setCollapsed(!collapsed)}
            />
            <Header.Brand>
              <Header.BrandLogo>
                <img src="/logo.svg" alt="Logo" height={32} />
              </Header.BrandLogo>
              <Header.BrandTitle>My App</Header.BrandTitle>
            </Header.Brand>
            <Header.Spacer />
            <Header.Actions>
              <ColorSchemeToggle />
              <UserMenu
                user={{ name: 'John Doe', email: 'john@example.com' }}
                onSignOut={() => console.log('Sign out')}
              />
            </Header.Actions>
          </Header>
        </AppShell.Navbar>

        <AppShell.Sidebar>
          <Sidebar
            collapsed={collapsed}
            activeItem={activeItem}
            onSelect={setActiveItem}
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
        </AppShell.Sidebar>

        <AppShell.Main>
          <Box sx={{ p: 3 }}>
            {/* Page content goes here */}
          </Box>
        </AppShell.Main>

        <AppShell.Footer>
          <Footer companyName="WSO2 LLC" />
        </AppShell.Footer>
      </AppShell>
    </OxygenUIThemeProvider>
  );
}
```

---

## Authentication Forms

### Login Form

```tsx
import { useState } from 'react';
import {
  Box,
  Paper,
  Typography,
  TextField,
  Button,
  Link,
  Divider,
  Alert,
} from '@wso2/oxygen-ui';

function LoginForm() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      // Login logic here
    } catch (err) {
      setError('Invalid credentials');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        bgcolor: 'background.default',
      }}
    >
      <Paper sx={{ p: 4, maxWidth: 400, width: '100%' }}>
        <Typography variant="h5" gutterBottom align="center">
          Sign In
        </Typography>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <Box component="form" onSubmit={handleSubmit}>
          <TextField
            fullWidth
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            margin="normal"
            required
          />

          <TextField
            fullWidth
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            margin="normal"
            required
          />

          <Button
            fullWidth
            type="submit"
            variant="contained"
            disabled={loading}
            sx={{ mt: 3, mb: 2 }}
          >
            {loading ? 'Signing in...' : 'Sign In'}
          </Button>

          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Link href="/forgot-password" variant="body2">
              Forgot password?
            </Link>
            <Link href="/register" variant="body2">
              Create account
            </Link>
          </Box>

          <Divider sx={{ my: 3 }}>or</Divider>

          <Button fullWidth variant="outlined" startIcon={<GoogleIcon />}>
            Continue with Google
          </Button>
        </Box>
      </Paper>
    </Box>
  );
}
```

### Password Reset Form

```tsx
import { useState } from 'react';
import {
  Box,
  Paper,
  Typography,
  TextField,
  Button,
  Alert,
} from '@wso2/oxygen-ui';

function PasswordResetForm() {
  const [email, setEmail] = useState('');
  const [submitted, setSubmitted] = useState(false);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Send reset email
    setSubmitted(true);
  };

  if (submitted) {
    return (
      <Paper sx={{ p: 4, maxWidth: 400 }}>
        <Alert severity="success">
          If an account exists for {email}, you will receive a password reset link.
        </Alert>
        <Button href="/login" sx={{ mt: 2 }}>
          Back to Login
        </Button>
      </Paper>
    );
  }

  return (
    <Paper sx={{ p: 4, maxWidth: 400 }}>
      <Typography variant="h5" gutterBottom>
        Reset Password
      </Typography>
      <Typography color="text.secondary" sx={{ mb: 3 }}>
        Enter your email address and we'll send you a reset link.
      </Typography>

      <Box component="form" onSubmit={handleSubmit}>
        <TextField
          fullWidth
          label="Email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <Button fullWidth type="submit" variant="contained" sx={{ mt: 3 }}>
          Send Reset Link
        </Button>
      </Box>
    </Paper>
  );
}
```

---

## Dashboard Layout

### Stats Cards

```tsx
import { Grid, Paper, Typography, Box } from '@wso2/oxygen-ui';
import { TrendingUpIcon, TrendingDownIcon } from '@wso2/oxygen-ui-icons-react';

interface StatCardProps {
  title: string;
  value: string | number;
  change?: number;
  icon?: React.ReactNode;
}

function StatCard({ title, value, change, icon }: StatCardProps) {
  const isPositive = change && change > 0;

  return (
    <Paper sx={{ p: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
        <Typography color="text.secondary" variant="body2">
          {title}
        </Typography>
        {icon}
      </Box>

      <Typography variant="h4" fontWeight="bold">
        {value}
      </Typography>

      {change !== undefined && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            mt: 1,
            color: isPositive ? 'success.main' : 'error.main',
          }}
        >
          {isPositive ? <TrendingUpIcon size={16} /> : <TrendingDownIcon size={16} />}
          <Typography variant="body2" sx={{ ml: 0.5 }}>
            {Math.abs(change)}% from last month
          </Typography>
        </Box>
      )}
    </Paper>
  );
}

function Dashboard() {
  return (
    <Box sx={{ p: 3 }}>
      <Typography variant="h4" gutterBottom>
        Dashboard
      </Typography>

      <Grid container spacing={3}>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard title="Total Users" value="12,345" change={12.5} />
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard title="Revenue" value="$45,678" change={8.2} />
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard title="Active Sessions" value="1,234" change={-3.1} />
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <StatCard title="Conversion Rate" value="3.2%" change={0.8} />
        </Grid>
      </Grid>
    </Box>
  );
}
```

### Dashboard with Charts

```tsx
import { Grid, Paper, Typography, Box, Charts } from '@wso2/oxygen-ui';

function DashboardCharts() {
  const salesData = [
    { month: 'Jan', sales: 4000, orders: 240 },
    { month: 'Feb', sales: 3000, orders: 139 },
    { month: 'Mar', sales: 2000, orders: 980 },
    { month: 'Apr', sales: 2780, orders: 390 },
    { month: 'May', sales: 1890, orders: 480 },
    { month: 'Jun', sales: 2390, orders: 380 },
  ];

  return (
    <Grid container spacing={3}>
      <Grid item xs={12} md={8}>
        <Paper sx={{ p: 3 }}>
          <Typography variant="h6" gutterBottom>
            Sales Overview
          </Typography>
          <Charts.LineChart
            height={300}
            series={[
              { data: salesData.map((d) => d.sales), label: 'Sales' },
              { data: salesData.map((d) => d.orders), label: 'Orders' },
            ]}
            xAxis={[{ data: salesData.map((d) => d.month), scaleType: 'band' }]}
          />
        </Paper>
      </Grid>

      <Grid item xs={12} md={4}>
        <Paper sx={{ p: 3 }}>
          <Typography variant="h6" gutterBottom>
            Traffic Sources
          </Typography>
          <Charts.PieChart
            height={300}
            series={[
              {
                data: [
                  { value: 45, label: 'Organic' },
                  { value: 30, label: 'Direct' },
                  { value: 15, label: 'Referral' },
                  { value: 10, label: 'Social' },
                ],
              },
            ]}
          />
        </Paper>
      </Grid>
    </Grid>
  );
}
```

---

## Data Tables

### Basic Data Table

```tsx
import { useState, useMemo } from 'react';
import { ListingTable, Button, IconButton, Chip } from '@wso2/oxygen-ui';
import { EditIcon, TrashIcon, MoreVerticalIcon } from '@wso2/oxygen-ui-icons-react';

interface User {
  id: string;
  name: string;
  email: string;
  role: string;
  status: 'active' | 'inactive';
}

function UsersTable({ users }: { users: User[] }) {
  const [search, setSearch] = useState('');
  const [sortField, setSortField] = useState('name');
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);

  const filteredData = useMemo(() => {
    let result = [...users];

    // Filter by search
    if (search) {
      result = result.filter(
        (u) =>
          u.name.toLowerCase().includes(search.toLowerCase()) ||
          u.email.toLowerCase().includes(search.toLowerCase())
      );
    }

    // Sort
    result.sort((a, b) => {
      const aVal = a[sortField as keyof User];
      const bVal = b[sortField as keyof User];
      const cmp = aVal < bVal ? -1 : aVal > bVal ? 1 : 0;
      return sortDirection === 'asc' ? cmp : -cmp;
    });

    return result;
  }, [users, search, sortField, sortDirection]);

  const paginatedData = filteredData.slice(
    page * rowsPerPage,
    page * rowsPerPage + rowsPerPage
  );

  return (
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
      totalCount={filteredData.length}
      onPageChange={setPage}
      onRowsPerPageChange={setRowsPerPage}
    >
      <ListingTable.Container>
        <ListingTable.Toolbar
          showSearch
          searchPlaceholder="Search users..."
          actions={
            <Button variant="contained" size="small">
              Add User
            </Button>
          }
        />

        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>
                <ListingTable.SortLabel field="name">Name</ListingTable.SortLabel>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <ListingTable.SortLabel field="email">Email</ListingTable.SortLabel>
              </ListingTable.Cell>
              <ListingTable.Cell>Role</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell align="right">Actions</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>

          <ListingTable.Body>
            {paginatedData.length === 0 ? (
              <ListingTable.Row>
                <ListingTable.Cell colSpan={5}>
                  <ListingTable.EmptyState message="No users found" />
                </ListingTable.Cell>
              </ListingTable.Row>
            ) : (
              paginatedData.map((user) => (
                <ListingTable.Row key={user.id}>
                  <ListingTable.Cell>{user.name}</ListingTable.Cell>
                  <ListingTable.Cell>{user.email}</ListingTable.Cell>
                  <ListingTable.Cell>{user.role}</ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      label={user.status}
                      color={user.status === 'active' ? 'success' : 'default'}
                      size="small"
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <IconButton size="small">
                      <EditIcon size={18} />
                    </IconButton>
                    <IconButton size="small" color="error">
                      <TrashIcon size={18} />
                    </IconButton>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))
            )}
          </ListingTable.Body>

          <ListingTable.Footer />
        </ListingTable>
      </ListingTable.Container>
    </ListingTable.Provider>
  );
}
```

### Card Variant Table

```tsx
<ListingTable variant="card" density="comfortable">
  <ListingTable.Head>
    <ListingTable.Row>
      <ListingTable.Cell>Project</ListingTable.Cell>
      <ListingTable.Cell>Status</ListingTable.Cell>
      <ListingTable.Cell>Last Updated</ListingTable.Cell>
      <ListingTable.Cell align="right">Actions</ListingTable.Cell>
    </ListingTable.Row>
  </ListingTable.Head>
  <ListingTable.Body>
    {projects.map((project) => (
      <ListingTable.Row key={project.id}>
        <ListingTable.Cell>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <ListingTable.CellIcon>
              <FolderIcon />
            </ListingTable.CellIcon>
            <Box>
              <Typography fontWeight="medium">{project.name}</Typography>
              <Typography variant="body2" color="text.secondary">
                {project.description}
              </Typography>
            </Box>
          </Box>
        </ListingTable.Cell>
        <ListingTable.Cell>
          <Chip label={project.status} size="small" />
        </ListingTable.Cell>
        <ListingTable.Cell>{project.updatedAt}</ListingTable.Cell>
        <ListingTable.Cell align="right">
          <ListingTable.RowActions
            actions={[
              { id: 'view', label: 'View', onClick: () => {} },
              { id: 'edit', label: 'Edit', onClick: () => {} },
              { id: 'delete', label: 'Delete', onClick: () => {}, color: 'error' },
            ]}
          />
        </ListingTable.Cell>
      </ListingTable.Row>
    ))}
  </ListingTable.Body>
</ListingTable>
```

---

## Modal Dialogs

### Confirmation Dialog

```tsx
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Button,
} from '@wso2/oxygen-ui';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
  loading?: boolean;
  destructive?: boolean;
}

function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  onConfirm,
  onCancel,
  loading = false,
  destructive = false,
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onClose={onCancel} maxWidth="xs" fullWidth>
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <DialogContentText>{message}</DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} disabled={loading}>
          {cancelLabel}
        </Button>
        <Button
          onClick={onConfirm}
          variant="contained"
          color={destructive ? 'error' : 'primary'}
          disabled={loading}
        >
          {loading ? 'Processing...' : confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// Usage
<ConfirmDialog
  open={deleteDialogOpen}
  title="Delete User"
  message="Are you sure you want to delete this user? This action cannot be undone."
  confirmLabel="Delete"
  onConfirm={handleDelete}
  onCancel={() => setDeleteDialogOpen(false)}
  destructive
/>
```

### Form Dialog

```tsx
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
} from '@wso2/oxygen-ui';

function CreateUserDialog({ open, onClose, onSubmit }) {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');

  const handleSubmit = () => {
    onSubmit({ name, email });
    onClose();
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create User</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1 }}>
          <TextField
            fullWidth
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            margin="normal"
            required
          />
          <TextField
            fullWidth
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            margin="normal"
            required
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={handleSubmit} variant="contained">
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
}
```

---

## Error States

### 404 Page

```tsx
import { Box, Typography, Button } from '@wso2/oxygen-ui';

function NotFoundPage() {
  return (
    <Box
      sx={{
        minHeight: '60vh',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        textAlign: 'center',
        p: 3,
      }}
    >
      <Typography variant="h1" sx={{ fontSize: '6rem', fontWeight: 'bold', color: 'text.secondary' }}>
        404
      </Typography>
      <Typography variant="h5" gutterBottom>
        Page not found
      </Typography>
      <Typography color="text.secondary" sx={{ mb: 4, maxWidth: 400 }}>
        The page you're looking for doesn't exist or has been moved.
      </Typography>
      <Button variant="contained" href="/">
        Go to Home
      </Button>
    </Box>
  );
}
```

### Error Boundary Fallback

```tsx
import { Box, Typography, Button, Paper } from '@wso2/oxygen-ui';
import { AlertTriangleIcon } from '@wso2/oxygen-ui-icons-react';

function ErrorFallback({ error, resetError }) {
  return (
    <Box
      sx={{
        minHeight: '60vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        p: 3,
      }}
    >
      <Paper sx={{ p: 4, maxWidth: 500, textAlign: 'center' }}>
        <AlertTriangleIcon size={48} color="error" />
        <Typography variant="h5" sx={{ mt: 2 }} gutterBottom>
          Something went wrong
        </Typography>
        <Typography color="text.secondary" sx={{ mb: 3 }}>
          {error?.message || 'An unexpected error occurred'}
        </Typography>
        <Button variant="contained" onClick={resetError}>
          Try Again
        </Button>
      </Paper>
    </Box>
  );
}
```

### Empty State

```tsx
import { Box, Typography, Button } from '@wso2/oxygen-ui';
import { InboxIcon } from '@wso2/oxygen-ui-icons-react';

function EmptyState({
  icon = <InboxIcon size={48} />,
  title,
  description,
  action,
  actionLabel,
}) {
  return (
    <Box
      sx={{
        py: 8,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        textAlign: 'center',
      }}
    >
      <Box sx={{ color: 'text.secondary', mb: 2 }}>{icon}</Box>
      <Typography variant="h6" gutterBottom>
        {title}
      </Typography>
      {description && (
        <Typography color="text.secondary" sx={{ mb: 3, maxWidth: 400 }}>
          {description}
        </Typography>
      )}
      {action && actionLabel && (
        <Button variant="contained" onClick={action}>
          {actionLabel}
        </Button>
      )}
    </Box>
  );
}

// Usage
<EmptyState
  title="No projects yet"
  description="Create your first project to get started"
  actionLabel="Create Project"
  action={() => setCreateDialogOpen(true)}
/>
```

---

## Form Patterns

### Multi-Step Wizard

```tsx
import { useState } from 'react';
import {
  Box,
  Paper,
  Stepper,
  Step,
  StepLabel,
  Button,
  Typography,
  TextField,
} from '@wso2/oxygen-ui';

const steps = ['Account', 'Profile', 'Preferences'];

function RegistrationWizard() {
  const [activeStep, setActiveStep] = useState(0);
  const [formData, setFormData] = useState({
    email: '',
    password: '',
    name: '',
    company: '',
    notifications: true,
  });

  const handleNext = () => {
    setActiveStep((prev) => prev + 1);
  };

  const handleBack = () => {
    setActiveStep((prev) => prev - 1);
  };

  const handleSubmit = () => {
    console.log('Form submitted:', formData);
  };

  const renderStepContent = (step: number) => {
    switch (step) {
      case 0:
        return (
          <>
            <TextField
              fullWidth
              label="Email"
              type="email"
              value={formData.email}
              onChange={(e) => setFormData({ ...formData, email: e.target.value })}
              margin="normal"
            />
            <TextField
              fullWidth
              label="Password"
              type="password"
              value={formData.password}
              onChange={(e) => setFormData({ ...formData, password: e.target.value })}
              margin="normal"
            />
          </>
        );
      case 1:
        return (
          <>
            <TextField
              fullWidth
              label="Full Name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              margin="normal"
            />
            <TextField
              fullWidth
              label="Company"
              value={formData.company}
              onChange={(e) => setFormData({ ...formData, company: e.target.value })}
              margin="normal"
            />
          </>
        );
      case 2:
        return (
          <Typography>
            Review your information and click Submit to complete registration.
          </Typography>
        );
      default:
        return null;
    }
  };

  return (
    <Paper sx={{ p: 4, maxWidth: 600, mx: 'auto' }}>
      <Stepper activeStep={activeStep} sx={{ mb: 4 }}>
        {steps.map((label) => (
          <Step key={label}>
            <StepLabel>{label}</StepLabel>
          </Step>
        ))}
      </Stepper>

      <Box sx={{ minHeight: 200 }}>{renderStepContent(activeStep)}</Box>

      <Box sx={{ display: 'flex', justifyContent: 'space-between', mt: 4 }}>
        <Button disabled={activeStep === 0} onClick={handleBack}>
          Back
        </Button>
        {activeStep === steps.length - 1 ? (
          <Button variant="contained" onClick={handleSubmit}>
            Submit
          </Button>
        ) : (
          <Button variant="contained" onClick={handleNext}>
            Next
          </Button>
        )}
      </Box>
    </Paper>
  );
}
```

### Form with Validation

```tsx
import { useState } from 'react';
import { Box, TextField, Button, Alert } from '@wso2/oxygen-ui';

interface FormErrors {
  [key: string]: string;
}

function ValidatedForm() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<FormErrors>({});

  const validate = (): boolean => {
    const newErrors: FormErrors = {};

    if (!email) {
      newErrors.email = 'Email is required';
    } else if (!/\S+@\S+\.\S+/.test(email)) {
      newErrors.email = 'Invalid email format';
    }

    if (!password) {
      newErrors.password = 'Password is required';
    } else if (password.length < 8) {
      newErrors.password = 'Password must be at least 8 characters';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (validate()) {
      // Submit form
    }
  };

  return (
    <Box component="form" onSubmit={handleSubmit}>
      <TextField
        fullWidth
        label="Email"
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        error={!!errors.email}
        helperText={errors.email}
        margin="normal"
      />

      <TextField
        fullWidth
        label="Password"
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        error={!!errors.password}
        helperText={errors.password}
        margin="normal"
      />

      <Button type="submit" variant="contained" sx={{ mt: 2 }}>
        Submit
      </Button>
    </Box>
  );
}
```

---

## Navigation Patterns

### Breadcrumbs

```tsx
import { Breadcrumbs, Link, Typography } from '@wso2/oxygen-ui';
import { ChevronRightIcon } from '@wso2/oxygen-ui-icons-react';

function PageBreadcrumbs({ items }: { items: { label: string; href?: string }[] }) {
  return (
    <Breadcrumbs
      separator={<ChevronRightIcon size={16} />}
      sx={{ mb: 2 }}
    >
      {items.map((item, index) => {
        const isLast = index === items.length - 1;
        return isLast ? (
          <Typography key={item.label} color="text.primary">
            {item.label}
          </Typography>
        ) : (
          <Link key={item.label} href={item.href} color="inherit" underline="hover">
            {item.label}
          </Link>
        );
      })}
    </Breadcrumbs>
  );
}

// Usage
<PageBreadcrumbs
  items={[
    { label: 'Home', href: '/' },
    { label: 'Users', href: '/users' },
    { label: 'John Doe' },
  ]}
/>
```

### Tab Navigation

```tsx
import { useState } from 'react';
import { Tabs, Tab, Box } from '@wso2/oxygen-ui';

function TabNavigation() {
  const [value, setValue] = useState(0);

  return (
    <Box>
      <Tabs value={value} onChange={(_, newValue) => setValue(newValue)}>
        <Tab label="Overview" />
        <Tab label="Settings" />
        <Tab label="Members" />
      </Tabs>

      <Box sx={{ p: 3 }}>
        {value === 0 && <div>Overview content</div>}
        {value === 1 && <div>Settings content</div>}
        {value === 2 && <div>Members content</div>}
      </Box>
    </Box>
  );
}
```

---

## Theme Switching

### Complete Theme Switcher Setup

```tsx
import {
  OxygenUIThemeProvider,
  OxygenTheme,
  AcrylicOrangeTheme,
  AcrylicPurpleTheme,
  ClassicTheme,
  HighContrastTheme,
  useThemeSwitcher,
  Select,
  MenuItem,
} from '@wso2/oxygen-ui';

const themes = [
  { key: 'default', label: 'Default', theme: OxygenTheme },
  { key: 'orange', label: 'Acrylic Orange', theme: AcrylicOrangeTheme },
  { key: 'purple', label: 'Acrylic Purple', theme: AcrylicPurpleTheme },
  { key: 'classic', label: 'Classic', theme: ClassicTheme },
  { key: 'highContrast', label: 'High Contrast', theme: HighContrastTheme },
];

function ThemeSelector() {
  const { currentTheme, themes, setTheme } = useThemeSwitcher();

  return (
    <Select
      value={currentTheme}
      onChange={(e) => setTheme(e.target.value as string)}
      size="small"
    >
      {themes.map((t) => (
        <MenuItem key={t.key} value={t.key}>
          {t.label}
        </MenuItem>
      ))}
    </Select>
  );
}

function App() {
  return (
    <OxygenUIThemeProvider themes={themes} initialTheme="default">
      <Header>
        <Header.Actions>
          <ThemeSelector />
          <ColorSchemeToggle />
        </Header.Actions>
      </Header>
      {/* Rest of app */}
    </OxygenUIThemeProvider>
  );
}
```
