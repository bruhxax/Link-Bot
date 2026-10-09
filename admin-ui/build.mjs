import { build } from "esbuild";
import postcss from "postcss";
import { readFile, writeFile, readdir } from "node:fs/promises";
import { zipSync } from "fflate";
import { fileURLToPath } from "node:url";
import path from "node:path";
const root = path.dirname(fileURLToPath(import.meta.url));
const out = path.resolve(root, "../internal/miniapp/static/admin-ui");
await build({
  absWorkingDir: root,
  entryPoints: ["main.jsx"],
  bundle: true,
  format: "esm",
  minify: true,
  target: ["es2022"],
  outfile: out + ".mjs",
  loader: { ".woff2": "file", ".woff": "file" },
  assetNames: "assets/admin-[name]-[hash]",
  define: { "process.env.NODE_ENV": '"production"' },
  legalComments: "eof",
});
// Keep generated license comments clean when the bundle is committed.
await writeFile(
  out + ".mjs",
  (await readFile(out + ".mjs", "utf8")).replace(/[\t ]+$/gm, ""),
);
const css = postcss.parse(await readFile(out + ".css", "utf8"));
// Cabinet editors use 7–11px labels. Keep their markup and behavior, but give
// captions the same 12px minimum as Remnawave. These rules are scoped below,
// so the original cabinet/administration retains its existing typography.
const cabinetStyles = postcss.parse(
  await readFile(path.join(path.dirname(out), "styles.css"), "utf8"),
);
const captionSelectors = new Set();
cabinetStyles.walkRules((rule) => {
  if (
    !rule.nodes.some(
      (node) =>
        node.prop === "font-size" &&
        /^\d+(\.\d+)?px$/.test(node.value) &&
        parseFloat(node.value) < 12,
    )
  )
    return;
  rule.selectors
    .filter((selector) =>
      /\.(admin-|support-|custom-bg-|modal__)/.test(selector),
    )
    .forEach((selector) => captionSelectors.add(selector));
});
css.append(
  postcss.rule({
    selectors: [...captionSelectors],
    nodes: [postcss.decl({ prop: "font-size", value: "12px" })],
  }),
);
// Mantine's reset and tokens belong only to the administration surface.
css.walkRules((rule) => {
  for (let ancestor = rule.parent; ancestor; ancestor = ancestor.parent) {
    if (ancestor.type === "atrule" && /keyframes$/.test(ancestor.name)) return;
  }
  const scope = 'html[data-admin-console="on"]';
  rule.selectors = rule.selectors.map((selector) => {
    if (selector.startsWith(":host("))
      return selector.replace(/^:host\(([^)]+)\)/, `${scope}$1`);
    if (selector.startsWith(":where([data-mantine-color-scheme"))
      return scope + selector;
    const rootSelector = /^(?:html\b|:root\b|:host\b)/;
    if (rootSelector.test(selector))
      return selector.replace(rootSelector, scope);
    if (selector.startsWith("[data-mantine-color-scheme="))
      return scope + selector;
    return `${scope} ${selector}`;
  });
});
await writeFile(out + ".css", css.toString());
// Reuse the same chart in the cabinet without loading the administration shell.
await build({
  absWorkingDir: root,
  entryPoints: ["finance-standalone.jsx"],
  bundle: true,
  format: "esm",
  minify: true,
  target: ["es2022"],
  outfile: path.resolve(path.dirname(out), "finance-ui.mjs"),
  define: { "process.env.NODE_ENV": '"production"' },
  legalComments: "eof",
});
const financeBundle = path.resolve(path.dirname(out), "finance-ui.mjs");
await writeFile(
  financeBundle,
  (await readFile(financeBundle, "utf8")).replace(/[\t ]+$/gm, ""),
);
// Keep the corresponding frontend source available with every shipped build.
const archive = {};
for (const filename of await readdir(root)) {
  if (
    /\.(jsx|mjs|css|json|md)$/.test(filename) ||
    filename.startsWith("LICENSE")
  ) {
    archive[`admin-ui/${filename}`] = new Uint8Array(
      await readFile(path.join(root, filename)),
    );
  }
}
for (const filename of await readdir(path.dirname(out))) {
  if (
    /\.(js|mjs|css|html)$/.test(filename) &&
    !/^(admin|finance)-ui\.(mjs|css)$/.test(filename)
  ) {
    archive[`internal/miniapp/static/${filename}`] = new Uint8Array(
      await readFile(path.join(path.dirname(out), filename)),
    );
  }
}
for (const font of ["fira-mono", "unbounded"]) {
  const license = await readFile(
    path.join(root, "node_modules/@fontsource", font, "LICENSE"),
  );
  await writeFile(
    path.join(path.dirname(out), "assets", `admin-LICENSE-${font}.txt`),
    license,
  );
  archive[`admin-ui/LICENSE-${font}`] = new Uint8Array(license);
}
await writeFile(
  path.join(path.dirname(out), "admin-ui-source.zip"),
  zipSync(archive, { level: 9 }),
);
console.log("Built isolated administration UI");
