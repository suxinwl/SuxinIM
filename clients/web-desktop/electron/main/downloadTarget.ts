import path from "node:path";
import type { AttachmentDownloadRequest } from "../../src/im/attachment-download";

const cacheTypes = new Set(["image", "video", "avatar", "file"]);

export function normalizeDownloadRequest(input: unknown): AttachmentDownloadRequest {
  // Retain compatibility with an older renderer without interpreting URL query
  // parameters as local filesystem instructions.
  const request = typeof input === "string" ? { url: input } : input;
  if (
    !request ||
    typeof request !== "object" ||
    !("url" in request) ||
    typeof request.url !== "string"
  ) {
    throw new Error("Invalid attachment download request");
  }
  const parsed = new URL(request.url);
  if (
    !["https:", "http:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password
  ) {
    throw new Error("Unsupported attachment URL");
  }
  const value = request as AttachmentDownloadRequest;
  if (
    value.randomPrefix !== undefined &&
    !/^[a-zA-Z0-9-]{1,80}$/.test(value.randomPrefix)
  ) {
    throw new Error("Invalid attachment cache prefix");
  }
  return {
    url: value.url,
    saveType: cacheTypes.has(value.saveType ?? "") ? value.saveType : "file",
    ...(value.randomPrefix ? { randomPrefix: value.randomPrefix } : {}),
  };
}

export function downloadRequestKey(url: string) {
  return new URL(url).toString();
}

export function attachmentSavePath(directory: string, filename: string, prefix = "") {
  if (!directory) throw new Error("Attachment cache is not initialized");
  // Content-Disposition is remote input. It must never choose a directory.
  const safeName =
    path.win32
      .basename(path.posix.basename(filename))
      .replace(/[<>:"/\\|?*\x00-\x1f]/g, "_")
      .replace(/[. ]+$/, "") || "attachment";
  const target = path.resolve(directory, `${prefix}${safeName}`);
  const relative = path.relative(path.resolve(directory), target);
  if (!relative || relative.startsWith("..") || path.isAbsolute(relative)) {
    throw new Error("Invalid attachment cache target");
  }
  return target;
}
