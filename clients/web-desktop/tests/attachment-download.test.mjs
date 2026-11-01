import test from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { EventEmitter } from "node:events";
import { mkdtempSync, existsSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

async function load(entry, plugins = []) {
  const result = await build({
    entryPoints: [entry],
    bundle: true,
    write: false,
    format: "esm",
    platform: "node",
    plugins,
  });
  return import(
    `data:text/javascript;base64,${Buffer.from(result.outputFiles[0].text).toString(
      "base64",
    )}`
  );
}
const { startAttachmentDownload } = await load("src/im/attachment-download.ts");
const { normalizeDownloadRequest, attachmentSavePath } = await load(
  "electron/main/downloadTarget.ts",
);
const syntheticUrl =
  "https://files.example.invalid/video%20one.mp4?X-Amz-Credential=SYNTHETIC%2Fscope&X-Amz-Signature=synthetic&part=a%20b~c";
function ports() {
  const tasks = new Map(),
    sent = [];
  return {
    tasks,
    sent,
    getTask: (key) => tasks.get(key),
    addTask: (key, task) => tasks.set(key, task),
    removeTask: (key) => tasks.delete(key),
    start: async (request) => {
      sent.push(request);
      assert.equal(tasks.get(request.url)?.downloadState, "downloading");
    },
    randomPrefix: () => "synthetic-unique",
  };
}
test("attachment URL signature bytes are untouched and cache metadata travels separately", async () => {
  const p = ports();
  await startAttachmentDownload(
    syntheticUrl,
    { saveType: "video", clientMsgID: "synthetic-video", randomName: true },
    p,
  );
  assert.deepEqual(p.sent, [
    { url: syntheticUrl, saveType: "video", randomPrefix: "synthetic-unique" },
  ]);
  assert.equal(p.tasks.get(syntheticUrl).downloadUrl, syntheticUrl);
  assert.equal(p.tasks.get(syntheticUrl).clientMsgID, "synthetic-video");
});
test("a rejected native start removes the task and a later user retry can start again", async () => {
  const p = ports();
  const start = p.start;
  p.start = async () => {
    throw new Error("synthetic IPC failure");
  };
  await assert.rejects(
    startAttachmentDownload(syntheticUrl, { saveType: "video" }, p),
    /IPC failure/,
  );
  assert.equal(p.tasks.size, 0);
  p.start = start;
  await startAttachmentDownload(syntheticUrl, { saveType: "video" }, p);
  assert.equal(p.sent.length, 1);
});
test("duplicate active clicks do not start another transfer", async () => {
  const p = ports();
  await Promise.all([
    startAttachmentDownload(syntheticUrl, { saveType: "video" }, p),
    startAttachmentDownload(syntheticUrl, { saveType: "video" }, p),
  ]);
  assert.equal(p.sent.length, 1);
});
test("non-network attachment URLs and credentials are refused before task creation", async () => {
  const p = ports();
  for (const url of [
    "file:///C:/secret",
    "javascript:alert(1)",
    "https://user:secret@example.invalid/a.mp4",
  ])
    await assert.rejects(startAttachmentDownload(url, {}, p));
  assert.equal(p.tasks.size, 0);
});
test("native normalization preserves signed URLs and refuses cache traversal prefixes", () => {
  assert.deepEqual(normalizeDownloadRequest({ url: syntheticUrl, saveType: "video" }), {
    url: syntheticUrl,
    saveType: "video",
  });
  assert.deepEqual(normalizeDownloadRequest(syntheticUrl + "&save-type=video"), {
    url: syntheticUrl + "&save-type=video",
    saveType: "file",
  });
  assert.throws(() =>
    normalizeDownloadRequest({
      url: syntheticUrl,
      saveType: "video",
      randomPrefix: "../../",
    }),
  );
  assert.throws(() => normalizeDownloadRequest({ url: "file:///C:/secret" }));
});
test("remote filenames cannot choose directories outside the managed cache", () => {
  const directory = path.resolve("synthetic-cache");
  for (const filename of [
    "../../escape.mp4",
    String.raw`C:\outside\escape.mp4`,
    "../",
    "",
  ]) {
    const output = attachmentSavePath(directory, filename);
    assert.equal(path.dirname(output), directory);
  }
  assert.throws(() => attachmentSavePath("", "video.mp4"));
});

const handlers = new Map();
globalThis.__attachmentIPC = {
  handle: (key, callback) => {
    if (handlers.has(key)) throw new Error("duplicate handler");
    handlers.set(key, callback);
  },
  removeHandler: (key) => handlers.delete(key),
};
const { initDownloadManage } = await load("electron/main/downloadManage.ts", [
  {
    name: "native-attachment-boundary",
    setup(builder) {
      builder.onResolve({ filter: /^electron$/ }, () => ({
        path: "electron",
        namespace: "attachment-native",
      }));
      builder.onLoad({ filter: /.*/, namespace: "attachment-native" }, () => ({
        contents: "export const ipcMain = globalThis.__attachmentIPC;",
      }));
    },
  },
]);
function nativeFixture() {
  const directory = mkdtempSync(path.join(tmpdir(), "openim-attachment-test-"));
  const session = new EventEmitter(),
    requests = [],
    events = [];
  session.downloadURL = (url) => requests.push(url);
  const contents = {
    session,
    isDestroyed: () => false,
    send: (...args) => events.push(args),
  };
  globalThis.pathConfig = Object.fromEntries(
    ["image", "video", "avatar", "file"].map((type) => [
      `${type}CachePath`,
      path.join(directory, type),
    ]),
  );
  const clean = initDownloadManage(contents);
  const invoke = (name, input, sender = contents) =>
    handlers.get(name)({ sender }, input);
  function item(url = syntheticUrl, filename = "movie.mp4") {
    const value = new EventEmitter();
    let savePath = "";
    let paused = false;
    Object.assign(value, {
      getURLChain: () => [url, "https://redirect.invalid/object"],
      getURL: () => url,
      getFilename: () => filename,
      setSavePath: (path) => (savePath = path),
      getSavePath: () => savePath,
      getTotalBytes: () => 0,
      getReceivedBytes: () => 5,
      isPaused: () => paused,
      pause: () => {
        paused = true;
      },
      resume: () => {
        paused = false;
      },
      cancel: () => value.emit("done", {}, "cancelled"),
    });
    return value;
  }
  return { directory, session, requests, events, contents, invoke, item, clean };
}
test("native download creates the selected cache and correlates completion to the unmodified URL", () => {
  const f = nativeFixture();
  try {
    f.invoke("startDownload", { url: syntheticUrl, saveType: "video" });
    assert.deepEqual(f.requests, [syntheticUrl]);
    const item = f.item();
    f.session.emit("will-download", { preventDefault() {} }, item);
    assert.equal(path.dirname(item.getSavePath()), path.join(f.directory, "video"));
    assert.equal(existsSync(path.dirname(item.getSavePath())), true);
    writeFileSync(item.getSavePath(), "synthetic video");
    item.emit("updated", {}, "progressing");
    assert.deepEqual(f.events.at(-1), ["downloadProgress", syntheticUrl, 0]);
    item.emit("done", {}, "completed");
    assert.deepEqual(f.events.at(-1), [
      "downloadSuccess",
      syntheticUrl,
      item.getSavePath(),
    ]);
    assert.equal(readFileSync(item.getSavePath(), "utf8"), "synthetic video");
  } finally {
    f.clean();
  }
});
test("a failed transfer removes partial bytes and accepts a retry, without deleting the cache directory", () => {
  const f = nativeFixture();
  try {
    f.invoke("startDownload", { url: syntheticUrl, saveType: "video" });
    const item = f.item();
    f.session.emit("will-download", { preventDefault() {} }, item);
    writeFileSync(item.getSavePath(), "partial");
    item.emit("done", {}, "interrupted");
    assert.equal(existsSync(item.getSavePath()), false);
    assert.equal(existsSync(path.dirname(item.getSavePath())), true);
    assert.deepEqual(f.events.at(-1).slice(0, 2), ["downloadFailed", syntheticUrl]);
    f.invoke("startDownload", { url: syntheticUrl, saveType: "video" });
    assert.equal(f.requests.length, 2);
  } finally {
    f.clean();
  }
});
test("pause, resume and cancellation use the original URL and reject another window", () => {
  const f = nativeFixture();
  try {
    assert.throws(() => f.invoke("startDownload", { url: syntheticUrl }, {}), /window/);
    f.invoke("startDownload", { url: syntheticUrl, saveType: "video" });
    const item = f.item();
    f.session.emit("will-download", { preventDefault() {} }, item);
    f.invoke("pauseDownload", syntheticUrl);
    assert.equal(item.isPaused(), true);
    f.invoke("resumeDownload", syntheticUrl);
    assert.equal(item.isPaused(), false);
    f.invoke("cancelDownload", syntheticUrl);
    assert.deepEqual(f.events.at(-1), ["downloadCancel", syntheticUrl]);
  } finally {
    f.clean();
  }
});
