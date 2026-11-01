import { build } from "esbuild";
import "./ensure-screenshot-preload.mjs";
await build({entryPoints:["electron/main/index.ts","electron/preload/index.ts"],outdir:"dist-electron",outbase:"electron",platform:"node",target:"node22",bundle:true,format:"cjs",packages:"external",sourcemap:true});
