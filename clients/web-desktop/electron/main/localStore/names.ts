import path from "node:path";
import { promises as fs } from "node:fs";
export function safeSourceName(source: string): string {
  let name = path.win32.basename(path.posix.basename(source)).replace(/[<>:"/\\|?*\x00-\x1f]/g, "_").replace(/[. ]+$/, "");
  if (!name || name === "." || name === "..") name = "attachment";
  if (/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)/i.test(name)) name = "_" + name;
  if ((process.platform==='win32'?name.length:Buffer.byteLength(name,"utf8")) > 255) throw new Error("文件名超出当前文件系统长度限制，请缩短后保存");
  return name;
}
export function numberedName(source: string, number: number): string {
  const name = safeSourceName(source), ext = path.extname(name), base = ext ? name.slice(0, -ext.length) : name;
  const candidate=number===0?name:`${base}（${number}）${ext}`;
  if((process.platform==='win32'?candidate.length:Buffer.byteLength(candidate,'utf8'))>255)throw new Error('重名编号后文件名超出系统长度限制，请缩短源文件名后保存');
  return candidate;
}
export function shanghaiMonth(timestampMS = Date.now()): string {
  if (!Number.isFinite(timestampMS)) throw new Error("Invalid archive date");
  const date = new Date(timestampMS + 8 * 3600 * 1000);
  return `${date.getUTCFullYear()}${String(date.getUTCMonth() + 1).padStart(2, "0")}`;
}
export function beforeShanghaiDate(value: string): number {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) throw new Error("请选择有效日期");
  const result = Date.parse(`${value}T00:00:00+08:00`);
  if (!Number.isFinite(result) || new Date(result + 8 * 3600 * 1000).toISOString().slice(0, 10) !== value) throw new Error("请选择有效日期");
  return result;
}
// Exclusive creation reserves names for in-flight transfers as well as complete
// files. Never use exists()->write, which races across concurrent downloads.
export async function reserveName(directory: string, source: string): Promise<{ path: string; name: string }> {
  await fs.mkdir(directory, { recursive: true });
  for (let n = 0; n < 100000; n++) {
    const name = numberedName(source, n), target = path.join(directory, name);
    try {
      const handle = await fs.open(target, "wx", 0o600); let owned: {dev:number;ino:number}|undefined;
      try { try { owned = await handle.stat(); await handle.sync(); } finally { await handle.close(); } }
      catch (error) {
        const current = await fs.lstat(target).catch(() => null);
        if (owned && current?.isFile() && !current.isSymbolicLink() && current.size === 0 && current.dev === owned.dev && current.ino === owned.ino) await fs.unlink(target);
        throw error;
      }
      return { path: target, name };
    }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error; }
  }
  throw new Error("同名文件过多，无法分配存档名称");
}
