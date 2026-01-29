# GitHub Pages Deployment Setup

## Files Created

1. **`.github/workflows/docs-deploy.yml`** - Deploys to GitHub Pages on pushes to `main`
2. **`.github/workflows/test-deploy.yml`** - Tests build on pull requests

## Configuration Changes

- Updated `website/docusaurus.config.ts` to use `anoshanj` instead of `wso2`
- Site will be available at: `https://anoshanj.github.io/ai-agent-management-platform/`

## Next Steps

### 1. Enable GitHub Pages in Repository Settings

Go to your repository settings on GitHub:
- Navigate to: Settings → Pages
- Under "Build and deployment":
  - **Source**: Select "GitHub Actions"
  
### 2. Push Changes to Main Branch

```bash
# From the repository root
git add .github/workflows/ website/docusaurus.config.ts
git commit -m "Add GitHub Actions workflow for docs deployment"
git push origin gh-pages

# Then merge to main (or create a PR)
git checkout main
git merge gh-pages
git push origin main
```

### 3. Monitor Deployment

- Go to the "Actions" tab in your GitHub repository
- Watch the "Deploy to GitHub Pages" workflow run
- Once complete, your site will be live at: `https://anoshanj.github.io/ai-agent-management-platform/`

## Workflow Features

### Deployment Workflow (`docs-deploy.yml`)
- Triggers on pushes to `main` that affect `website/**` files
- Uses Node.js 20 with npm caching
- Builds the Docusaurus site
- Deploys to GitHub Pages using GitHub Actions artifacts
- Includes proper permissions for Pages deployment

### Test Workflow (`test-deploy.yml`)
- Runs on pull requests to `main`
- Tests the build process
- Runs security audit (`npm audit`)
- Ensures changes don't break the documentation build

## Troubleshooting

If the deployment fails:
1. Check that GitHub Pages is enabled in repository settings
2. Ensure the repository is public (or you have GitHub Pro for private repos)
3. Verify the workflows have the necessary permissions
4. Check the Actions logs for specific error messages

## Reverting to WSO2 Organization

When you're ready to deploy to the WSO2 organization, update `website/docusaurus.config.ts`:
```typescript
url: 'https://wso2.github.io',
organizationName: 'wso2',
```
