import { CheckOutlined } from "@ant-design/icons";
import { Spin } from "antd";
import i18n, { t } from "i18next";
import { GroupVerificationType } from "@openim/wasm-client-sdk";
import { memo, useState } from "react";

export const verificationMenuList = [
  {
    title: t("placeholder.applyNeedInvite"),
    type: GroupVerificationType.ApplyNeedInviteNot,
  },
  {
    title: t("placeholder.applyNeedVerification"),
    type: GroupVerificationType.AllNeed,
  },
  {
    title: t("placeholder.applyAll"),
    type: GroupVerificationType.AllNot,
  },
];

i18n.on("languageChanged", () => {
  verificationMenuList[0].title = t("placeholder.applyNeedInvite");
  verificationMenuList[1].title = t("placeholder.applyNeedVerification");
  verificationMenuList[2].title = t("placeholder.applyAll");
});

const VerificationSelectContent = memo(
  ({
    activeType,
    tryChange,
  }: {
    activeType?: GroupVerificationType;
    tryChange: (type: GroupVerificationType) => Promise<void>;
  }) => {
    const [loading, setLoading] = useState(false);

    return (
      <Spin spinning={loading}>
        <div className="p-1">
          {verificationMenuList.map((item) => (
            <div
              className="flex cursor-pointer items-center rounded p-3 pr-2 text-xs hover:bg-[var(--primary-active)]"
              key={item.type}
              onClick={async () => {
                if (item.type !== activeType) {
                  setLoading(true);
                  await tryChange(item.type);
                  setLoading(false);
                }
              }}
            >
              <div className="w-40">{item.title}</div>
              {activeType === item.type && (
                <CheckOutlined className="text-[var(--primary)]" rev={undefined} />
              )}
            </div>
          ))}
        </div>
      </Spin>
    );
  },
);

export default VerificationSelectContent;
