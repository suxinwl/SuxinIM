import { Button, Result } from "antd";
import { t } from "i18next";
import { useState } from "react";
import { useRouteError } from "react-router-dom";
import { v4 as uuidV4 } from "uuid";

import { uploadLogs } from "@/api/imApi";
import { IMSDK } from "@/layout/MainContentWrap";

const GlobalErrorElement = () => {
  const [loading, setLoading] = useState(false);
  const error = useRouteError();

  const tryLogReport = () => {
    // setLoading(true);
    // await logReport();
    // setLoading(false);
    window.location.reload();
  };

  console.error("GlobalErrorElement");
  console.error(error);

  return (
    <div className="flex h-full w-full items-center justify-center">
      <Result
        status="404"
        subTitle={t("toast.somethingError")}
        extra={
          <Button type="primary" loading={loading} onClick={tryLogReport}>
            {t("placeholder.recover")}
          </Button>
        }
      />
    </div>
  );
};

export default GlobalErrorElement;

export const logReport = async () => {
  if (!window.electronAPI) return null;
  try {
    const filePath = await window.electronAPI?.ipcInvoke("prepareUploadLogs");
    const logsFile = await window.electronAPI?.getFileByPath(filePath as string);
    if (!logsFile) {
      throw new Error(t("toast.getLogFailed"));
    }
    const {
      data: { url },
    } = await IMSDK.uploadFile({
      name: logsFile.name,
      contentType: "application/zip",
      uuid: uuidV4(),
      file: logsFile,
    });
    IMSDK.uploadFile;
    await uploadLogs([
      {
        url,
        filename: logsFile.name,
      },
    ]);
  } catch (error) {
    return error;
  }
  return null;
};
