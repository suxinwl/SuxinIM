import { Popover, Space, Tooltip } from "antd";
import { t } from "i18next";
import {
  FriendUserItem,
  PublicUserItem,
  SelfUserInfo,
} from "@/im/types";
import { memo, useState } from "react";

import { modal } from "@/AntdGlobalComp";
import bench from "@/assets/images/common/bench.png";
import call from "@/assets/images/common/call.png";
import cancel from "@/assets/images/common/cancel.png";
import card from "@/assets/images/common/card.png";
import { IMSDK } from "@/layout/MainContentWrap";
import CallPopContent from "@/pages/chat/queryChat/ChatFooter/SendActionBar/CallPopContent";
import { feedbackToast } from "@/utils/common";
import emitter from "@/utils/events";

const CardActionRow = ({
  isFriend,
  isSelf,
  cardInfo,
  closeOverlay,
}: {
  isFriend?: boolean;
  isSelf?: boolean;
  cardInfo?: Partial<SelfUserInfo & FriendUserItem>;
  closeOverlay: () => void;
}) => {
  const [callOptionsVisible, setCallOptionsVisible] = useState(false);
  const shareCard = () => {
    const cardMessageOptions = {
      userID: cardInfo?.userID ?? "",
      nickname: cardInfo?.nickname ?? "",
      faceURL: cardInfo?.faceURL ?? "",
      ex: cardInfo?.ex ?? "",
    };
    emitter.emit("OPEN_CHOOSE_MODAL", {
      type: "SHARE_CARD",
      extraData: cardMessageOptions,
    });
    closeOverlay();
  };

  const tryUnfriend = () => {
    modal.confirm({
      title: t("placeholder.unfriend"),
      content: t("toast.confirmUnfriend"),
      onOk: async () => {
        try {
          await IMSDK.deleteFriend(cardInfo!.userID!);
        } catch (error) {
          feedbackToast({ error, msg: t("toast.unfriendFailed") });
        }
      },
    });
    closeOverlay();
  };

  return (
    <div className="flex items-center">
      <Space size={4}>
        <Tooltip title={t("placeholder.share")} placement="bottom">
          <img
            className="cursor-pointer"
            width={18}
            src={card}
            alt=""
            onClick={shareCard}
          />
        </Tooltip>
        {isFriend && (
          <Tooltip title={t("placeholder.unfriend")} placement="bottom">
            <img
              className="cursor-pointer"
              width={18}
              src={cancel}
              alt=""
              onClick={tryUnfriend}
            />
          </Tooltip>
        )}
        {!isSelf && (
          <>
            <Popover
              content={
                <CallPopContent
                  userInfo={cardInfo as PublicUserItem}
                  callClick={() => setCallOptionsVisible(false)}
                />
              }
              arrow={false}
              title={null}
              trigger="click"
              placement="bottom"
              open={callOptionsVisible}
              onOpenChange={(vis) => setCallOptionsVisible(vis)}
            >
              <img className="cursor-pointer" width={18} src={call} alt="" />
            </Popover>
            {/* <Tooltip title={t("placeholder.moments")} placement="bottom">
              <img className="cursor-pointer" width={18} src={bench} alt="" />
            </Tooltip> */}
          </>
        )}
      </Space>
    </div>
  );
};

export default memo(CardActionRow);
