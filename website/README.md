# Website

This website is built using [Docusaurus](https://docusaurus.io/), a modern static website generator.

## Installation

```bash
yarn
```

## Local Development

```bash
yarn start
```

This command starts a local development server and opens up a browser window. Most changes are reflected live without having to restart the server.

## Build

```bash
yarn build
```

This command generates static content into the `build` directory and can be served using any static contents hosting service.

## Versioning

### Creating a New Version

To create a new documentation version (e.g., v0.4.x):

```bash
npm run docusaurus docs:version v0.4.x
```

This command will:
- Snapshot all current docs from `docs/` into `versioned_docs/version-v0.4.x/`
- Create a sidebar snapshot at `versioned_sidebars/version-v0.4.x-sidebars.json`
- Add the version to `versions.json`

After creating a version, update `docusaurus.config.ts` to configure the version label and banner settings.

### Deleting a Version

To delete a version (e.g., v0.3.x):

1. Remove the version from `versions.json`
2. Delete `versioned_docs/version-v0.3.x/`
3. Delete `versioned_sidebars/version-v0.3.x-sidebars.json`
4. Remove version configuration from `docusaurus.config.ts`

## Deployment

Using SSH:

```bash
USE_SSH=true yarn deploy
```

Not using SSH:

```bash
GIT_USER=<Your GitHub username> yarn deploy
```

If you are using GitHub pages for hosting, this command is a convenient way to build the website and push to the `gh-pages` branch.
