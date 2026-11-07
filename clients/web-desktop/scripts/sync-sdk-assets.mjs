import { cp, copyFile, writeFile, readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
const root = new URL("../", import.meta.url);
const pkg = new URL("node_modules/@openim/wasm-client-sdk/", root);
const records = [];
for (const name of ["openIM.wasm", "sql-wasm.wasm", "wasm_exec.js"]) {
 const source = new URL(`../../sdks/wasm/assets/${name}`, root);
 const data = await readFile(source);
 await copyFile(source, new URL(`public/${name}`, root));
 records.push({file:name,sha256:createHash("sha256").update(data).digest("hex"),size:data.length});
}
await writeFile(new URL("public/sdk-assets.json", root), JSON.stringify({sdk:"3.8.3-patch.15.2",assets:records},null,2));

await cp(new URL("extraResources/twemoji/",root),new URL("public/twemoji/",root),{recursive:true});
