import fs from "fs";
import { join, normalize } from "node:path";
import sudo from "sudo-prompt";
import zipUtil from "adm-zip";

import { isWin } from "../utils";
import { logger } from ".";

export const asarHotUpdate = (pkgPath: string) =>
  extractWithElevation(pkgPath, global.pathConfig.asarPath);

const extractWithElevation = (zipPath: string, outputPath: string) => {
  const canWrite = checkWritePermission(outputPath);
  logger.error("canWrite", canWrite);
  if (canWrite) {
    try {
      const task = new zipUtil(zipPath);
      task.extractAllTo(outputPath, true);
      return Promise.resolve(true);
    } catch (error) {
      logger.error("extractWithElevation error");
      logger.error(error);
      return Promise.reject(false);
    }
  }
  const scriptPath = join(__dirname, "../scripts/unzipTask.js");
  return new Promise<boolean>((resolve, reject) => {
    const electronExecutable = process.execPath;
    const command = `"${normalize(electronExecutable)}" "${normalize(
      scriptPath,
    )}" "${normalize(zipPath)}" "${normalize(outputPath)}"`;
    logger.error("command", command);
    const env = getCleanedEnv();
    sudo.exec(
      command,
      {
        name: "OpenIM",
        env: {
          ...env,
          ELECTRON_RUN_AS_NODE: "1",
        },
      },
      (error, stdout, stderr) => {
        if (error) {
          logger.error("extractWithElevation error");
          logger.error(error, stdout, stderr);
          reject(false);
          return;
        }
        resolve(true);
      },
    );
  });
};

const checkWritePermission = (path: string) => {
  try {
    if (isWin) {
      const fd = fs.openSync(path, "w");
      fs.closeSync(fd);
    } else {
      fs.accessSync(path, fs.constants.W_OK);
    }
    return true;
  } catch (err) {
    return false;
  }
};

const getCleanedEnv = () => {
  const cleanedEnv = { ...process.env };
  for (const key of Object.keys(cleanedEnv)) {
    if (/[()]/.test(key)) {
      delete cleanedEnv[key];
    }
  }
  return cleanedEnv;
};
