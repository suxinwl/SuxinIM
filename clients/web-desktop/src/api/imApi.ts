import { v4 as uuidv4 } from "uuid";

import { getApiUrl, SDK_VERSION } from "@/config";
import createAxiosInstance from "@/utils/request";
import { getIMToken } from "@/utils/storage";

const request = createAxiosInstance(getApiUrl());

interface FileItem {
  filename: string;
  url: string;
}

export const uploadLogs = async (fileURLs: FileItem[]) => {
  const token = (await getIMToken()) as string;
  return request<unknown>({
    url: "/third/logs/upload",
    method: "POST",
    data: {
      platform: window.electronAPI?.getPlatform() ?? 5,
      version: SDK_VERSION,
      systemType: window.electronAPI?.getSystemVersion() ?? "",
      ex: "",
      fileURLs,
    },
    headers: {
      token,
      operationID: uuidv4(),
    },
  });
};
