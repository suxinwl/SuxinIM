import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
globalThis.window = { location: { origin: "https://im.szputy.com:9885" } };
globalThis.localStorage = { getItem: () => null };
const result = await build({ entryPoints: ["src/api/public-brand.ts"], bundle: true, write: false, platform: "node", format: "esm" });
const api = await import("data:text/javascript;base64," + Buffer.from(result.outputFiles[0].text).toString("base64"));
test("pre-login brand uses the existing anonymous HTTPS route and consumes display fields only", async () => {
  let seen;
  globalThis.fetch = async (url, options) => { seen = { url, ...options }; return { ok: true, json: async () => ({ errCode: 0, data: { config: { appName: " 配置名称 ", webTitle: "配置标题", administrativeSetting: "ignored" } } }) }; };
  assert.deepEqual(await api.getPublicBrand(), { appName: "配置名称", webTitle: "配置标题" });
  assert.equal(seen.url, "https://im.szputy.com:9885/chat/client_config/get");
  assert.equal(seen.credentials, "omit"); assert.equal(seen.redirect, "error"); assert.equal(seen.body, "{}");
  assert.equal(seen.headers.token, undefined); assert.equal(seen.headers.chatToken, undefined);
});
test("unavailable public config fails without manufacturing a brand or requesting administrator access", async () => {
  globalThis.fetch = async () => ({ ok: false, json: async () => ({ errCode: 1501 }) });
  await assert.rejects(api.getPublicBrand(), /品牌配置暂不可用/);
});
test("invalid display values are ignored and current LAN entry is honored", async () => {
  globalThis.window.location.origin = "https://192.168.10.11:9885";
  globalThis.fetch = async url => { assert.equal(url, "https://192.168.10.11:9885/chat/client_config/get"); return { ok: true, json: async () => ({ errCode: 0, data: { config: { appName: "Name\nBreak", webTitle: { value: "bad" }, logoURL: "ignored" } } }) }; };
  assert.deepEqual(await api.getPublicBrand(), {});
});
