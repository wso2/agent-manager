const fs = require('fs');

/**
 * Load constants from _constants.mdx file
 */
async function loadConstants(constantsPath) {
  if (!fs.existsSync(constantsPath)) {
    return {};
  }

  const content = fs.readFileSync(constantsPath, 'utf-8');

  // Parse the exported versions object
  const match = content.match(/export\s+const\s+versions\s*=\s*\{([^}]+)\}/s);
  if (!match) return {};

  const versionsBlock = match[1];
  const constants = {};

  // Extract key-value pairs
  const dockerTag = versionsBlock.match(/dockerTag:\s*['"]([^'"]+)['"]/);
  const githubRef = versionsBlock.match(/githubRef:\s*['"]([^'"]+)['"]/);
  const helmChart = versionsBlock.match(/helmChart:\s*['"]([^'"]+)['"]/);
  const helmSource = versionsBlock.match(/helmSource:\s*['"]([^'"]+)['"]/);

  if (dockerTag) constants.dockerTag = dockerTag[1];
  if (githubRef) constants.githubRef = githubRef[1];
  if (helmChart) constants.helmChart = helmChart[1];
  if (helmSource) constants.helmSource = helmSource[1];

  return constants;
}

/**
 * Process MDX content and convert to clean markdown
 */
async function processMarkdownFile(content, constants, sourceDir) {
  let result = content;

  // Step 1: Remove import statements
  result = removeImports(result);

  // Step 2: Remove frontmatter (will be added back with minimal info)
  const { frontmatter, body } = extractFrontmatter(result);
  result = body;

  // Step 3: Process CodeBlock components
  result = processCodeBlocks(result, constants);

  // Step 4: Process step-card divs into proper markdown headings
  result = processStepCards(result);

  // Step 5: Process prerequisite cards
  result = processPrerequisiteCards(result);

  // Step 6: Process content-section-highlight divs
  result = processContentSections(result);

  // Step 7: Replace version interpolations
  result = replaceVersionInterpolations(result, constants);

  // Step 8: Process image tags
  result = processImageTags(result);

  // Step 9: Process Link components
  result = processLinks(result, constants);

  // Step 10: Clean up remaining JSX/HTML artifacts
  result = cleanupArtifacts(result);

  // Step 11: Clean up excessive whitespace
  result = cleanupWhitespace(result);

  // Add back minimal frontmatter if title exists
  if (frontmatter.title) {
    result = `---\ntitle: ${frontmatter.title}\n---\n\n${result}`;
  }

  return result;
}

function extractFrontmatter(content) {
  const frontmatterMatch = content.match(/^---\n([\s\S]*?)\n---\n/);
  if (!frontmatterMatch) {
    return { frontmatter: {}, body: content };
  }

  const frontmatterBlock = frontmatterMatch[1];
  const frontmatter = {};

  // Parse simple key: value pairs
  const titleMatch = frontmatterBlock.match(/title:\s*(.+)/);
  if (titleMatch) {
    frontmatter.title = titleMatch[1].trim();
  }

  const body = content.slice(frontmatterMatch[0].length);
  return { frontmatter, body };
}

function removeImports(content) {
  // Remove all import statements (single and multi-line)
  return content.replace(/^import\s+[\s\S]*?from\s+['"][^'"]+['"];?\s*$/gm, '');
}

function processCodeBlocks(content, constants) {
  // Match <CodeBlock language="...">...</CodeBlock>
  const codeBlockRegex = /<CodeBlock\s+language="([^"]+)">\s*([\s\S]*?)\s*<\/CodeBlock>/g;

  return content.replace(codeBlockRegex, (match, language, codeContent) => {
    let code = codeContent;

    // Handle String.raw template literals
    code = code.replace(/\{String\.raw`([\s\S]*?)`\}/g, '$1');

    // Handle template literals with version interpolation
    code = code.replace(/\{`([\s\S]*?)`\}/g, '$1');

    // Replace version placeholders in template literal syntax
    code = code.replace(/\$\{versions\.dockerTag\}/g, constants.dockerTag || 'latest');
    code = code.replace(/\$\{versions\.githubRef\}/g, constants.githubRef || 'main');
    code = code.replace(/\$\{versions\.helmChart\}/g, constants.helmChart || '');
    code = code.replace(/\$\{versions\.helmSource\}/g, constants.helmSource || '');

    // Clean up the code
    code = code.trim();

    return '```' + language + '\n' + code + '\n```';
  });
}

function processStepCards(content) {
  // Process step-card divs with step headers
  // This handles the nested div structure used for step cards

  // Pattern for step cards with numbered headers
  const stepCardRegex = /<div\s+className="step-card">\s*<div\s+className="step-header">\s*<div\s+className="step-header-icon">(\d+)<\/div>\s*<div\s+className="step-header-text">\s*([\s\S]*?)\s*<\/div>\s*<\/div>([\s\S]*?)<\/div>(?=\s*(?:<div\s+className="|$|\n\n[^<]|<div\s+className="content-section))/g;

  let result = content.replace(stepCardRegex, (match, number, title, innerContent) => {
    const cleanTitle = title.trim();
    const cleanContent = innerContent.trim();
    return `## ${number}. ${cleanTitle}\n\n${cleanContent}\n\n`;
  });

  return result;
}

