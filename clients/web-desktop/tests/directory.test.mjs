import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
globalThis.directorySession = { userID: "employee-a", sessionToken: "opaque-a" };
const result = await build({ entryPoints: ["src/api/directory.ts"], bundle: true, write: false, format: "esm", platform: "node", plugins: [{ name: "directory-boundary", setup(b) {
  b.onResolve({ filter: /^@\/utils\/storage$/ }, () => ({ path: "storage", namespace: "stub" }));
  b.onResolve({ filter: /^\.\/sso$/ }, () => ({ path: "sso", namespace: "stub" }));
  b.onLoad({ filter: /.*/, namespace: "stub" }, args => ({ contents: args.path === "storage"
    ? "export const getSSOSession=async()=>globalThis.directorySession;"
    : "export class SSOError extends Error{constructor(msg,status){super(msg);this.status=status;}} export const ssoRequest=(...args)=>globalThis.directoryRequest(...args);" }));
} }] });
const api = await import("data:text/javascript;base64," + Buffer.from(result.outputFiles[0].text).toString("base64"));

test("employee search uses HRM session and preserves multiple namesakes with employee numbers", async () => {
  globalThis.directorySession = { userID: "employee-a", sessionToken: "opaque-a" };
  const people = [{ userID: "a", nickname: "陈明", employeeNo: "1001" }, { userID: "b", nickname: "陈明", employeeNo: "1002" }];
  let seen;
  globalThis.directoryRequest = async (...args) => { seen = args; return { items: people, total: 2, departments: [] }; };
  assert.deepEqual((await api.getDirectory({ keyword: " 陈明 ", page: 1, pageSize: 20 })).items, people);
  assert.deepEqual(seen, ["directory", { keyword: "陈明", departmentID: "", page: 1, pageSize: 20, sessionToken: "opaque-a" }]);
});
test("department pagination is bounded and uses the selected HRM department", async () => {
  let seen;
  globalThis.directoryRequest = async (...args) => { seen = args; return {}; };
  await api.getDirectory({ departmentID: "dept-9", page: -5, pageSize: 1000 });
  assert.equal(seen[1].departmentID, "dept-9"); assert.equal(seen[1].page, 1); assert.equal(seen[1].pageSize, 100);
});
test("missing or changed account sessions discard directory data", async () => {
  globalThis.directorySession = null;
  globalThis.directoryRequest = () => { throw new Error("must not request without HRM session"); };
  await assert.rejects(api.getDirectory(), error => error.status === 401);
  globalThis.directorySession = { userID: "employee-a", sessionToken: "opaque-a" };
  globalThis.directoryRequest = async () => { globalThis.directorySession = { userID: "employee-b", sessionToken: "opaque-b" }; return { items: [{ userID: "private" }] }; };
  await assert.rejects(api.getDirectory(), error => error.status === 409);
});
test("profile eligibility is requested from HRM and cannot be supplied by a userID prefix", async () => {
  globalThis.directorySession = { userID: "employee-a", sessionToken: "opaque-a" };
  let seen;
  globalThis.directoryRequest = async (...args) => { seen = args; return { userID: "emp_any", canDirectChat: false }; };
  assert.equal((await api.getDirectoryPerson("emp_any")).canDirectChat, false);
  assert.deepEqual(seen, ["person", { userID: "emp_any", sessionToken: "opaque-a" }]);
});
test("department trees preserve children and tolerate orphans and cycles without recursion loops", () => {
  const input = [{ id: "1", parentID: "", name: "总部" }, { id: "2", parentID: "1", name: "研发" }, { id: "3", parentID: "missing", name: "独立部门" }, { id: "4", parentID: "5", name: "环一" }, { id: "5", parentID: "4", name: "环二" }];
  const tree = api.departmentTree(input);
  assert.equal(tree[0].children[0].key, "2");
  const flatten = values => values.flatMap(value => [value.key, ...flatten(value.children)]);
  assert.deepEqual(flatten(tree).sort(), ["1", "2", "3", "4", "5"]);
});
test("person data for a different user cannot grant a profile action", async () => {
  globalThis.directorySession = { userID: "employee-a", sessionToken: "opaque-a" };
  globalThis.directoryRequest = async () => ({ userID: "different-person", canDirectChat: true });
  await assert.rejects(api.getDirectoryPerson("requested-person"), error => error.status === 503);
});
