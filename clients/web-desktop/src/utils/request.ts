import axios from "axios";
import { getEnterpriseUrl } from "@/config";
import { getChatToken, getIMToken } from "./storage";
import { v4 as uuidv4 } from "uuid";

const createAxiosInstance = (baseURL: string, imToken = true) => {
  const serves = axios.create({
    baseURL,
    timeout: 25000,
  });

  serves.interceptors.request.use(
    async (config) => {
      if (/^\/(office|rtc|signaling|enterprise)\//.test(config.url || "")) {
        config.baseURL = getEnterpriseUrl();
        config.headers.token = String(await getIMToken() || "");
        config.headers.chatToken = String(await getChatToken() || "");
        config.headers.platformID = String(window.electronAPI?.getPlatform() || 5);
      }
      const token = imToken ? await getIMToken() : await getChatToken();
      config.headers.token = config.headers.token ?? token;
      config.headers.operationID = uuidv4();
      return config;
    },
    (err) => Promise.reject(err),
  );

  serves.interceptors.response.use(
    (res) => {
      if (res.data.errCode !== 0) {
        return Promise.reject(res.data);
      }
      return res.data;
    },
    (err) => {
      if (err.message.includes("timeout")) {
        console.error("error", err);
      }
      if (err.message.includes("Network Error")) {
        console.error("error", err);
      }
      return Promise.reject(err);
    },
  );

  return serves;
};

export default createAxiosInstance;
