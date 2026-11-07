import { build } from "esbuild";
import { mkdir, copyFile, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve, relative, isAbsolute, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";

const source = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const args = process.argv.slice(2);
if (args.length && !((args.length === 2 || args.length === 4) && args[0] === "--outdir" && (args.length === 2 || args[2] === "--source-overrides"))) {
  throw new Error("Usage: node scripts/build.mjs [--outdir <release-directory> [--source-overrides <reviewed-json>]]");
}
const overrides = new Map();
if (args.length === 4) {
  const records = JSON.parse((await readFile(resolve(args[3]), "utf8")).replace(/^\uFEFF/, ""));
  for (const [name, item] of Object.entries(records)) {
    if (!["assets/openim.js", "assets/openim-access.js"].includes(name)) throw Error("Only reviewed R6 IM frontend modules may be overridden");
    const contents = await readFile(item.path, "utf8");
    if (createHash("sha256").update(contents).digest("hex") !== item.sha256) throw Error("Compatibility source hash mismatch: " + name);
    overrides.set(resolve(source, name), contents);
  }
}
const livekitExports = "export { Room, RoomEvent } from 'livekit-client';";
const common = { absWorkingDir: source, bundle: true, format: "esm", target: "es2022" };

if (args.length) {
  const output = resolve(args[1]);
  const insideSource = relative(source, output);
  if (!insideSource || (!insideSource.startsWith(`..${sep}`) && insideSource !== ".." && !isAbsolute(insideSource))) {
    throw new Error("Release output must be outside the frontend source directory");
  }
  const result = await build({
    ...common,
    entryPoints: { app: "assets/app.js", style: "assets/style.css" },
    outdir: output,
    entryNames: "assets/[name]-[hash]",
    chunkNames: "assets/chunks/[name]-[hash]",
    assetNames: "assets/[name]-[hash]",
    splitting: true,
    minify: true,
    metafile: true,
    legalComments: "eof",
    // Keep calls lazy while letting esbuild version the complete module graph.
    plugins: [{name:"reviewed-compatibility-sources", setup(builder) {
      builder.onLoad({filter:/[/\\]assets[/\\](openim|openim-access)\.js$/}, args => overrides.has(args.path) ? {contents:overrides.get(args.path),resolveDir:dirname(args.path),loader:"js"}:undefined);
    }}, {
      name: "release-livekit",
      setup(builder) {
        builder.onResolve({ filter: /^\.\/livekit\.js$/ }, () => ({ path: "livekit", namespace: "release" }));
        builder.onLoad({ filter: /.*/, namespace: "release" }, () => ({ contents: livekitExports, resolveDir: source, loader: "js" }));
      },
    }],
  });
  const entryURL = (entry) => {
    const file = Object.entries(result.metafile.outputs).find(([, info]) => info.entryPoint === entry)?.[0];
    if (!file) throw new Error(`Missing release entry: ${entry}`);
    return "/plugins/biz-suite/admin/" + relative(output, resolve(source, file)).split(sep).join("/");
  };
  let html = await readFile(resolve(source, "index.html"), "utf8");
  for (const entry of ["assets/app.js", "assets/style.css"]) {
    const original = `/plugins/biz-suite/admin/${entry}`;
    if (!html.includes(original)) throw new Error(`Missing HTML asset reference: ${entry}`);
    html = html.replaceAll(original, entryURL(entry));
  }
  await writeFile(resolve(output, "index.html"), html);
  await copyFile(resolve(source, "node_modules/livekit-client/LICENSE"), resolve(output, "assets/LIVEKIT-LICENSE.txt"));
  console.log("Business SPA release ready; JavaScript, lazy chunks and CSS use content hashes.");
} else {
  await mkdir(resolve(source, "assets"), { recursive: true });
  await build({ ...common, entryPoints: ["assets/app.js"], write: false, external: ["./livekit.js"] });
  await build({
    ...common,
    stdin: { contents: livekitExports, resolveDir: source },
    outfile: "assets/livekit.js",
    minify: true,
    legalComments: "eof",
  });
  await copyFile(resolve(source, "node_modules/livekit-client/LICENSE"), resolve(source, "assets/LIVEKIT-LICENSE.txt"));
  console.log("Business SPA ready; local LiveKit SDK bundled.");
}
