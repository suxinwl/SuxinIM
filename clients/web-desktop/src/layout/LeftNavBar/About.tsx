import { CloseOutlined, RightOutlined } from "@ant-design/icons";
import { Divider, Form, Modal, Spin } from "antd";
import { t } from "i18next";
import { forwardRef, ForwardRefRenderFunction, memo, useState } from "react";

import logo from "@/assets/images/profile/logo.png";
import { APP_VERSION, SDK_VERSION } from "@/config";
import { appDisplayName } from "@/config/brand";
import { useUserStore } from "@/store";
import { logReport } from "@/routes/GlobalErrorElement";

import { OverlayVisibleHandle, useOverlayVisible } from "../../hooks/useOverlayVisible";

const About: ForwardRefRenderFunction<OverlayVisibleHandle, unknown> = (_, ref) => {
  const [form] = Form.useForm();

  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  return (
    <Modal
      title={null}
      footer={null}
      closable={false}
      open={isOverlayOpen}
      centered
      onCancel={closeOverlay}
      afterClose={() => form.resetFields()}
      styles={{
        mask: {
          opacity: 0,
          transition: "none",
        },
      }}
      width={360}
      className="no-padding-modal"
      maskTransitionName=""
    >
      <AboutContent closeOverlay={closeOverlay} />
    </Modal>
  );
};

export default memo(forwardRef(About));

export const AboutContent = ({ closeOverlay }: { closeOverlay?: () => void }) => {
  const displayName = useUserStore(state => appDisplayName(state.appConfig));
  const [loading, setLoading] = useState(false);
  const tryLogReport = async () => {
    setLoading(true);
    await logReport();
    setLoading(false);
  };

  return (
    <Spin spinning={loading} tip={t("placeholder.uploadToast")}>
      <div className="bg-[var(--chat-bubble)]">
        <div className="flex items-center justify-between bg-[var(--gap-text)] p-5">
          <span className="text-base font-medium">{t("placeholder.about")}</span>
          <CloseOutlined
            className="app-no-drag cursor-pointer text-[#8e9aaf]"
            rev={undefined}
            onClick={closeOverlay}
          />
        </div>
        <div className="flex flex-col items-center justify-center">
          <img className="mb-2 mt-7" width={56} src={logo} alt="" />
          <div className="mb-5 flex flex-col items-center">
            <div className="font-medium">{displayName}</div>
            <div>{APP_VERSION}</div>
            <div>{SDK_VERSION}</div>
          </div>
        </div>

        <Divider className="border-1 m-0 border-[var(--gap-text)]" />

        {window.electronAPI && (
          <>
            <div
              className="flex cursor-pointer items-center justify-between border-b border-[var(--gap-text)] px-3 py-2"
              onClick={tryLogReport}
            >
              <div>{t("placeholder.reportLog")}</div>
              <RightOutlined rev={undefined} />
            </div>
            <div
              className="flex cursor-pointer items-center justify-between px-3 py-2"
              onClick={() => window.electronAPI?.ipcInvoke("showLogsInFinder")}
            >
              <div>{t("placeholder.viewingLocalLogs")}</div>
              <RightOutlined rev={undefined} />
            </div>
          </>
        )}

        {/* <div className="flex cursor-pointer items-center justify-between px-6 py-4">
        <div className="text-base">{t("placeholder.checkNewVersion")}</div>
        <RightOutlined rev={undefined} />
      </div> */}
      </div>
    </Spin>
  );
};
