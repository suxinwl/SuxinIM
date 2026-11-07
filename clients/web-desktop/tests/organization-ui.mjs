// Offline browser check: only synthetic directory data, no production requests.
import assert from "node:assert/strict";
import { build } from "esbuild";
import { compile } from "sass";
import { chromium } from "playwright";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
const cssPath = resolve("src/pages/organization/organization.module.scss");
const classes = Object.fromEntries(["organization", "departments", "people", "toolbar", "hint", "person", "name", "pagination"].map(value => [value, value]));
const result = await build({ stdin: { contents: `import React from 'react';import{createRoot}from'react-dom/client';import{Organization}from'./src/pages/organization/index';createRoot(document.getElementById('root')).render(<Organization/>);`, resolveDir: process.cwd(), loader: "tsx" }, bundle: true, write: false, format: "iife", jsx: "automatic", define: { "process.env.NODE_ENV": '"production"' }, plugins: [{ name: "offline-organization", setup(b) {
  b.onResolve({ filter: /^@\/api\/directory$/ }, () => ({ path: "directory", namespace: "fixture" }));
  b.onResolve({ filter: /^@\/components\/OIMAvatar$/ }, () => ({ path: "avatar", namespace: "fixture" }));
  b.onResolve({ filter: /^@\/utils\/events$/ }, () => ({ path: "events", namespace: "fixture" }));
  b.onResolve({ filter: /organization\.module\.scss$/ }, () => ({ path: "style", namespace: "fixture" }));
  b.onLoad({ filter: /.*/, namespace: "fixture" }, args => ({ resolveDir: process.cwd(), loader: "jsx", contents: args.path === "directory"
    ? `export{departmentTree}from${JSON.stringify(resolve("src/api/directory.ts"))};export const getDirectory=(query)=>window.fixtureDirectory(query);`
    : args.path === "avatar" ? `import React from'react';export default function Avatar(){return <span aria-hidden="true">●</span>;}`
    : args.path === "events" ? `export default{emit:(name,data)=>window.fixtureEvents.push({name,data})};`
    : `const style=document.createElement('style');style.textContent=${JSON.stringify(compile(cssPath).css.replace(/:global\(([^)]+)\)/g, '$1'))};document.head.append(style);export default ${JSON.stringify(classes)};` }));
  // The pure tree helper shares a module with authenticated transport. Do not
  // pull account storage/network into this isolated component harness.
  b.onResolve({ filter: /^@\/utils\/storage$/ }, () => ({ path: "storage", namespace: "empty" }));
  b.onResolve({ filter: /^\.\/sso$/ }, () => ({ path: "sso", namespace: "empty" }));
  b.onLoad({ filter: /.*/, namespace: "empty" }, args => ({ contents: args.path === "storage" ? "export const getSSOSession=()=>null;" : "export const ssoRequest=()=>{};export class SSOError extends Error{};" }));
} }] });
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE });
const output = "test-results/organization-candidate";
await mkdir(output, { recursive: true });
const checks = [];
try {
  const page = await browser.newPage({ viewport: { width: 1180, height: 760 } });
  await page.route("**/*", route => route.abort());
  const errors = []; page.on("pageerror", error => errors.push(error.message));
  await page.setContent('<html><head></head><body style="margin:0;font-family:Arial,\'Microsoft YaHei\',sans-serif"><div id="root" style="height:100vh"></div></body></html>');
  await page.evaluate(() => {
    window.fixtureEvents = [];
    window.fixtureQueries = [];
    const departments = [{ id: "1", parentID: "", name: "速信总部" }, { id: "2", parentID: "1", name: "研发部" }, { id: "3", parentID: "1", name: "人事部" }];
    const people = [{ userID: "person-a", nickname: "陈明", employeeNo: "1001", department: "研发部", departmentID: "2", position: "工程师", phoneNumber: "13800000001" }, { userID: "person-b", nickname: "陈明", employeeNo: "1002", department: "人事部", departmentID: "3", position: "专员", phoneNumber: "13800000002" }];
    window.fixtureDirectory = async query => {
      window.fixtureQueries.push(query);
      if (query.keyword === "故障") throw new Error("synthetic unavailable");
      const items = people.filter(person => (!query.departmentID || person.departmentID === query.departmentID) && (!query.keyword || person.nickname.includes(query.keyword) || person.employeeNo.includes(query.keyword) || person.phoneNumber.includes(query.keyword)));
      return { items, total: items.length, departments };
    };
  });
  await page.addScriptTag({ content: result.outputFiles[0].text });
  await page.getByRole("columnheader", { name: "工号", exact: true }).waitFor();
  await page.getByText("1002", { exact: true }).waitFor();
  checks.push("initial organization tree and personnel table");
  await page.locator(".ant-tree-title").getByText("研发部", { exact: true }).click();
  await page.getByText("1001", { exact: true }).waitFor();
  await page.waitForFunction(() => !document.body.innerText.includes("1002"));
  checks.push("department selection restricts the right-hand personnel list");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").fill("陈明");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").press("Enter");
  await page.getByText("1002", { exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "陈明", exact: true }).count(), 2);
  checks.push("same-name search retains multiple employees and their numbers");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").fill("1002");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").press("Enter");
  await page.waitForFunction(() => !document.body.innerText.includes("1001"));
  await page.getByRole("button", { name: "陈明", exact: true }).click();
  assert.deepEqual(await page.evaluate(() => window.fixtureEvents), [{ name: "OPEN_USER_CARD", data: { userID: "person-b" } }]);
  checks.push("employee-number search opens the existing profile card for the exact user");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").fill("13800000002");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").press("Enter");
  await page.waitForFunction(() => window.fixtureQueries.at(-1).keyword === "13800000002");
  await page.getByText("1002", { exact: true }).waitFor();
  checks.push("phone search resolves the matching employee without exposing a phone field");
  await page.screenshot({ path: `${output}/desktop.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: `${output}/mobile-web.png` });
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").fill("无匹配");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").press("Enter");
  await page.getByText("没有找到匹配手机号、姓名或工号的同事", { exact: true }).waitFor();
  checks.push("empty search state");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").fill("故障");
  await page.getByPlaceholder("搜索员工手机号、姓名或工号").press("Enter");
  await page.getByText("组织通讯录暂不可用，请重试", { exact: true }).waitFor();
  checks.push("request failure remains an error with retry, not an empty directory");
  assert.deepEqual(errors, []);
  await writeFile(`${output}/offline-ui.json`, JSON.stringify({ passed: true, scope: "isolated synthetic UI; no production requests", checks, pageErrors: errors }, null, 2));
  console.log(`Organization offline browser: ${checks.length} checks passed`);
} finally { await browser.close(); }
