import { createCipheriv, createDecipheriv, hkdfSync, randomBytes, scryptSync } from "node:crypto";

export const FORMAT = "PUTY-LOCAL-STORE-1";
export const MAGIC = Buffer.from("PUTYLS1\n", "ascii");
export const CHUNK_SIZE = 1024 * 1024;
export type ObjectContext = { account: string; objectID: string; type: string };
export type ObjectHeader = ObjectContext & { format: string; chunkSize: number; noncePrefix: string };
export function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (value && typeof value === "object") return `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical((value as Record<string, unknown>)[key])}`).join(",")}}`;
  return JSON.stringify(value);
}
export function u32(value: number): Buffer {
  if (!Number.isInteger(value) || value < 0 || value > 0xffffffff) throw new Error("Invalid encrypted frame number");
  const bytes = Buffer.alloc(4); bytes.writeUInt32BE(value); return bytes;
}
export function objectKey(master: Uint8Array, context: ObjectContext): Buffer {
  if (master.length !== 32 || [context.account, context.objectID, context.type].some(value => typeof value !== "string" || !value || value.includes("\0"))) throw new Error("Invalid local encryption context");
  return Buffer.from(hkdfSync("sha256", master, Buffer.from(FORMAT), Buffer.from(`${context.account}\0${context.type}\0${context.objectID}`), 32));
}
export function createHeader(context: ObjectContext, prefix: Buffer = randomBytes(8)): { header: ObjectHeader; bytes: Buffer; prefix: Buffer } {
  if (prefix.length !== 8) throw new Error("Invalid nonce prefix");
  const header = { ...context, chunkSize: CHUNK_SIZE, format: FORMAT, noncePrefix: prefix.toString("base64") };
  const bytes = Buffer.from(canonical(header));
  return { header, bytes, prefix: Buffer.concat([MAGIC, u32(bytes.length), bytes]) };
}
export function parseHeader(input: Uint8Array): { header: ObjectHeader; bytes: Buffer; offset: number } {
  const data = Buffer.from(input);
  if (data.length < 12 || !data.subarray(0, 8).equals(MAGIC)) throw new Error("Invalid encrypted local file");
  const length = data.readUInt32BE(8);
  if (length < 2 || length > 65536 || data.length < 12 + length) throw new Error("Incomplete encrypted header");
  const bytes = data.subarray(12, 12 + length), header = JSON.parse(bytes.toString("utf8")) as ObjectHeader;
  if (header.format !== FORMAT || header.chunkSize !== CHUNK_SIZE || Buffer.from(header.noncePrefix, "base64").length !== 8 || canonical(header) !== bytes.toString("utf8")) throw new Error("Invalid local file format");
  return { header, bytes, offset: 12 + length };
}
export function encryptFrame(key: Uint8Array, header: ObjectHeader, headerBytes: Uint8Array, index: number, plain: Uint8Array): Buffer {
  if (plain.length > CHUNK_SIZE) throw new Error("Encrypted frame too large");
  const iv = Buffer.concat([Buffer.from(header.noncePrefix, "base64"), u32(index)]);
  const cipher = createCipheriv("aes-256-gcm", key, iv);
  cipher.setAAD(Buffer.concat([Buffer.from(headerBytes), u32(index), u32(plain.length)]));
  const ciphertext = Buffer.concat([cipher.update(plain), cipher.final()]);
  return Buffer.concat([u32(ciphertext.length), ciphertext, cipher.getAuthTag()]);
}
export function decryptFrame(key: Uint8Array, header: ObjectHeader, headerBytes: Uint8Array, index: number, ciphertext: Uint8Array, tag: Uint8Array): Buffer {
  if (ciphertext.length > CHUNK_SIZE || tag.length !== 16) throw new Error("Invalid encrypted frame");
  const cipher = createDecipheriv("aes-256-gcm", key, Buffer.concat([Buffer.from(header.noncePrefix, "base64"), u32(index)]));
  cipher.setAAD(Buffer.concat([Buffer.from(headerBytes), u32(index), u32(ciphertext.length)]));
  cipher.setAuthTag(tag);
  return Buffer.concat([cipher.update(ciphertext), cipher.final()]);
}
export function seal(master: Uint8Array, context: ObjectContext, plain: Uint8Array, noncePrefix?: Buffer): Buffer {
  const { header, bytes, prefix } = createHeader(context, noncePrefix), key = objectKey(master, context);
  const parts: Buffer[] = [prefix];
  let index = 0;
  for (let offset = 0; offset < plain.length; offset += CHUNK_SIZE) parts.push(encryptFrame(key, header, bytes, index++, plain.subarray(offset, offset + CHUNK_SIZE)));
  parts.push(encryptFrame(key, header, bytes, index, Buffer.alloc(0)));
  key.fill(0); return Buffer.concat(parts);
}
export function unseal(master: Uint8Array, context: ObjectContext, input: Uint8Array): Buffer {
  const data = Buffer.from(input), { header, bytes, offset } = parseHeader(data);
  if (header.account !== context.account || header.objectID !== context.objectID || header.type !== context.type) throw new Error("Local file belongs to another context");
  const key = objectKey(master, context), result: Buffer[] = [];
  let cursor = offset, index = 0, ended = false;
  try {
    while (cursor < data.length) {
      if (cursor + 4 > data.length) throw new Error("Truncated encrypted file");
      const length = data.readUInt32BE(cursor); cursor += 4;
      if (length > CHUNK_SIZE || cursor + length + 16 > data.length) throw new Error("Truncated encrypted frame");
      const plain = decryptFrame(key, header, bytes, index++, data.subarray(cursor, cursor + length), data.subarray(cursor + length, cursor + length + 16)); cursor += length + 16;
      if (length === 0) { ended = true; break; }
      result.push(plain);
    }
    if (!ended || cursor !== data.length) throw new Error("Encrypted end marker missing or trailing data");
    return Buffer.concat(result);
  } finally { key.fill(0); }
}
export function exportRecovery(master: Uint8Array, account: string, passphrase: string): Buffer {
  if (passphrase.length < 12) throw new Error("恢复口令至少需要 12 个字符");
  const salt = randomBytes(16), key = scryptSync(passphrase, salt, 32, { N: 32768, r: 8, p: 1, maxmem: 64 * 1024 * 1024 });
  try { return Buffer.from(JSON.stringify({ format: FORMAT, account, kdf: "scrypt-32768-8-1", salt: salt.toString("base64"), data: seal(key, { account, type: "recovery", objectID: "master-key" }, master).toString("base64") })); }
  finally { key.fill(0); }
}
export function importRecovery(input: Uint8Array, account: string, passphrase: string): Buffer {
  const envelope = JSON.parse(Buffer.from(input).toString("utf8"));
  if (envelope.format !== FORMAT || envelope.account !== account || envelope.kdf !== "scrypt-32768-8-1") throw new Error("恢复包不属于当前账号或格式不支持");
  const salt = Buffer.from(envelope.salt, "base64"); if (salt.length !== 16) throw new Error("Invalid recovery salt");
  const key = scryptSync(passphrase, salt, 32, { N: 32768, r: 8, p: 1, maxmem: 64 * 1024 * 1024 });
  try { const master = unseal(key, { account, type: "recovery", objectID: "master-key" }, Buffer.from(envelope.data, "base64")); if (master.length !== 32) throw new Error("Invalid restored key"); return master; }
  finally { key.fill(0); }
}
