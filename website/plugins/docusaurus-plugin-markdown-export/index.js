const fs = require('fs');
const path = require('path');
const { processMarkdownFile, loadConstants } = require('./mdxProcessor');

module.exports = function pluginMarkdownExport(context, options = {}) {
  const { siteDir } = context;

  // Auto-detect versions from versions.json (no manual updates needed)
  function getVersionConfig() {
    const versionsPath = path.join(siteDir, 'versions.json');
    let versions = [];

    if (fs.existsSync(versionsPath)) {
      try {
        versions = JSON.parse(fs.readFileSync(versionsPath, 'utf-8'));
      } catch (e) {
        console.warn('[markdown-export] Could not parse versions.json:', e.message);
      }
    }

    // Build VERSION_DIRS dynamically
    // 'current' always maps to docs/ (for /docs/next/)
    const VERSION_DIRS = {
      'current': 'docs',
    };

    // Add versioned docs from versions.json
    for (const version of versions) {
      VERSION_DIRS[version] = `versioned_docs/version-${version}`;
    }

    // First version in versions.json is the lastVersion (no URL prefix)
    const LAST_VERSION = versions[0] || null;

    return { VERSION_DIRS, LAST_VERSION };
  }

  const { VERSION_DIRS, LAST_VERSION } = getVersionConfig();

  // Get URL prefix for a version
  function getVersionPrefix(versionName) {
    if (versionName === 'current') return 'next/';
    if (versionName === LAST_VERSION) return '';  // lastVersion has no prefix
    return `${versionName}/`;
  }

  // Check if file has slug: / in frontmatter (root page)
  function hasRootSlug(content) {
    const frontmatterMatch = content.match(/^---\n([\s\S]*?)\n---/);
    if (!frontmatterMatch) return false;
    return /^slug:\s*\/\s*$/m.test(frontmatterMatch[1]);
  }

  // Recursively find all .md and .mdx files
  function findMarkdownFiles(dir, baseDir = dir) {
    const files = [];
    if (!fs.existsSync(dir)) return files;

    const entries = fs.readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        files.push(...findMarkdownFiles(fullPath, baseDir));
      } else if (entry.isFile() && /\.(md|mdx)$/.test(entry.name)) {
        // Skip _constants.mdx and category files
        if (entry.name.startsWith('_') || entry.name === 'category.json') continue;

        const relativePath = path.relative(baseDir, fullPath);
        // Convert to slug (remove extension, use forward slashes)
        const slug = relativePath.replace(/\.(md|mdx)$/, '').split(path.sep).join('/');
        files.push({ fullPath, slug });
      }
    }
    return files;
  }

  // Cache for constants to avoid reloading on every file change
  const constantsCache = new Map();

  // Get version info from a file path
  function getVersionFromPath(filePath) {
    const relativePath = path.relative(siteDir, filePath);

    // Check if it's in docs/ (current version)
    if (relativePath.startsWith('docs' + path.sep)) {
      return { versionName: 'current', docsDir: path.join(siteDir, 'docs') };
    }

    // Check versioned_docs
    for (const [versionName, versionDir] of Object.entries(VERSION_DIRS)) {
      if (versionName === 'current') continue;
      if (relativePath.startsWith(versionDir.replace(/\//g, path.sep))) {
        return { versionName, docsDir: path.join(siteDir, versionDir) };
      }
    }
    return null;
  }

  // Export a single markdown file (used by watcher)
  async function exportSingleFile(filePath, outputBaseDir) {
    // Skip non-markdown files and special files
    if (!/\.(md|mdx)$/.test(filePath)) return false;
    const fileName = path.basename(filePath);
    if (fileName.startsWith('_') || fileName === 'category.json') return false;

    const versionInfo = getVersionFromPath(filePath);
    if (!versionInfo) return false;

    const { versionName, docsDir } = versionInfo;

    // Get or load constants (cached)
    let constants = constantsCache.get(versionName);
    if (!constants) {
      const constantsPath = path.join(docsDir, '_constants.mdx');
      constants = await loadConstants(constantsPath);
      constantsCache.set(versionName, constants);
    }

    // Calculate output path
    const relativePath = path.relative(docsDir, filePath);
    const slug = relativePath.replace(/\.(md|mdx)$/, '').split(path.sep).join('/');
    const versionPrefix = getVersionPrefix(versionName);
    const outputPath = path.join(outputBaseDir, 'docs', versionPrefix, slug + '.md');

    try {
      const sourceContent = fs.readFileSync(filePath, 'utf-8');
      const cleanMarkdown = await processMarkdownFile(
        sourceContent,
        constants,
        path.dirname(filePath)
      );

      fs.mkdirSync(path.dirname(outputPath), { recursive: true });
      fs.writeFileSync(outputPath, cleanMarkdown);

      // If this file has slug: /, also export it as the version root
      // e.g., /docs/next.md for /docs/next/, or /docs.md for /docs/
      if (hasRootSlug(sourceContent)) {
        const rootPath = versionPrefix
          ? path.join(outputBaseDir, 'docs', versionPrefix.replace(/\/$/, '') + '.md')
          : path.join(outputBaseDir, 'docs.md');
        fs.mkdirSync(path.dirname(rootPath), { recursive: true });
        fs.writeFileSync(rootPath, cleanMarkdown);
      }

      return true;
    } catch (error) {
      console.error(`[markdown-export] Error processing ${filePath}:`, error.message);
      return false;
    }
  }

  // Export markdown files for all versions
  async function exportAllVersions(outputBaseDir) {
    let totalExported = 0;

    for (const [versionName, versionDir] of Object.entries(VERSION_DIRS)) {
      const docsDir = path.join(siteDir, versionDir);
      if (!fs.existsSync(docsDir)) {
        console.log(`[markdown-export] Skipping ${versionName}: ${versionDir} not found`);
        continue;
      }

      // Load constants for this version
      const constantsPath = path.join(docsDir, '_constants.mdx');
      const constants = await loadConstants(constantsPath);
      constantsCache.set(versionName, constants);

      // Find all markdown files
      const files = findMarkdownFiles(docsDir);
      console.log(`[markdown-export] Processing ${versionName}: ${files.length} docs`);

      const versionPrefix = getVersionPrefix(versionName);

      for (const { fullPath, slug } of files) {
        const outputPath = path.join(outputBaseDir, 'docs', versionPrefix, slug + '.md');

        try {
          const sourceContent = fs.readFileSync(fullPath, 'utf-8');
          const cleanMarkdown = await processMarkdownFile(
            sourceContent,
            constants,
            path.dirname(fullPath)
          );

          fs.mkdirSync(path.dirname(outputPath), { recursive: true });
          fs.writeFileSync(outputPath, cleanMarkdown);
          totalExported++;

          // If this file has slug: /, also export it as the version root
          // e.g., /docs/next.md for /docs/next/, or /docs.md for /docs/
          if (hasRootSlug(sourceContent)) {
            const rootPath = versionPrefix
              ? path.join(outputBaseDir, 'docs', versionPrefix.replace(/\/$/, '') + '.md')
              : path.join(outputBaseDir, 'docs.md');
            fs.mkdirSync(path.dirname(rootPath), { recursive: true });
            fs.writeFileSync(rootPath, cleanMarkdown);
          }
        } catch (error) {
          console.error(`[markdown-export] Error processing ${fullPath}:`, error.message);
        }
      }
    }

    return totalExported;
  }

  // Track if watcher is already set up
  let watcherInitialized = false;

  // Simple debounce function
  function debounce(fn, delay) {
    let timeoutId;
    return (...args) => {
      clearTimeout(timeoutId);
      timeoutId = setTimeout(() => fn(...args), delay);
    };
  }

  // Setup file watcher for dev mode (runs async, doesn't block hot reload)
  function setupDevWatcher(outputBaseDir) {
    if (watcherInitialized) return;
    watcherInitialized = true;

    // Use chokidar (already a Docusaurus dependency)
    let chokidar;
    try {
      chokidar = require('chokidar');
    } catch (e) {
      console.warn('[markdown-export] chokidar not available, file watching disabled');
      return;
    }

    // Build watch paths
    const watchPaths = Object.values(VERSION_DIRS).map(dir =>
      path.join(siteDir, dir, '**/*.{md,mdx}')
    );

    // Create debounced handler (300ms) - processes only the changed file
    const handleFileChange = debounce(async (filePath) => {
      const success = await exportSingleFile(filePath, outputBaseDir);
      if (success) {
        const relativePath = path.relative(siteDir, filePath);
        console.log(`[markdown-export] Updated: ${relativePath}`);
      }
    }, 300);

    // Start watcher with minimal overhead
    const watcher = chokidar.watch(watchPaths, {
      ignoreInitial: true, // Don't process existing files (already done)
      ignored: [
        /(^|[\/\\])_/, // Ignore files starting with _
        /node_modules/,
      ],
      persistent: true,
      awaitWriteFinish: {
        stabilityThreshold: 100,
        pollInterval: 50,
      },
    });

    watcher
      .on('change', handleFileChange)
      .on('add', handleFileChange);

    console.log('[markdown-export] File watcher started for dev mode');
  }

  return {
    name: 'docusaurus-plugin-markdown-export',

    // Don't generate during dev to avoid conflicts with Docusaurus versioning
    // Files are generated only during build via postBuild

    // Generate during build to ensure files are in the build output
    async postBuild({ outDir }) {
      console.log('[markdown-export] Generating markdown files for build...');

      const totalExported = await exportAllVersions(outDir);

      console.log(`[markdown-export] Exported ${totalExported} markdown files to build/`);
    },
  };
};
