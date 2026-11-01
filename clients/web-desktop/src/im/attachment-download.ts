import type { DownloadData, SaveType } from "../store/type";

export type AttachmentDownloadRequest = {
  url: string;
  saveType?: SaveType;
  randomPrefix?: string;
};

type DownloadPorts = {
  getTask: (url: string) => DownloadData | undefined;
  addTask: (url: string, task: DownloadData) => void;
  removeTask: (url: string) => void;
  start: (request: AttachmentDownloadRequest) => Promise<unknown>;
  randomPrefix: () => string;
};

// A storage URL may be signed, including every query parameter. Cache metadata
// belongs in IPC, never in the request URL sent to the attachment server.
export async function startAttachmentDownload(
  originUrl: string,
  data: DownloadData,
  ports: DownloadPorts,
) {
  const parsed = new URL(originUrl);
  if (
    !["https:", "http:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password
  ) {
    throw new Error("Unsupported attachment URL");
  }
  if (ports.getTask(originUrl)) return;
  ports.addTask(originUrl, {
    ...data,
    originUrl,
    downloadUrl: originUrl,
    downloadState: "downloading",
  });
  try {
    // Add the task before IPC: a tiny cached attachment can finish immediately.
    await ports.start({
      url: originUrl,
      saveType: data.saveType ?? "file",
      ...(data.randomName ? { randomPrefix: ports.randomPrefix() } : {}),
    });
  } catch (error) {
    ports.removeTask(originUrl);
    throw error;
  }
}
