import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { build } from "esbuild";
const brand = JSON.parse(await readFile("../brand.json", "utf8"));
const result = await build({ entryPoints: ["src/config/brand.ts"], bundle: true, write: false, platform: "node", format: "esm" });
const api = await import("data:text/javascript;base64," + Buffer.from(result.outputFiles[0].text).toString("base64"));
test("display defaults and technical package name share the cross-platform brand source", () => {
  assert.equal(api.appDisplayName(), brand.displayName); assert.equal(api.CLIENT_BRAND.technicalName, brand.technicalName);
  assert.equal(api.webDisplayTitle({ appName: "" }), brand.displayName);
});
test("runtime appName and webTitle override display text independently of package identity", () => {
  assert.equal(api.appDisplayName({ appName: "Configured Staff Chat" }), "Configured Staff Chat");
  assert.equal(api.webDisplayTitle({ appName: "Chat", webTitle: "Company Portal" }), "Company Portal");
  assert.equal(api.webDisplayTitle({ appName: "Chat", webTitle: " " }), "Chat");
  assert.equal(api.CLIENT_BRAND.technicalName, brand.technicalName);
});