function processPrerequisiteCards(content) {
  // Process prerequisite cards (step-card with prereq-header-text)
  const prereqRegex = /<div\s+className="step-card">\s*<div\s+className="prereq-header-text">([^<]*)<\/div>([\s\S]*?)<\/div>(?=\s*(?:<div\s+className="|$|\n\n[^<]))/g;

  return content.replace(prereqRegex, (match, header, innerContent) => {
    // Clean header - remove emojis and extra whitespace
    const cleanHeader = header.replace(/[^\w\s]/g, '').trim();
    const cleanContent = innerContent.trim();
    return `## ${cleanHeader}\n\n${cleanContent}\n\n`;
  });
}

function processContentSections(content) {
  // Remove content-section-highlight wrapper divs, keeping inner content
  // Use a loop to handle nested structures
  let result = content;
  let previousResult = '';

  while (result !== previousResult) {
    previousResult = result;
    result = result.replace(
      /<div\s+className="content-section-highlight">([\s\S]*?)<\/div>(?=\s*(?:<div\s+className="(?:step-card|content-section)"|$|\n\n[^<]|You have now))/g,
      (match, innerContent) => innerContent.trim() + '\n\n'
    );
  }

  return result;
}

function replaceVersionInterpolations(content, constants) {
  let result = content;

  // Replace {versions.xxx} patterns (JSX interpolation)
  result = result.replace(/\{versions\.dockerTag\}/g, constants.dockerTag || 'latest');
  result = result.replace(/\{versions\.githubRef\}/g, constants.githubRef || 'main');
  result = result.replace(/\{versions\.helmChart\}/g, constants.helmChart || '');
  result = result.replace(/\{versions\.helmSource\}/g, constants.helmSource || '');

  return result;
}

function processImageTags(content) {
  // Convert <img src={require(...).default} /> to markdown images
  // Handle various attribute orders and optional attributes

  let result = content;

  // Pattern 1: src first, then alt
  result = result.replace(
    /<img\s+src=\{require\(['"]([^'"]+)['"]\)\.default\}\s+alt="([^"]*)"\s*[^>]*\/?>/g,
    (match, imgPath, alt) => `![${alt}](${imgPath})`
  );

  // Pattern 2: alt first (or other attributes), then src
  result = result.replace(
    /<img\s+[^>]*src=\{require\(['"]([^'"]+)['"]\)\.default\}[^>]*alt="([^"]*)"[^>]*\/?>/g,
    (match, imgPath, alt) => `![${alt}](${imgPath})`
  );

  // Pattern 3: Only src, no alt
  result = result.replace(
    /<img\s+src=\{require\(['"]([^'"]+)['"]\)\.default\}[^>]*\/?>/g,
    (match, imgPath) => `![](${imgPath})`
  );

  return result;
}

function processLinks(content, constants) {
  let result = content;

  // Convert <Link to={`...`}>text</Link> to markdown links (template literals)
  result = result.replace(
    /<Link\s+to=\{`([^`]+)`\}>([^<]+)<\/Link>/g,
    (match, url, text) => {
      let resolvedUrl = url;
      resolvedUrl = resolvedUrl.replace(/\$\{versions\.githubRef\}/g, constants.githubRef || 'main');
      resolvedUrl = resolvedUrl.replace(/\$\{versions\.dockerTag\}/g, constants.dockerTag || 'latest');
      return `[${text}](${resolvedUrl})`;
    }
  );

  // Convert <Link to="...">text</Link> to markdown links (static strings)
  result = result.replace(/<Link\s+to="([^"]+)">([^<]+)<\/Link>/g, '[$2]($1)');

  return result;
}

function cleanupArtifacts(content) {
  let result = content;

  // Remove remaining div tags with classNames (iteratively to handle nesting)
  let previousResult = '';
  while (result !== previousResult) {
    previousResult = result;
    // Remove opening tags
    result = result.replace(/<div\s+className="[^"]*">\s*/g, '');
    // Remove closing tags
    result = result.replace(/<\/div>/g, '');
  }

  // Remove JSX-style className attributes from any remaining tags
  result = result.replace(/\s+className="[^"]*"/g, '');

  // Remove width/style attributes from remaining tags
  result = result.replace(/\s+width="[^"]*"/g, '');
  result = result.replace(/\s+style=\{\{[^}]*\}\}/g, '');

  // Clean up empty JSX expressions
  result = result.replace(/\{['"][^'"]*['"]\}/g, '');

  return result;
}

function cleanupWhitespace(content) {
  let result = content;

  // Remove excessive blank lines (more than 2 consecutive)
  result = result.replace(/\n{4,}/g, '\n\n\n');

  // Remove trailing whitespace from lines
  result = result.replace(/[ \t]+$/gm, '');

  // Trim leading/trailing whitespace
  result = result.trim();

  // Ensure file ends with single newline
  result += '\n';

  return result;
}

module.exports = {
  loadConstants,
  processMarkdownFile,
};
