#!/usr/bin/env node
/**
 * Generate docs/reference/helm-charts/*.mdx from each chart's values.schema.json.
 *
 * The schema is the single source of truth: `deployments/helm-charts/tools/`
 * generates it from values.yaml, and this script renders it as documentation.
 *
 *   node scripts/gen-helm-reference.mjs           # write pages
 *   node scripts/gen-helm-reference.mjs --check   # non-zero exit if stale
 */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..", "..");
const CHARTS = path.join(ROOT, "deployments", "helm-charts");
const OUT = path.join(import.meta.dirname, "..", "docs", "reference", "helm-charts");
const CHECK = process.argv.includes("--check");

const TITLES = {
  "wso2-agent-manager": "Agent Manager",
  "wso2-amp-api-platform-gateway-extension": "API Platform Gateway Extension",
  "wso2-amp-thunder-extension": "Thunder Extension",
  "wso2-amp-evaluation-extension": "Evaluation Extension",
  "wso2-amp-observability-extension": "Observability Extension",
  "wso2-amp-platform-resources-extension": "Platform Resources Extension",
};

// MDX parses <foo> as JSX and {foo} as an expression, and | breaks table cells.
const cell = (s = "") =>
  String(s).replace(/</g, "&lt;").replace(/>/g, "&gt;")
           .replace(/\{/g, "&#123;").replace(/\}/g, "&#125;")
           .replace(/\|/g, "\\|");

const fmtDefault = (node) => {
  if (node.type === "object" && !("default" in node)) return "";
  if (!("default" in node)) return "";
  const d = node.default;
  if (Array.isArray(d)) return d.length ? "see `values.yaml`" : "`[]`";
  if (d && typeof d === "object") return "`{}`";
  return "`" + JSON.stringify(d) + "`";
};

function flatten(node, prefix = "", out = []) {
  for (const [key, value] of Object.entries(node.properties ?? {})) {
    const dotted = prefix ? `${prefix}.${key}` : key;
    out.push({ dotted, type: value.type, description: value.description ?? "", def: fmtDefault(value) });
    flatten(value, dotted, out);
  }
  return out;
}

const chartDescription = (name) => {
  const txt = fs.readFileSync(path.join(CHARTS, name, "Chart.yaml"), "utf8");
  return (txt.match(/^description:\s*(.+)$/m)?.[1] ?? "").trim();
};

let stale = [];
const index = [];

for (const name of Object.keys(TITLES)) {
  const schemaPath = path.join(CHARTS, name, "values.schema.json");
  if (!fs.existsSync(schemaPath)) continue;
  const schema = JSON.parse(fs.readFileSync(schemaPath, "utf8"));
  const rows = flatten(schema);

  // Group by top-level key, or by second level when the chart has a single root.
  const roots = new Set(rows.map((r) => r.dotted.split(".")[0]));
  const depth = roots.size === 1 ? 2 : 1;
  const groups = new Map();
  for (const r of rows) {
    const parts = r.dotted.split(".");
    const key = parts.slice(0, depth).join(".");
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(r);
  }

  const body = [
    "---", `title: ${TITLES[name]}`, "---", "",
    `# ${TITLES[name]}`, "",
    chartDescription(name), "",
    "```bash",
    `helm install ${name.replace("wso2-", "")} oci://ghcr.io/wso2/helm-charts/${name} \\`,
    "  --namespace <namespace> --create-namespace \\",
    "  --values my-values.yaml",
    "```", "",
  ];
  for (const key of [...groups.keys()].sort()) {
    body.push(`## ${key}`, "");
    body.push("| Parameter | Description | Type | Default |", "|---|---|---|---|");
    for (const r of groups.get(key)) {
      body.push(`| \`${r.dotted}\` | ${cell(r.description)} | ${r.type ?? ""} | ${r.def} |`);
    }
    body.push("");
  }
  const text = body.join("\n") + "\n";
  const target = path.join(OUT, `${name}.mdx`);
  if (CHECK) {
    if (!fs.existsSync(target) || fs.readFileSync(target, "utf8") !== text) stale.push(name);
  } else {
    fs.mkdirSync(OUT, { recursive: true });
    fs.writeFileSync(target, text);
  }
  index.push({ name, title: TITLES[name], desc: chartDescription(name), count: rows.length });
  console.log(`${name.padEnd(46)} ${String(rows.length).padStart(4)} parameters, ${groups.size} groups`);
}

const idx = [
  "---", "title: Helm Charts", "---", "",
  "# Helm Charts", "",
  "WSO2 Agent Manager is installed as a set of Helm charts: one core chart plus five",
  "extension charts that add gateway, identity, evaluation, observability, and platform",
  "resources on top of OpenChoreo. Each page below is the values reference for one chart,",
  "generated from that chart's `values.schema.json`.", "",
  "| Chart | Purpose | Parameters |", "|---|---|---|",
  ...index.map((c) => `| [${c.title}](./${c.name}.mdx) | ${cell(c.desc)} | ${c.count} |`),
  "",
  "Because the pages are generated from the schema, `helm lint` and this reference cannot",
  "disagree: a value the schema rejects is a value documented here as the wrong type.", "",
  "For installation walkthroughs rather than value tables, see the",
  "[Installation guides](../../guides/on-k3d.mdx).", "",
].join("\n");
const idxPath = path.join(OUT, "index.mdx");
if (CHECK) {
  if (!fs.existsSync(idxPath) || fs.readFileSync(idxPath, "utf8") !== idx) stale.push("index");
} else {
  fs.writeFileSync(idxPath, idx);
}

if (CHECK && stale.length) {
  console.error(`\nOut of date: ${stale.join(", ")}\nRun: node scripts/gen-helm-reference.mjs`);
  process.exit(1);
}
console.log(CHECK ? "\nAll helm reference pages are up to date." : "\nWrote helm chart reference pages.");
