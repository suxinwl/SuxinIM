import test from 'node:test';
import assert from 'node:assert/strict';
import { build } from 'esbuild';

const compiled = await build({ entryPoints: ['src/im/local-media.ts'], write: false, bundle: true, platform: 'node', format: 'esm' });
const { readLocalImage, readLocalVideo, MediaPreparationError, fileMessageErrorKey } = await import('data:text/javascript;base64,' + Buffer.from(compiled.outputFiles[0].text).toString('base64'));
const sample = new File(['synthetic input'], 'synthetic.mp4', { type: 'video/mp4' });
function mediaBoundary(changes = {}) {
  const previous = { document: globalThis.document, Image: globalThis.Image, create: URL.createObjectURL, revoke: URL.revokeObjectURL };
  const state = { revoked: [], loadCount: 0, pauseCount: 0, drawCount: 0, video: undefined, ...changes };
  URL.createObjectURL = () => 'blob:synthetic-local-media';
  URL.revokeObjectURL = value => state.revoked.push(value);
  class LocalImage {
    width = changes.width ?? 160; height = changes.height ?? 90;
    set src(value) { if (value && changes.imageEvent !== 'none') queueMicrotask(() => this[changes.imageEvent === 'error' ? 'onerror' : 'onload']?.()); }
  }
  globalThis.Image = LocalImage;
  globalThis.document = { createElement(type) {
    if (type === 'canvas') return { getContext: () => changes.noContext ? null : { drawImage() { state.drawCount++; } }, toBlob(callback) {
      if (changes.delayBlob) { state.blobCallback = callback; return; }
      callback(changes.noBlob ? null : new Blob(['synthetic PNG bytes'], { type: 'image/png' }));
    } };
    assert.equal(type, 'video');
    return state.video = {
      duration: changes.duration ?? 2.4, videoWidth: changes.width ?? 160, videoHeight: changes.height ?? 90,
      pause() { state.pauseCount++; }, removeAttribute(name) { assert.equal(name, 'src'); delete this.src; },
      load() { state.loadCount++; if (this.src && changes.videoEvent !== 'none') queueMicrotask(() => this[changes.videoEvent === 'error' ? 'onerror' : 'onloadeddata']?.()); },
    };
  } };
  return { state, restore() { globalThis.document = previous.document; globalThis.Image = previous.Image; URL.createObjectURL = previous.create; URL.revokeObjectURL = previous.revoke; } };
}
async function boundary(changes, action) { const env = mediaBoundary(changes); try { return await action(env.state); } finally { env.restore(); } }

test('video preparation decodes one frame and supplies integer seconds plus a PNG File', async () => boundary({}, async state => {
  const result = await readLocalVideo(sample, 1000);
  assert.equal(result.duration, 2); assert.equal(result.snapshot.type, 'image/png'); assert.ok(result.snapshot.size > 0);
  assert.match(result.snapshot.name, /^snapshot-\d+\.png$/);
  assert.equal(state.video.preload, 'auto'); assert.equal(state.video.muted, true); assert.equal(state.video.playsInline, true);
  assert.equal(state.drawCount, 1); assert.equal(state.pauseCount, 1); assert.deepEqual(state.revoked, ['blob:synthetic-local-media']);
  assert.equal(state.video.onloadeddata, null); assert.equal(state.video.onerror, null);
}));

test('unsupported video rejects on decode error and releases its local source', async () => boundary({ videoEvent: 'error' }, async state => {
  await assert.rejects(readLocalVideo(sample, 1000), error => error instanceof MediaPreparationError && error.reason === 'decode');
  assert.equal(state.drawCount, 0); assert.equal(state.revoked.length, 1);
}));

test('stalled video metadata is bounded instead of leaving the file selection pending', async () => boundary({ videoEvent: 'none' }, async state => {
  await assert.rejects(readLocalVideo(sample, 5), error => error.reason === 'timeout');
  assert.equal(state.pauseCount, 1); assert.equal(state.revoked.length, 1);
}));

test('invalid duration and absent video tracks cannot produce an uploadable video message', async () => {
  for (const changes of [{ duration: NaN }, { duration: Infinity }, { duration: 0 }, { duration: -1 }, { width: 0 }, { height: 0 }]) {
    await boundary(changes, async state => { await assert.rejects(readLocalVideo(sample, 100), error => error.reason === 'decode'); assert.equal(state.drawCount, 0); assert.equal(state.revoked.length, 1); });
  }
});

test('canvas failure rejects locally and does not leak the video source', async () => {
  for (const changes of [{ noContext: true }, { noBlob: true }]) await boundary(changes, async state => {
    await assert.rejects(readLocalVideo(sample, 100), error => error.reason === 'decode'); assert.equal(state.revoked.length, 1);
  });
});

test('late PNG encoding after timeout cannot revive the rejected preparation', async () => boundary({ delayBlob: true }, async state => {
  await assert.rejects(readLocalVideo(sample, 5), error => error.reason === 'timeout');
  state.blobCallback(new Blob(['late PNG'], { type: 'image/png' }));
  assert.equal(state.revoked.length, 1); assert.equal(state.pauseCount, 1);
}));

test('image metadata retains its dimensions and releases only its temporary metadata URL', async () => boundary({}, async state => {
  const image = await readLocalImage(new File(['synthetic'], 'image.png'), 100);
  assert.equal(image.width, 160); assert.equal(image.height, 90); assert.equal(state.revoked.length, 1);
}));

test('corrupt and stalled images reject with the same bounded error contract', async () => {
  for (const changes of [{ imageEvent: 'error' }, { imageEvent: 'none' }, { width: 0 }]) await boundary(changes, async state => {
    await assert.rejects(readLocalImage(sample, 5), error => error instanceof MediaPreparationError);
    assert.equal(state.revoked.length, 1);
  });
});

test('visible error keys never contain remote SDK errors or signed URLs', () => {
  assert.equal(fileMessageErrorKey(new MediaPreparationError('decode')), 'toast.mediaDecodeFailed');
  assert.equal(fileMessageErrorKey(new MediaPreparationError('timeout')), 'toast.mediaPreparationTimeout');
  assert.equal(fileMessageErrorKey({ errMsg: 'PUT https://private.invalid/opaque?signature=secret' }), 'toast.uploadFailed');
});
