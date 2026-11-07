import {promises as fs, constants} from 'node:fs';
import path from 'node:path';
import {createHash, randomUUID} from 'node:crypto';

export type RestoreTransactionOptions = {
  protect: (plain: Buffer) => Promise<Uint8Array>;
  unprotect: (protectedBytes: Buffer) => Promise<Uint8Array>;
  /** Must bind the caller's current online account, SID, epoch and operation generation. */
  check: () => Promise<void>;
  /** Required only for collection, after the restored SDK has completed fresh synchronization. */
  freshSync?: () => Promise<void>;
};
export type RestoreTransactionResult = {
  state: 'none' | 'promoted' | 'rolled-back' | 'collected' | 'pending';
  pendingPrevious: boolean;
  transactionID?: string;
  reasonCode?: string;
};
type Identity = {dev: string; ino: string; mode: number};
type Entry = {relative: string; identity: Identity; directory: boolean;
  size?: string; modified?: string; sha256?: string};
type Tree = {entries: Entry[]};
type RootBinding = {directory: Identity; marker: Entry; markerID: string};
type Journal = {format: 'PUTY-RESTORE-TRANSACTION-1'; root: string; account: string;
  transactionID: string; stage: string; previous: string; binding: RootBinding;
  oldTree: Tree | null; newTree: Tree};
type Receipt = {format: 'PUTY-RESTORE-RECEIPT-1'; kind: 'promoted' | 'gc';
  account: string; transactionID: string; journalSHA256: string; active: Identity};
type Document<T> = {value: T; entry: Entry; hash: string};
type Location = {root: string; account: string; accounts: string; metadata: string;
  journal: string; promoted: string; gc: string; collected: string; rollback: string};
const UUID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const operations = new Map<string, Promise<unknown>>();
const digest = (bytes: Uint8Array) => createHash('sha256').update(bytes).digest('hex');
const canonical = (value: unknown): string => Array.isArray(value)
  ? `[${value.map(canonical).join(',')}]`
  : value && typeof value === 'object'
    ? `{${Object.keys(value).sort().map(k => `${JSON.stringify(k)}:${canonical((value as Record<string, unknown>)[k])}`).join(',')}}`
    : JSON.stringify(value);
class RestoreError extends Error { constructor(public code: string) {super(code);} }
const require = (condition: unknown, code: string): void => {if (!condition) throw new RestoreError(code);};
const id = (s: {dev: bigint; ino: bigint; mode: bigint}): Identity =>
  ({dev: String(s.dev), ino: String(s.ino), mode: Number(s.mode & 0o777n)});
