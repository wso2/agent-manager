# AI Agent Management Platform Documentation & Website

This repository contains the source code for the AI Agent Management Platform documentation website, built with [Docusaurus](https://docusaurus.io/).

The AI Agent Management Platform is an enterprise-grade platform for managing, monitoring, and orchestrating AI agents at scale.

## Prerequisites

- **Node.js** version 20.0 or above
- **npm** (comes with Node.js)

## Installation

```bash
npm install
```

## Local Development

```bash
npm run start
```

This command starts a local development server and opens a browser window. Most changes are reflected live without having to restart the server.

The site will be available at `http://localhost:3000`

## Build

```bash
npm run build
```

This command generates static content into the `build` directory and can be served using any static contents hosting service.

## Project Structure

```
├── blog/                 # Blog content (optional)
├── docs/                 # Current documentation (next version)
├── versioned_docs/       # Documentation for released versions
│   └── version-v0.x.x/   # Docs for released versions
├── versioned_sidebars/   # Sidebar configs for each version
├── versions.json         # List of documentation versions
├── src/                  # React components and custom pages
│   ├── components/       # Reusable React components
│   ├── css/              # Custom styles
│   └── pages/            # Custom pages (homepage, etc.)
├── static/               # Static assets
├── docusaurus.config.ts  # Main configuration file
├── sidebars.ts           # Sidebar navigation structure
└── package.json          # Dependencies and scripts
```

## Writing Documentation

### Adding a New Documentation Page

1. Create a new Markdown file in the appropriate `docs/` subdirectory
2. Add front matter at the beginning of the file:
   ```markdown
   ---
   title: Your Page Title
   ---
   ```
3. Write your content using Markdown or MDX
4. Update `sidebars.ts` to include your page in the navigation

### Linking to Other Docs

When linking to other documentation pages, use relative file paths with the `.md` extension:

```markdown
[Link to another page](../concepts/architecture.md)
```

This approach ensures links work on GitHub, in Markdown editors, and with Docusaurus versioning.

### Using Icons

Use Lucide icons instead of emojis for consistent, professional documentation. Two components are available:

**DocIcon** - For inline usage in text:
```mdx
import DocIcon from '@site/src/components/DocIcon';

<DocIcon name="Info" /> This is an informational note
<DocIcon name="CheckCircle" color="green" /> Success message
```

**Icon** - For standalone or larger icons:
```mdx
import Icon from '@site/src/components/Icon';

<Icon name="Rocket" size={32} />
```

Browse icons at [lucide.dev/icons](https://lucide.dev/icons). See the [Icons Guide](docs/contributing/icons-guide.md) for detailed usage and examples.

## Documentation Versioning

The platform documentation uses versioning to maintain docs for different releases. We version by minor releases (e.g., v0.3.x, v0.4.x) since breaking changes may occur between minor versions during pre-1.0 development.

### Creating a New Version

When releasing a new minor version (e.g., v0.4.0):

1. **Create the documentation version**:
   ```bash
   npm run docusaurus docs:version v0.4.x
   ```

   This will:
   - Copy the current `docs/` folder to `versioned_docs/version-v0.4.x/`
   - Create `versioned_sidebars/version-v0.4.x-sidebars.json`
   - Add the version to `versions.json`

2. **Update `docusaurus.config.ts`** to configure the version label and banner settings if needed.

### Updating Versioned Documentation

To update documentation for a specific version:

1. **For the current development version**: Edit files in the `docs/` folder
2. **For a released version** (e.g., v0.3.x): Edit files in `versioned_docs/version-v0.3.x/`

### Version Strategy

- **Current (`docs/`)**: Next unreleased minor version documentation
- **Versioned (`versioned_docs/version-v0.X.x/`)**: Frozen documentation for released versions
- **Patch releases** (e.g., v0.3.1, v0.3.2): Update the corresponding minor version docs (v0.3.x) if needed

### Deleting a Version

To delete a version (e.g., v0.3.x):

1. Remove the version from `versions.json`
2. Delete `versioned_docs/version-v0.3.x/`
3. Delete `versioned_sidebars/version-v0.3.x-sidebars.json`
4. Remove version configuration from `docusaurus.config.ts`

## Deployment

The site automatically deploys to GitHub Pages when changes are pushed to the main branch via GitHub Actions.

### Manual Deployment

Using SSH:

```bash
USE_SSH=true npm run deploy
```

Not using SSH:

```bash
GIT_USER=<Your GitHub username> npm run deploy
```

If you are using GitHub pages for hosting, this command is a convenient way to build the website and push to the `gh-pages` branch.

## Contributing

We welcome contributions to improve the AI Agent Management Platform documentation!

### How to Contribute

1. Fork the repository
2. Create a feature branch (`git checkout -b improve-<topic>`)
3. Make your changes
4. Test locally with `npm run start`
5. Build to check for errors: `npm run build`
6. Commit your changes with a descriptive message
7. Push to your fork (`git push origin improve-<topic>`)
8. Create a Pull Request with a clear description of your changes

### Pull Request Checklist

Before submitting a PR, please ensure:

- [ ] Updated `sidebars.ts` if adding a new documentation page
- [ ] Run `npm run start` to preview the changes locally
- [ ] Run `npm run build` to ensure the build passes without errors
- [ ] Verified all links are working (no broken links)

## Links

- [AI Agent Management Platform Repository](https://github.com/wso2/ai-agent-management-platform)
- [Documentation](https://wso2.github.io/ai-agent-management-platform/)
