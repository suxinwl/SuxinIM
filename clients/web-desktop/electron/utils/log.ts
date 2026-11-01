import log from "electron-log/main";
import { join } from "node:path";
import fs from "fs";

const getLogger = (logsPath: string) => {
  log.transports.file.level = "debug";
  log.transports.file.maxSize = 52428800; // max size 50M
  log.transports.file.format = "[{y}-{m}-{d} {h}:{i}:{s}.{ms}] [{level}]{scope} {text}";
  let date = new Date();
  let dateStr = date.getFullYear() + "-" + (date.getMonth() + 1) + "-" + date.getDate();
  log.transports.file.resolvePathFn = () => join(logsPath, `log${dateStr}.log`);
  log.initialize({ preload: true });
  return log.scope("ipcMain");
};

const checkClearLogs = async () => {
  const logsPath = global.pathConfig.logsPath;
  const today = new Date();
  let files: string[] = [];
  try {
    files = await fs.promises.readdir(logsPath);
  } catch (err) {
    if (err.code === "ENOENT") {
      log.error(`Error: ${logsPath} does not exist`);
    } else {
      log.error(`Error reading directory ${logsPath}: ${err}`);
    }
    return;
  }
  await Promise.all(
    files.map(async (file) => {
      const filePath = join(logsPath, file);
      const stats = await fs.promises.stat(filePath);
      const fileDate = new Date(stats.mtime);
      if (
        fileDate.getFullYear() < today.getFullYear() ||
        fileDate.getMonth() < today.getMonth() ||
        fileDate.getDate() < today.getDate()
      ) {
        await fs.promises.unlink(filePath);
      }
    }),
  );
};

export { getLogger, checkClearLogs };