const equalID = (a: Identity, b: Identity) => a.dev === b.dev && a.ino === b.ino && a.mode === b.mode;
const exists = async (p: string) => fs.lstat(p).then(() => true, e => {
  if (e.code === 'ENOENT') return false; throw e;
});
async function authorized(options: RestoreTransactionOptions, fresh = false) {
  try {await options.check(); if (fresh) {
    require(typeof options.freshSync === 'function', 'FRESH_SYNC_REQUIRED');
    await options.freshSync!(); await options.check();
  }} catch (e) {if (e instanceof RestoreError) throw e; throw new RestoreError('AUTHORIZATION_REQUIRED');}
}
async function serial<T>(root: string, account: string, work: () => Promise<T>): Promise<T> {
  const key = root + '\0' + account, previous = operations.get(key) || Promise.resolve();
  const next = previous.catch(() => {}).then(work); operations.set(key, next);
  try {return await next;} finally {if (operations.get(key) === next) operations.delete(key);}
}
async function directory(p: string): Promise<Identity> {
  const s = await fs.lstat(p, {bigint: true});
  require(s.isDirectory() && !s.isSymbolicLink() && s.ino > 0n && await fs.realpath(p) === p, 'DIRECTORY_OWNERSHIP_REQUIRED');
  return id(s);
}
async function syncDirectory(p: string) {
  let handle;
  try {handle = await fs.open(p, constants.O_RDONLY); await handle.sync();}
  catch (e) {
    // Node cannot open Windows directories for FlushFileBuffers. Metadata files
    // themselves are flushed both before and after their atomic rename there.
    if (!(process.platform === 'win32' && ['EPERM', 'EISDIR', 'EINVAL', 'EBADF'].includes((e as NodeJS.ErrnoException).code || ''))) throw e;
  } finally {await handle?.close();}
}
async function ensureDirectory(p: string, parent: string, options: RestoreTransactionOptions) {
  if (!await exists(p)) {
    await authorized(options); await fs.mkdir(p); await syncDirectory(parent); await authorized(options);
  }
  return directory(p);
}
function resolveEntry(base: string, relative: string) {
  require(relative === '' || (relative.length < 2048 && !relative.includes('\\') && !relative.includes(':') &&
    relative.split('/').every(x => x !== '' && x !== '.' && x !== '..' && !/[\x00-\x1f\x7f]/.test(x))), 'INVALID_RECEIPT_PATH');
  const result = path.resolve(base, ...relative.split('/').filter(Boolean));
  require(result === base || result.startsWith(base + path.sep), 'PATH_OUTSIDE_OWNED_TREE'); return result;
}
async function fileEntry(p: string, relative: string, options: RestoreTransactionOptions): Promise<Entry> {
  await authorized(options);
  const before = await fs.lstat(p, {bigint: true});
  require(before.isFile() && !before.isSymbolicLink() && before.nlink === 1n && before.ino > 0n &&
    await fs.realpath(p) === p && before.size <= BigInt(Number.MAX_SAFE_INTEGER), 'REGULAR_SINGLE_LINK_FILE_REQUIRED');
  const handle = await fs.open(p, constants.O_RDONLY | (constants.O_NOFOLLOW || 0));
  const hash = createHash('sha256'), buffer = Buffer.alloc(1024 * 1024);
  const same = (s: typeof before) => s.isFile() && s.nlink === 1n && equalID(id(s), id(before)) &&
    s.size === before.size && s.mtimeNs === before.mtimeNs;
  try {
    require(same(await handle.stat({bigint: true})), 'FILE_CHANGED');
    for (;;) {await authorized(options); const read = await handle.read(buffer, 0, buffer.length, null);
      await authorized(options); if (!read.bytesRead) break; hash.update(buffer.subarray(0, read.bytesRead));}
    require(same(await handle.stat({bigint: true})) && same(await fs.lstat(p, {bigint: true})) &&
      await fs.realpath(p) === p, 'FILE_CHANGED');
    return {relative, identity: id(before), directory: false, size: String(before.size),
      modified: String(before.mtimeNs), sha256: hash.digest('hex')};
  } finally {buffer.fill(0); await handle.close();}
}
async function tree(base: string, options: RestoreTransactionOptions): Promise<Tree> {
  const entries: Entry[] = [];
  async function walk(relative: string, depth: number) {
    require(depth <= 32 && entries.length <= 100000, 'TREE_LIMIT_EXCEEDED');
    await authorized(options); const p = resolveEntry(base, relative), identity = await directory(p);
    entries.push({relative, identity, directory: true});
    const names = (await fs.readdir(p)).sort();
    for (const name of names) {
      const next = relative ? relative + '/' + name : name, child = resolveEntry(base, next);
      const s = await fs.lstat(child);
      if (s.isDirectory() && !s.isSymbolicLink()) await walk(next, depth + 1);
      else entries.push(await fileEntry(child, next, options));
      require(entries.length <= 100000, 'TREE_LIMIT_EXCEEDED');
    }
    require(equalID(identity, await directory(p)) && canonical(names) === canonical((await fs.readdir(p)).sort()), 'TREE_CHANGED');
  }
  await walk('', 0); return {entries: entries.sort((a, b) => a.relative.localeCompare(b.relative, 'en'))};
}
function validateTree(value: Tree) {
  require(value && Array.isArray(value.entries) && value.entries.length > 0 && value.entries.length <= 100000, 'INVALID_TREE_RECEIPT');
  const paths = new Set<string>();
  for (const item of value.entries) {
    require(item && typeof item.relative === 'string' && !paths.has(item.relative) && typeof item.directory === 'boolean', 'INVALID_TREE_RECEIPT');
    resolveEntry(path.resolve('.'), item.relative); paths.add(item.relative);
    require(item.identity && /^\d+$/.test(item.identity.dev) && /^[1-9]\d*$/.test(item.identity.ino) &&
      Number.isInteger(item.identity.mode) && item.identity.mode >= 0 && item.identity.mode <= 0o777, 'INVALID_TREE_RECEIPT');
    if (!item.directory) require(typeof item.size === 'string' && /^\d+$/.test(item.size) &&
      typeof item.modified === 'string' && /^\d+$/.test(item.modified) && /^[a-f0-9]{64}$/.test(item.sha256 || ''), 'INVALID_TREE_RECEIPT');
  }
  require(paths.has('') && value.entries.find(e => e.relative === '')!.directory, 'INVALID_TREE_RECEIPT');
  const directories = new Set(value.entries.filter(e => e.directory).map(e => e.relative));
  for (const item of value.entries) if (item.relative) {
    const parent = item.relative.includes('/') ? item.relative.slice(0, item.relative.lastIndexOf('/')) : '';
    require(directories.has(parent), 'INVALID_TREE_RECEIPT');
  }
}
async function flushStage(base: string, snapshot: Tree, options: RestoreTransactionOptions) {
  for (const entry of snapshot.entries.filter(e => !e.directory)) {
    await authorized(options); const file = resolveEntry(base, entry.relative);
    const handle = await fs.open(file, constants.O_RDWR | (constants.O_NOFOLLOW || 0));
    try {
      const before = await handle.stat({bigint: true});
      require(before.isFile() && before.nlink === 1n && equalID(id(before), entry.identity) &&
        String(before.size) === entry.size && String(before.mtimeNs) === entry.modified && await fs.realpath(file) === file, 'TREE_CHANGED');
      await handle.sync(); await authorized(options);
    } finally {await handle.close();}
  }
  for (const entry of snapshot.entries.filter(e => e.directory).sort((a, b) => b.relative.length - a.relative.length))
    await syncDirectory(resolveEntry(base, entry.relative));
  await syncDirectory(path.dirname(base)); await authorized(options);
}
async function verifyTree(base: string, expected: Tree, options: RestoreTransactionOptions, missing = false) {
  if (!await exists(base)) {require(missing, 'RESTORE_TREE_MISSING'); return;}
  const actual = await tree(base, options), known = new Map(expected.entries.map(e => [e.relative, e]));
  require(missing || actual.entries.length === expected.entries.length, 'TREE_CHANGED');
  for (const entry of actual.entries) require(canonical(known.get(entry.relative)) === canonical(entry), 'TREE_CHANGED');
}
async function location(root: string, account: string, options: RestoreTransactionOptions): Promise<Location> {
  await authorized(options);
  require(path.isAbsolute(root) && path.resolve(root) === root && /^emp_[a-f0-9]{32}$/.test(account), 'OWNED_ROOT_AND_ACCOUNT_REQUIRED');
  await directory(root); const accounts = path.join(root, 'accounts'); await directory(accounts);
  const metadata = path.join(root, '.restore-transactions'); await ensureDirectory(metadata, root, options);
  const stem = path.join(metadata, account);
  return {root, account, accounts, metadata, journal: stem + '.journal', promoted: stem + '.promoted',
    gc: stem + '.gc', collected: stem + '.collected', rollback: stem + '.rolled-back'};
}
async function rootBinding(loc: Location, options: RestoreTransactionOptions): Promise<RootBinding> {
  const markerPath = path.join(loc.root, '.puty-local-root.json'), marker = await fileEntry(markerPath, '.puty-local-root.json', options);
  require(BigInt(marker.size!) < 4096n, 'INVALID_ROOT_MARKER');
  const value = JSON.parse(await fs.readFile(markerPath, 'utf8'));
  require(value.format === 'PUTY-LOCAL-ROOT-1' && UUID.test(value.id || ''), 'INVALID_ROOT_MARKER');
  require(canonical(await fileEntry(markerPath, '.puty-local-root.json', options)) === canonical(marker), 'ROOT_MARKER_CHANGED');
  return {directory: await directory(loc.root), marker, markerID: value.id};
}
async function verifyBinding(loc: Location, journal: Journal, options: RestoreTransactionOptions) {
  require(canonical(await rootBinding(loc, options)) === canonical(journal.binding), 'ROOT_BINDING_CHANGED');
}
async function writeDocument(file: string, value: unknown, options: RestoreTransactionOptions) {
  await authorized(options); require(!await exists(file), 'TRANSACTION_METADATA_EXISTS');
  const plain = Buffer.from(canonical(value)); let sealed: Buffer;
  try {sealed = Buffer.from(await options.protect(plain));} finally {plain.fill(0);}
  require(sealed.length > 0 && sealed.length <= 64 * 1024 * 1024, 'INVALID_PROTECTED_METADATA');
  const temporary = file + '.next-' + randomUUID(); let handle; let written = false;
  try {
    await authorized(options); handle = await fs.open(temporary, 'wx', 0o600);
    await handle.writeFile(sealed); await handle.sync(); await handle.close(); handle = undefined;
    await authorized(options); require(!await exists(file), 'TRANSACTION_METADATA_EXISTS');
    await fs.rename(temporary, file); written = true;
    const final = await fs.open(file, 'r+'); try {await final.sync();} finally {await final.close();}
    await syncDirectory(path.dirname(file)); await authorized(options);
  } finally {
    sealed.fill(0); await handle?.close();
    if (!written) await fs.unlink(temporary).catch(e => {if (e.code !== 'ENOENT') throw e;});
  }
}
async function readDocument<T>(file: string, options: RestoreTransactionOptions): Promise<Document<T>> {
  const entry = await fileEntry(file, path.basename(file), options);
  require(BigInt(entry.size!) > 0n && BigInt(entry.size!) <= 64n * 1024n * 1024n, 'INVALID_PROTECTED_METADATA');
  const bytes = await fs.readFile(file); let plain: Buffer | undefined;
  try {
    require(digest(bytes) === entry.sha256, 'TRANSACTION_METADATA_CHANGED'); await authorized(options);
    try {plain = Buffer.from(await options.unprotect(bytes));} catch {throw new RestoreError('PROTECTED_METADATA_REJECTED');}
    require(canonical(await fileEntry(file, path.basename(file), options)) === canonical(entry), 'TRANSACTION_METADATA_CHANGED');
    return {value: JSON.parse(plain.toString('utf8')) as T, entry, hash: entry.sha256!};
  } finally {plain?.fill(0); bytes.fill(0);}
}
async function flushDocument(file: string, doc: Document<unknown>, options: RestoreTransactionOptions) {
  await authorized(options);
  require(canonical(await fileEntry(file, path.basename(file), options)) === canonical(doc.entry), 'TRANSACTION_METADATA_CHANGED');
  const handle = await fs.open(file, constants.O_RDWR | (constants.O_NOFOLLOW || 0));
  try {
    const s = await handle.stat({bigint: true});
    require(s.isFile() && s.nlink === 1n && equalID(id(s), doc.entry.identity) &&
      String(s.size) === doc.entry.size && String(s.mtimeNs) === doc.entry.modified, 'TRANSACTION_METADATA_CHANGED');
    await handle.sync(); await authorized(options);
  } finally {await handle.close();}
  await syncDirectory(path.dirname(file)); await authorized(options);
  require(canonical(await fileEntry(file, path.basename(file), options)) === canonical(doc.entry), 'TRANSACTION_METADATA_CHANGED');
}
function validateJournal(j: Journal, loc: Location) {
  require(j && j.format === 'PUTY-RESTORE-TRANSACTION-1' && j.root === loc.root && j.account === loc.account && UUID.test(j.transactionID || ''), 'JOURNAL_BINDING_REJECTED');
  require(/^\.account-[a-f0-9-]{36}$/.test(j.stage || '') && UUID.test(j.stage.slice(9)) &&
    j.previous === `.restore-rollback/${loc.account}-${j.transactionID}`, 'JOURNAL_PATH_REJECTED');
  validateTree(j.newTree); if (j.oldTree) validateTree(j.oldTree);
  require(j.binding && j.binding.directory && j.binding.marker && UUID.test(j.binding.markerID || ''), 'JOURNAL_BINDING_REJECTED');
}
const activeRoot = (loc: Location) => path.join(loc.accounts, loc.account);
const stageRoot = (loc: Location, j: Journal) => path.join(loc.accounts, j.stage);
const previousRoot = (loc: Location, j: Journal) => path.join(loc.root, ...j.previous.split('/'));
const treeID = (value: Tree) => value.entries.find(e => e.relative === '')!.identity;
function receipt(j: Journal, hash: string, kind: 'promoted' | 'gc'): Receipt {
  return {format: 'PUTY-RESTORE-RECEIPT-1', kind, account: j.account,
    transactionID: j.transactionID, journalSHA256: hash, active: treeID(j.newTree)};
}
async function validateReceipt(file: string, j: Journal, hash: string, kind: 'promoted' | 'gc', options: RestoreTransactionOptions) {
  const value = await readDocument<Receipt>(file, options);
  require(canonical(value.value) === canonical(receipt(j, hash, kind)), 'PROMOTED_RECEIPT_REJECTED');
  await flushDocument(file, value, options); return value;
}
async function move(from: string, to: string, options: RestoreTransactionOptions) {
  await authorized(options); require(!await exists(to), 'RESTORE_DESTINATION_OCCUPIED');
  await directory(from); await directory(path.dirname(to));
  await fs.rename(from, to); await syncDirectory(path.dirname(from));
  if (path.dirname(from) !== path.dirname(to)) await syncDirectory(path.dirname(to));
  await authorized(options);
}
async function unlinkDocument(file: string, expected: Document<unknown>, options: RestoreTransactionOptions) {
  await authorized(options);
  require(canonical(await fileEntry(file, path.basename(file), options)) === canonical(expected.entry), 'TRANSACTION_METADATA_CHANGED');
  await fs.unlink(file); await syncDirectory(path.dirname(file)); await authorized(options);
}
async function finalCleanup(loc: Location, doc: Document<Journal>, finished: 'collected' | 'rolled-back', options: RestoreTransactionOptions): Promise<RestoreTransactionResult> {
  const j = doc.value;
  await verifyBinding(loc, j, options);
  require(equalID(await directory(activeRoot(loc)), treeID(finished === 'collected' ? j.newTree : j.oldTree!)), 'ACTIVE_ROOT_CHANGED');
  require(!await exists(stageRoot(loc, j)) && !await exists(previousRoot(loc, j)), 'UNFINISHED_RESTORE_TREE');
  for (const [file, kind] of [[loc.gc, 'gc'], [loc.promoted, 'promoted']] as const) if (await exists(file)) {
    require(finished === 'collected', 'UNEXPECTED_TRANSACTION_RECEIPT');
    const value = await validateReceipt(file, j, doc.hash, kind, options); await unlinkDocument(file, value, options);
  }
  await unlinkDocument(finished === 'collected' ? loc.collected : loc.rollback, doc, options);
  return {state: finished, transactionID: j.transactionID, pendingPrevious: false};
}
async function finishJournal(loc: Location, doc: Document<Journal>, type: 'collected' | 'rolled-back', options: RestoreTransactionOptions) {
  const target = type === 'collected' ? loc.collected : loc.rollback;
  require(!await exists(target), 'TRANSACTION_METADATA_EXISTS'); await authorized(options);
  require(canonical(await fileEntry(loc.journal, path.basename(loc.journal), options)) === canonical(doc.entry), 'TRANSACTION_METADATA_CHANGED');
  await fs.rename(loc.journal, target); await syncDirectory(loc.metadata); await authorized(options);
  return finalCleanup(loc, {...doc, entry: {...doc.entry, relative: path.basename(target)}}, type, options);
}
async function recover(loc: Location, options: RestoreTransactionOptions): Promise<RestoreTransactionResult> {
  const found = await Promise.all([loc.journal, loc.collected, loc.rollback].map(exists));
  require(found.filter(Boolean).length <= 1, 'AMBIGUOUS_RESTORE_TRANSACTION');
  if (!found.some(Boolean)) {
    require(!await exists(loc.promoted) && !await exists(loc.gc), 'ORPHAN_RESTORE_RECEIPT');
    if (await exists(activeRoot(loc))) await directory(activeRoot(loc));
    return {state: 'none', pendingPrevious: false};
  }
  const source = found[0] ? loc.journal : found[1] ? loc.collected : loc.rollback;
  const doc = await readDocument<Journal>(source, options), j = doc.value;
  validateJournal(j, loc); await verifyBinding(loc, j, options);
  // A previous file/parent sync may have failed AFTER rename. Complete bytes
  // are not proof of durability: establish it again before any tree mutation.
  await flushDocument(source, doc, options);
  if (!found[0]) return finalCleanup(loc, doc, found[1] ? 'collected' : 'rolled-back', options);
  const active = activeRoot(loc), staged = stageRoot(loc, j), previous = previousRoot(loc, j);
  if (await exists(loc.promoted)) {
    await validateReceipt(loc.promoted, j, doc.hash, 'promoted', options);
    require(equalID(await directory(active), treeID(j.newTree)) && !await exists(staged), 'ACTIVE_ROOT_CHANGED');
    return {state: 'promoted', transactionID: j.transactionID, pendingPrevious: j.oldTree !== null};
  }
  require(!await exists(loc.gc), 'UNEXPECTED_GC_RECEIPT');
  const current = await exists(active), stage = await exists(staged), old = await exists(previous);
  if (current && equalID(await directory(active), treeID(j.newTree))) {
    require(!stage && (j.oldTree !== null) === old, 'RESTORE_LAYOUT_CHANGED');
    await verifyTree(active, j.newTree, options);
    if (j.oldTree) await verifyTree(previous, j.oldTree, options);
  } else if (stage) {
    await verifyTree(staged, j.newTree, options);
    if (j.oldTree) {
      require(current !== old, 'RESTORE_LAYOUT_CHANGED');
      if (current) {await verifyTree(active, j.oldTree, options); await move(active, previous, options);}
      else await verifyTree(previous, j.oldTree, options);
    } else require(!current && !old, 'RESTORE_LAYOUT_CHANGED');
    await verifyBinding(loc, j, options); await verifyTree(staged, j.newTree, options);
    await move(staged, active, options); await verifyTree(active, j.newTree, options);
  } else {
    require(j.oldTree && current !== old, 'RESTORE_DATA_MISSING');
    if (old) {await verifyTree(previous, j.oldTree!, options); await move(previous, active, options);}
    await verifyTree(active, j.oldTree!, options); return finishJournal(loc, doc, 'rolled-back', options);
  }
  await authorized(options); await writeDocument(loc.promoted, receipt(j, doc.hash, 'promoted'), options);
  return {state: 'promoted', transactionID: j.transactionID, pendingPrevious: j.oldTree !== null};
}

