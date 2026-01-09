# Agent Manager Console

React/TypeScript web application for the Agent Manager platform, built as a Rush monorepo.

## Tech Stack

- **React 19** - UI framework
- **TypeScript** - Type safety
- **Vite** - Build tool and dev server
- **Rush** - Monorepo management
- **pnpm** - Package manager

## Prerequisites

Before you begin, ensure you have the following installed:

- **Node.js**: Version 18.20.3+ or 20.14.0+ (see supported versions in rush.json)
- **Rush**: The monorepo management tool
- **pnpm**: Package manager (installed automatically by Rush)

### Installing Rush

Install Rush globally:

```bash
npm install -g @microsoft/rush
```

Verify installation:
```bash
rush --version
```

## Getting Started

### 1. Install Dependencies

From the `console/` directory, install all dependencies for the monorepo:

```bash
cd console
make install
```

This command will:
- Install Rush's local copy of pnpm
- Install all dependencies for all projects in the monorepo
- Create symlinks between local packages

### 2. Build Libraries

Build all shared libraries first:

```bash
make build-webapp
```

Or build all projects:
```bash
make build
```

### 3. Start Development Server

```bash
make dev
```

This will:
- Start all library dependencies in watch mode
- Launch the Vite dev server at `http://localhost:3000`
- Automatically rebuild dependencies when you make changes
- Hot-reload the webapp when dependencies update

Press `Ctrl+C` to stop all processes.

### 4. Environment Configuration

Copy the configuration template and customize it:

```bash
cp apps/webapp/public/config.js.template apps/webapp/public/config.js
```

Edit `apps/webapp/public/config.js` to set your API URL:

```javascript
window.APP_CONFIG = {
  API_URL: 'http://localhost:8080'
};
```

## Available Commands

### Make Commands (Recommended)

```bash
# Start development mode with hot-reload
make dev

# Install dependencies
make install

# Build all projects
make build

# Clean build outputs
make clean

# Purge Rush cache
make purge

# Show all available commands
make help
```

### Rush Commands

```bash
# Install dependencies
rush install

# Build all projects
rush build

# Build specific project and its dependencies
rush build --to @agent-management-platform/webapp

# Run linting for all projects
rush lint

# Run tests for all projects
rush test

# Clean all build outputs
rush purge

# Update dependencies
rush update

# Create a new page component
rush create-page
```

### Project-Specific Commands

Navigate to any project directory and use `rushx`:

```bash
cd apps/webapp

# Start development server
rushx dev

# Build for production
rushx build

# Run linting
rushx lint

# Fix linting issues
rushx lint:fix

# Preview production build
rushx preview
```

## Creating New Page Components

Create new page components with the same structure and dependencies as existing pages using the integrated Yeoman generator.

### Using the Page Generator

From the `console/` directory, run the Rush command:
```bash
cd console
rush create-page
```

**Answer the prompts**:
   - **Package name** (e.g., `user-dashboard`) - use kebab-case
   - **Display title** (e.g., `User Dashboard`) - will auto-generate from package name
   - **Description** (e.g., `A dashboard page for managing users`)
   - **Route path** (e.g., `/user-dashboard`)

**Example interaction**:
```bash
$ rush create-page

? What is the name of your page package? (my-page) user-dashboard
? What is the display title for your page? (User Dashboard) User Dashboard
? What is the description for your page? (A page component for User Dashboard) A dashboard page for managing users
? What is the route path for your page? (/user/dashboard) /user-dashboard

Template generated successfully!
Next steps:
1. Add the new page to rush.json projects list
2. Run rush update to install dependencies
3. Run rushx build to build the package
4. Run rushx storybook to view the component in Storybook
```

**After generating a new page, update Rush configuration**:

1. **Add the new page to Rush projects list**:
   Edit `rush.json` and add your new page to the `projects` array:
   ```json
   {
     "packageName": "@agent-management-platform/your-page-name",
     "projectFolder": "workspaces/pages/your-page-name"
   }
   ```

2. **Update Rush to recognize the new package**:
   ```bash
   # Go back to console root
   cd ../../..
   
   # Update Rush to recognize the new package
   rush update
   ```

3. **Build your new page**:
   ```bash
   cd workspaces/pages/your-page-name
   rushx build
   rushx storybook  # Optional: to view in Storybook
   ```

## Project Structure Details

### Apps
- **webapp**: Main React application with Vite build system

### Libraries
- **auth**: Authentication provider and hooks
- **types**: Shared TypeScript type definitions
- **eslint-config**: Shared ESLint configuration
- **views**: Shared UI components and themes
- **api-client**: API client utilities

### Pages
- **AgentsListPage**: Example page component (use as reference)
- **.template**: Yeoman generator template for creating new pages (integrated with Rush)

### Rush Commands
- **create-page**: Rush command to create new page components using the Yeoman generator

