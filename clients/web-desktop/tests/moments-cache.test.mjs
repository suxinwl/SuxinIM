import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const {
  QueryClient,
  MutationObserver,
  QueryObserver,
  setLogger,
} = require("react-query");
// The failure case below deliberately rejects a synthetic request.
setLogger({ log() {}, warn() {}, error() {} });
const built = await build({
  entryPoints: ["src/api/moments-cache.ts"],
  write: false,
  bundle: true,
  format: "esm",
  platform: "node",
  plugins: [{ name: "session-boundary", setup(builder) {
    builder.onResolve({ filter: /^@\/im\/resource-session$/ }, () => ({ path: "scope", namespace: "synthetic" }));
    builder.onLoad({ filter: /.*/, namespace: "synthetic" }, () => ({ contents: "export const getResourceSessionVersion=()=>globalThis.momentResourceScope || 0;" }));
  } }],
});
const {
  momentKeys,
  updateMomentCaches,
  removeMomentCaches,
  uniqueMoments,
  momentMutationOptions,
} = await import(
  `data:text/javascript;base64,${Buffer.from(built.outputFiles[0].text).toString(
    "base64",
  )}`
);
const moment = (id = "m1", changes = {}) => ({
  workMomentID: id,
  userID: "alice",
  content: { text: "post", type: 0 },
  likeUsers: [],
  comments: [],
  ...changes,
});
const pages = (...items) => ({ pages: items, pageParams: items.map((_, i) => i + 1) });
const client = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false, cacheTime: Infinity },
      mutations: { retry: false },
    },
    logger: { log() {}, warn() {}, error() {} },
  });
const flush = () => new Promise((resolve) => setImmediate(resolve));

test("viewer/SID generations isolate keys and discard a late previous-session mutation", async () => {
  const q=client();
  const options=momentMutationOptions(q);
  const oldKey=momentKeys.list();
  q.setQueryData(oldKey,pages([moment()]));
  globalThis.momentResourceScope=1;
  try {
    const currentKey=momentKeys.list(); assert.notDeepEqual(currentKey,oldKey);
    q.setQueryData(currentKey,pages([moment('current-viewer')]));
    await options.onSuccess({data:{workMoment:moment('stale-viewer')}});
    assert.equal(q.getQueryData(momentKeys.detail('stale-viewer')),undefined);
    assert.deepEqual(q.getQueryData(currentKey).pages[0].map(item=>item.workMomentID),['current-viewer']);
  } finally { delete globalThis.momentResourceScope; q.clear(); }
});

test("confirmed likes update every cached feed/author page and the open detail, retaining pagination", () => {
  const q = client(),
    original = moment(),
    other = moment("m2");
  const feed = pages([other], [original]);
  q.setQueryData(momentKeys.list(), feed);
  q.setQueryData(momentKeys.list("alice"), pages([original]));
  q.setQueryData(momentKeys.detail("m1"), original);
  q.setQueryData(["Unrelated"], { content: "untouched" });
  const liked = moment("m1", { likeUsers: [{ userID: "bob", nickname: "Bob" }] });
  updateMomentCaches(q, liked);
  assert.deepEqual(q.getQueryData(momentKeys.list()).pages, [[other], [liked]]);
  assert.deepEqual(q.getQueryData(momentKeys.list()).pageParams, [1, 2]);
  assert.deepEqual(q.getQueryData(momentKeys.list("alice")).pages, [[liked]]);
  assert.deepEqual(q.getQueryData(momentKeys.detail("m1")), liked);
  assert.deepEqual(q.getQueryData(["Unrelated"]), { content: "untouched" });
  assert.deepEqual(feed.pages, [[other], [original]]);
  q.clear();
});

test("repeated authoritative snapshots do not duplicate likes/comments or insert a post into an unrelated author page", () => {
  const q = client(),
    original = moment(),
    other = moment("m2", { userID: "carol" });
  q.setQueryData(momentKeys.list(), pages([original]));
  q.setQueryData(momentKeys.list("carol"), pages([other]));
  const snapshot = moment("m1", {
    likeUsers: [{ userID: "bob" }],
    comments: [{ commentID: "c1", content: "reply" }],
  });
  updateMomentCaches(q, snapshot);
  updateMomentCaches(q, snapshot);
  assert.equal(q.getQueryData(momentKeys.list()).pages[0][0].likeUsers.length, 1);
  assert.equal(q.getQueryData(momentKeys.detail("m1")).comments.length, 1);
  assert.deepEqual(q.getQueryData(momentKeys.list("carol")).pages, [[other]]);
  q.clear();
});