/** Caller must hold its ordinary single-instance/structural storage-transition gate. */
export async function installRestoredAccount(root: string, account: string, staged: string, options: RestoreTransactionOptions): Promise<RestoreTransactionResult> {
  return serial(root, account, async () => {
    const loc = await location(root, account, options);
    for (const file of [loc.journal, loc.promoted, loc.gc, loc.collected, loc.rollback]) require(!await exists(file), 'PENDING_RESTORE_TRANSACTION');
    require(path.dirname(staged) === loc.accounts && /^\.account-[a-f0-9-]{36}$/.test(path.basename(staged)) && UUID.test(path.basename(staged).slice(9)), 'OWNED_STAGE_REQUIRED');
    await directory(staged); const binding = await rootBinding(loc, options);
    const rollbackBase = path.join(root, '.restore-rollback'); await ensureDirectory(rollbackBase, root, options);
    const transactionID = randomUUID(), previous = path.join(rollbackBase, account + '-' + transactionID);
    require(!await exists(previous), 'RESTORE_DESTINATION_OCCUPIED');
    const oldTree = await exists(activeRoot(loc)) ? await tree(activeRoot(loc), options) : null;
    const newTree = await tree(staged, options);
    require(newTree.entries.some(e => e.relative === 'master.wrap' && !e.directory) &&
      newTree.entries.some(e => e.relative === 'vault' && e.directory), 'COMPLETE_ACCOUNT_STAGE_REQUIRED');
    await flushStage(staged, newTree, options);
    const journal: Journal = {format: 'PUTY-RESTORE-TRANSACTION-1', root, account, transactionID,
      stage: path.basename(staged), previous: `.restore-rollback/${account}-${transactionID}`, binding, oldTree, newTree};
    await verifyBinding(loc, journal, options); await writeDocument(loc.journal, journal, options);
    return recover(loc, options);
  });
}

