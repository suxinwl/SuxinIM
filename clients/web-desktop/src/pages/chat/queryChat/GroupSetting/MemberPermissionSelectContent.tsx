import { CheckOutlined } from "@ant-design/icons";
import { Spin } from "antd";
import { t } from "i18next";
import { AllowType } from "@openim/wasm-client-sdk";
import { memo, useState } from "react";

import { PermissionField } from "./useGroupSettings";

const memberPermissionList = [
  {
    title: t("placeholder.forbidLookMemberInfo"),
    field: "lookMemberInfo",
  },
  {
    title: t("placeholder.forbidAddMember"),
    field: "applyMemberFriend",
  },
];

const MemberPermissionSelectContent = memo(
  ({
    applyMemberFriend,
    lookMemberInfo,
    tryChange,
  }: {
    applyMemberFriend?: AllowType;
    lookMemberInfo?: AllowType;
    tryChange: (rule: AllowType, field: PermissionField) => Promise<void>;
  }) => {
    const [loading, setLoading] = useState(false);

    return (
      <Spin spinning={loading}>
        <div className="p-1">
          {memberPermissionList.map((item) => {
            const rule =
              item.field === "applyMemberFriend" ? applyMemberFriend : lookMemberInfo;
            return (
              <div
                className="flex cursor-pointer items-center rounded p-3 pr-1 text-xs hover:bg-[var(--primary-active)]"
                key={item.field}
                onClick={async () => {
                  setLoading(true);
                  await tryChange(Number(!rule), item.field as PermissionField);
                  setLoading(false);
                }}
              >
                <div className="w-44">{item.title}</div>
                <div className="w-4">
                  {rule === AllowType.NotAllowed && (
                    <CheckOutlined className="text-[var(--primary)]" rev={undefined} />
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </Spin>
    );
  },
);

export default MemberPermissionSelectContent;
