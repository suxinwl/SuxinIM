import { readFile, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";

// Electron 45 sandbox preloads do not provide a global Buffer. Keep the
// dependency sandboxed and send a structured-clone-safe Uint8Array instead.
const packageFile = new URL("../node_modules/electron-screenshots/package.json", import.meta.url);
const preloadFile = new URL("../node_modules/electron-screenshots/lib/preload.js", import.meta.url);
const originalHash = "c534e99a7707e1a7811e918f03f8d4b15176f08d26da768fe4e01828b328d710";
const patchedHash = "965acb7796bd2779198f8db6b3fbe9209841d1ef3e297ee9a5a768d350746bfb";
const sha256 = (value) => createHash("sha256").update(value).digest("hex");

export async function ensureScreenshotPreload() {
  const metadata = JSON.parse(await readFile(packageFile, "utf8"));
  if (metadata.version !== "0.5.28") throw new Error("Review the screenshot sandbox patch before changing electron-screenshots 0.5.28");
  const original = await readFile(preloadFile);
  const currentHash = sha256(original);
  if (currentHash === patchedHash) return;
  if (currentHash !== originalHash) throw new Error("Unexpected screenshot preload bytes; refusing to overwrite local changes");
  const patched = original.toString("utf8").replaceAll("Buffer.from(arrayBuffer)", "new Uint8Array(arrayBuffer)");
  if (sha256(patched) !== patchedHash) throw new Error("Screenshot preload patch validation failed");
  await writeFile(preloadFile, patched);
}

await ensureScreenshotPreload();