test("real mutation lifecycle reflects add-comment, delete-comment and unlike without a manual refresh", async () => {
  const q = client();
  q.setQueryData(momentKeys.list(), pages([moment()]));
  q.setQueryData(momentKeys.detail("m1"), moment());
  const snapshots = [
    moment("m1", {
      likeUsers: [{ userID: "bob" }],
      comments: [{ commentID: "c1", content: "reply" }],
    }),
    moment("m1", { likeUsers: [{ userID: "bob" }], comments: [] }),
    moment(),
  ];
  for (const snapshot of snapshots) {
    const observer = new MutationObserver(q, {
      mutationFn: async () => ({ data: { workMoment: snapshot } }),
      ...momentMutationOptions(q),
    });
    await observer.mutate();
    assert.deepEqual(q.getQueryData(momentKeys.list()).pages[0][0], snapshot);
    assert.deepEqual(q.getQueryData(momentKeys.detail("m1")), snapshot);
  }
  q.clear();
});

test("failed mutation leaves the last confirmed comments/likes intact and rejects for the caller to show an error", async () => {
  const q = client(),
    original = moment("m1", { comments: [{ commentID: "keep" }] });
  q.setQueryData(momentKeys.list(), pages([original]));
  q.setQueryData(momentKeys.detail("m1"), original);
  const observer = new MutationObserver(q, {
    mutationFn: async () => {
      throw new Error("network unavailable");
    },
    ...momentMutationOptions(q),
  });
  await assert.rejects(observer.mutate(), /network unavailable/);
  assert.deepEqual(q.getQueryData(momentKeys.list()).pages, [[original]]);
  assert.deepEqual(q.getQueryData(momentKeys.detail("m1")), original);
  assert.equal(q.getQueryState(momentKeys.list()).isInvalidated, true);
  q.clear();
});

test("a stale interval fetch started during a mutation cannot overwrite its confirmed response", async () => {
  const q = client(),
    confirmed = moment("m1", { comments: [{ commentID: "fresh" }] });
  q.setQueryData(momentKeys.list(), pages([moment()]));
  let resolveOld;
  const observer = new MutationObserver(q, {
    ...momentMutationOptions(q),
    mutationFn: async () => {
      void q
        .fetchQuery(
          momentKeys.list(),
          () =>
            new Promise((resolve) => {
              resolveOld = resolve;
            }),
        )
        .catch(() => {});
      await flush();
      return { data: { workMoment: confirmed } };
    },
  });
  await observer.mutate();
  resolveOld(pages([moment()]));
  await flush();
  assert.deepEqual(q.getQueryData(momentKeys.list()).pages, [[confirmed]]);
  q.clear();
});

test("an observed feed re-fetches authoritative concurrent changes after the mutation snapshot", async () => {
  const q = client(),
    latest = moment("m1", { likeUsers: [{ userID: "bob" }, { userID: "carol" }] });
  q.setQueryData(momentKeys.list(), pages([moment()]));
  let authoritative = moment();
  const query = new QueryObserver(q, {
    queryKey: momentKeys.list(),
    staleTime: Infinity,
    queryFn: async () => pages([authoritative]),
  });
  const stop = query.subscribe(() => {});
  const mutation = new MutationObserver(q, {
    ...momentMutationOptions(q),
    mutationFn: async () => {
      authoritative = latest;
      return { data: { workMoment: moment("m1", { likeUsers: [{ userID: "bob" }] }) } };
    },
  });
  await mutation.mutate();
  await flush();
  assert.deepEqual(q.getQueryData(momentKeys.list()).pages, [[latest]]);
  stop();
  q.clear();
});

test("delete removes only the target from every page/detail and repeated deletion is safe", () => {
  const q = client(),
    original = moment(),
    other = moment("m2");
  q.setQueryData(momentKeys.list(), pages([original], [other, original]));
  q.setQueryData(momentKeys.list("alice"), pages([original]));
  q.setQueryData(momentKeys.detail("m1"), original);
  removeMomentCaches(q, "m1");
  removeMomentCaches(q, "m1");
  assert.deepEqual(q.getQueryData(momentKeys.list()).pages, [[], [other]]);
  assert.deepEqual(q.getQueryData(momentKeys.list("alice")).pages, [[]]);
  assert.equal(q.getQueryData(momentKeys.detail("m1")), undefined);
  q.clear();
});

test("page boundary overlap renders one item per ID without changing the stored page layout", () => {
  const original = moment(),
    other = moment("m2"),
    overlap = [[original], [original, other]];
  assert.deepEqual(uniqueMoments(overlap), [original, other]);
  assert.equal(overlap[1].length, 2);
  assert.deepEqual(uniqueMoments(), []);
});

test("malformed legacy notification does not destroy confirmed cache data", () => {
  const q = client(),
    original = moment();
  q.setQueryData(momentKeys.list(), pages([original]));
  updateMomentCaches(q, { workMomentID: "m1" });
  assert.deepEqual(q.getQueryData(momentKeys.list()).pages, [[original]]);
  q.clear();
});
