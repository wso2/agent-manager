# AI Agent Management Platform - Views Library

This is the common UI components library for the AI Agent Management Platform. It provides reusable React components built with Material-UI (MUI) v7.

## Features

- **Material-UI v7**: Modern, accessible UI components
- **TypeScript**: Full type safety and IntelliSense support
- **Storybook**: Interactive component documentation and testing
- **Vitest**: Fast unit testing with React Testing Library
- **AI Agent Focused**: Components designed specifically for AI agent management

## Components

### Card
A flexible card component built on MUI's Card with additional features:
- Supports all MUI Card props
- Custom className support
- Built-in CardContent wrapper
- AI agent themed examples

## Development

### Prerequisites
- Node.js 18+ 
- pnpm (recommended) or npm

### Setup
```bash
# Install dependencies
pnpm install

# Start development server
pnpm dev

# Run tests
pnpm test

# Run Storybook
pnpm storybook

# Build library
pnpm build
```

### Testing
```bash
# Run tests once
pnpm test

# Run tests in watch mode
pnpm test-watch

# Run tests with UI
pnpm test:ui
```

### Storybook
```bash
# Start Storybook development server
pnpm storybook

# Build Storybook for production
pnpm build-storybook
```

## Usage

```tsx
import { Card } from '@agent-management-platform/views';

function MyComponent() {
  return (
    <Card variant="outlined" className="my-custom-class">
      <h3>AI Agent Status</h3>
      <p>Agent is running and healthy</p>
    </Card>
  );
}
```

## Architecture

- **Components**: Located in `src/` directory
- **Stories**: Component documentation in `*.stories.tsx` files
- **Tests**: Unit tests in `*.test.tsx` files
- **Build**: Outputs to `dist/` directory
- **Types**: TypeScript definitions included

## Migration from Open Choreo

This library has been migrated from Open Choreo's views library with the following changes:
- Updated to AI Agent Management Platform branding
- Upgraded to MUI v7
- Removed Choreo-specific components
- Added AI agent focused examples and themes
- Updated testing setup to use Vitest
- Enhanced Storybook configuration with MUI theme support

## Contributing

1. Create feature branch from `main`
2. Add/update components with tests and stories
3. Ensure all tests pass: `pnpm test`
4. Update Storybook stories if needed
5. Submit pull request

## License

Apache-2.0