/** Run after verified online SSO and BEFORE mkdir/open/create-key for the account. */
export async function recoverRestoreTransaction(root: string, account: string, options: RestoreTransactionOptions): Promise<RestoreTransactionResult> {
  return serial(root, account, async () => recover(await location(root, account, options), options));
}

/** Only exact old receipt entries are deleted. Unknown/changed entries retain pending state. */
export async function collectRestorePrevious(root: string, account: string, options: RestoreTransactionOptions): Promise<RestoreTransactionResult> {
  return serial(root, account, async () => {
    let transactionID: string | undefined;
    try {
      await authorized(options, true); const loc = await location(root, account, options);
      const status = await recover(loc, options); transactionID = status.transactionID;
      if (status.state !== 'promoted') return status;
      const doc = await readDocument<Journal>(loc.journal, options), j = doc.value;
      validateJournal(j, loc); await verifyBinding(loc, j, options);
      await validateReceipt(loc.promoted, j, doc.hash, 'promoted', options);
      require(equalID(await directory(activeRoot(loc)), treeID(j.newTree)), 'ACTIVE_ROOT_CHANGED');
      const previous = previousRoot(loc, j), resuming = await exists(loc.gc);
      if (resuming) await validateReceipt(loc.gc, j, doc.hash, 'gc', options);
      if (j.oldTree) await verifyTree(previous, j.oldTree, options, resuming);
      else require(!await exists(previous), 'RESTORE_LAYOUT_CHANGED');
      await authorized(options, true);
      if (!resuming) await writeDocument(loc.gc, receipt(j, doc.hash, 'gc'), options);
      if (j.oldTree && await exists(previous)) {
        for (const entry of j.oldTree.entries.filter(e => !e.directory)) {
          const file = resolveEntry(previous, entry.relative); if (!await exists(file)) continue;
          await authorized(options, true); await verifyBinding(loc, j, options);
          await directory(path.dirname(file));
          require(canonical(await fileEntry(file, entry.relative, options)) === canonical(entry), 'TREE_CHANGED');
          await authorized(options, true); require(canonical(await fileEntry(file, entry.relative, options)) === canonical(entry), 'TREE_CHANGED');
          await fs.unlink(file); await syncDirectory(path.dirname(file)); await authorized(options, true);
        }
        const directories = j.oldTree.entries.filter(e => e.directory).sort((a, b) => b.relative.length - a.relative.length);
        for (const entry of directories) {
          const folder = resolveEntry(previous, entry.relative); if (!await exists(folder)) continue;
          await authorized(options, true); require(equalID(await directory(folder), entry.identity), 'TREE_CHANGED');
          require((await fs.readdir(folder)).length === 0, 'TREE_CHANGED'); await fs.rmdir(folder);
          await syncDirectory(path.dirname(folder)); await authorized(options, true);
        }
      }
      require(!await exists(previous), 'PREVIOUS_COLLECTION_PENDING');
      await authorized(options, true); return await finishJournal(loc, doc, 'collected', options);
    } catch (error) {
      const e = error as NodeJS.ErrnoException;
      return {state: 'pending', transactionID, pendingPrevious: true,
        reasonCode: error instanceof RestoreError ? error.code : ['EACCES', 'EPERM', 'ENOSPC', 'EIO', 'EBUSY', 'ENOTEMPTY', 'ENOENT'].includes(e.code || '') ? e.code : 'RESTORE_COLLECTION_REJECTED'};
    }
  });
}
