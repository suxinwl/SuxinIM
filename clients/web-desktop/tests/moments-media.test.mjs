import test from 'node:test';
import assert from 'node:assert/strict';
import { build } from 'esbuild';
const compiled = await build({ entryPoints: ['src/api/moments-media.ts'], write: false, bundle: true, platform: 'node', format: 'esm', plugins: [{ name: 'read-grant-boundary', setup(builder) {
  builder.onResolve({ filter: /^@\/im\/resources$/ }, () => ({ path: 'resources', namespace: 'mock' }));
  builder.onResolve({ filter: /^@\/im\/resource-session$/ }, () => ({ path: 'scope', namespace: 'mock' }));
  builder.onLoad({ filter: /.*/, namespace: 'mock' }, args => ({ contents: args.path==='scope' ? 'export const getResourceSessionVersion=()=>globalThis.momentMediaScope || 0;' : 'export const authorizeResource=(...args)=>globalThis.authorizeMomentFixture(...args);' }));
} }] });
const { authorizeMomentMedia, publishedMomentMedia } = await import('data:text/javascript;base64,' + Buffer.from(compiled.outputFiles[0].text).toString('base64'));
test('viewer reads reauthorize both stale publisher URLs without mutating persisted/upload metadata', async () => {
  const input = { workMomentID: 'synthetic', content: { text: 'synthetic', type: 0, metas: [{ original: 'https://im.szputy.com:9885/attachments/revoked-publisher/openim38/image?old=1', thumb: 'https://im.szputy.com:9885/attachments/revoked-publisher/openim38/image?old=1?type=image', width: 80, height: 60 }] } };
  const before = JSON.stringify(input), calls = [];
  globalThis.authorizeMomentFixture = async url => { calls.push(url); return 'https://192.168.10.11:9885/attachments/current-viewer/openim38/image'; };
  const output = await authorizeMomentMedia(input);
  assert.deepEqual(calls, [input.content.metas[0].original, input.content.metas[0].thumb]);
  assert.equal(output.content.metas[0].original, 'https://192.168.10.11:9885/attachments/current-viewer/openim38/image');
  assert.equal(output.content.metas[0].thumb, output.content.metas[0].original);
  assert.equal(JSON.stringify(input), before); assert.notEqual(output, input);
});
test('publishing preserves an existing signed query byte-for-byte and never appends transformations', () => {
  for (const url of ['https://im.szputy.com:9885/api/object/image', 'https://im.szputy.com:9885/attachments/upload/openim38/image?X-Amz-Signature=a%2Fb&X-Amz-SignedHeaders=host']) {
    assert.deepEqual(publishedMomentMedia(url, 80, 60), { original: url, thumb: url, width: 80, height: 60 });
  }
});
test('text-only posts require no media credentials', async () => {
  const post = { workMomentID: 'text-only', content: { text: 'synthetic', type: 0 } };
  globalThis.authorizeMomentFixture = () => { throw new Error('must not authorize text'); };
  assert.equal(await authorizeMomentMedia(post), post);
});
test('one denied asset stays empty in the read copy while other images/posts remain available', async () => {
  globalThis.authorizeMomentFixture = async url => { if (url === 'missing') throw new Error('synthetic denied'); return 'viewer-grant-for-' + url; };
  const input = { content: { text: 'visible post', metas: [{ original: 'missing', thumb: 'good-thumb' }, { original: 'good-image', thumb: 'good-image' }] } };
  const result = await authorizeMomentMedia(input);
  assert.equal(result.content.metas[0].original, '');
  assert.equal(result.content.metas[0].thumb, 'viewer-grant-for-good-thumb');
  assert.equal(result.content.metas[1].original, 'viewer-grant-for-good-image');
  assert.equal(result.content.text, 'visible post');
  assert.equal(result.content.metas[0].readSources.original, 'missing'); // only for controlled component retry
  assert.equal(input.content.metas[0].original, 'missing');
});

test('session changes still reject the entire stale read instead of treating it as an individual missing image', async () => {
  globalThis.authorizeMomentFixture = async () => { globalThis.momentMediaScope=1; throw new Error('session changed'); };
  try { await assert.rejects(authorizeMomentMedia({content:{metas:[{original:'old-viewer',thumb:'old-viewer-thumb'}]}}),/session changed/); }
  finally { delete globalThis.momentMediaScope; }
});
