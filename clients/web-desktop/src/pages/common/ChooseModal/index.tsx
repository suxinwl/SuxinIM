import { CloseOutlined } from "@ant-design/icons";
import { Button, Input, Modal, Upload } from "antd";
import i18n, { t } from "i18next";
import { GroupType, SessionType } from "@openim/wasm-client-sdk";
import { CardElem } from "@/im/types";
import { CustomMsgParams, MergerMsgParams } from "@openim/wasm-client-sdk/lib/types/params";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useEffect,
  useRef,
  useState,
} from "react";
import { v4 as uuidV4 } from "uuid";

import { message } from "@/AntdGlobalComp";
import OIMAvatar from "@/components/OIMAvatar";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { forwardBatch, type ForwardProgress } from "@/im/forward-batch";
import { IMSDK } from "@/layout/MainContentWrap";
import { useSendMessage } from "@/pages/chat/queryChat/ChatFooter/useSendMessage";
import { ExMessageItem, useConversationStore, useUserStore } from "@/store";
import { feedbackToast } from "@/utils/common";
import emitter from "@/utils/events";
import { formatMessageByType } from "@/utils/imCommon";

import ChooseBox, { ChooseBoxHandle } from "./ChooseBox";
import { CheckListItem } from "./ChooseBox/CheckItem";

export type ChooseModalType =
  | "CRATE_GROUP"
  | "INVITE_TO_GROUP"
  | "KICK_FORM_GROUP"
  | "TRANSFER_IN_GROUP"
  | "FORWARD_MESSAGE"
  | "SELECT_CARD"
  | "SHARE_CARD"
  | "SELECT_USER"
  | "RTC_INVITE"
  | "MEETING_INVITE";

export interface SelectUserExtraData {
  notConversation: boolean;
  list: CheckListItem[];
}

export interface ChooseModalState {
  type: ChooseModalType;
  extraData?: unknown;
}

interface IChooseModalProps {
  state: ChooseModalState;
}

const titleMap = {
  CRATE_GROUP: t("placeholder.createGroup"),
  INVITE_TO_GROUP: t("placeholder.invitation"),
  KICK_FORM_GROUP: t("placeholder.kickMember"),
  TRANSFER_IN_GROUP: t("placeholder.transferGroup"),
  FORWARD_MESSAGE: t("placeholder.mergeForward"),
  SELECT_CARD: t("placeholder.share"),
  SHARE_CARD: t("placeholder.share"),
  SELECT_USER: t("placeholder.selectUser"),
  RTC_INVITE: t("placeholder.selectUser"),
  MEETING_INVITE: t("placeholder.selectUser"),
};

i18n.on("languageChanged", () => {
  titleMap.CRATE_GROUP = t("placeholder.createGroup");
  titleMap.INVITE_TO_GROUP = t("placeholder.invitation");
  titleMap.KICK_FORM_GROUP = t("placeholder.kickMember");
  titleMap.TRANSFER_IN_GROUP = t("placeholder.transferGroup");
  titleMap.FORWARD_MESSAGE = t("placeholder.mergeForward");
  titleMap.SELECT_CARD = t("placeholder.share");
  titleMap.SHARE_CARD = t("placeholder.share");
  titleMap.SELECT_USER = t("placeholder.selectUser");
  titleMap.RTC_INVITE = t("placeholder.share");
  titleMap.MEETING_INVITE = t("placeholder.selectUser");
});

const showConversationTypes = ["FORWARD_MESSAGE", "SHARE_CARD", "MEETING_INVITE"];
const onlyOneTypes = ["TRANSFER_IN_GROUP", "SELECT_CARD"];
const onlyMemberTypes = ["KICK_FORM_GROUP", "TRANSFER_IN_GROUP", "RTC_INVITE"];

const ChooseModal: ForwardRefRenderFunction<OverlayVisibleHandle, IChooseModalProps> = (
  { state: { type, extraData } },
  ref,
) => {
  const chooseBoxRef = useRef<ChooseBoxHandle>(null);
  const [loading, setLoading] = useState(false);
  const confirming = useRef(false);
  const forwardProgress = useRef(new Map<string, ForwardProgress>());
  const [groupBaseInfo, setGroupBaseInfo] = useState({
    groupName: "",
    groupAvatar: "",
  });

  const { sendMessage } = useSendMessage();
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  useEffect(() => {
    if (isOverlayOpen && type === "CRATE_GROUP" && extraData) {
      setTimeout(
        () => chooseBoxRef.current?.updatePrevCheckList(extraData as CheckListItem[]),
        100,
      );
    }
    if (isOverlayOpen && type === "SELECT_USER" && extraData) {
      setTimeout(
        () =>
          chooseBoxRef.current?.updatePrevCheckList(
            (extraData as SelectUserExtraData).list,
          ),
        100,
      );
    }
  }, [isOverlayOpen]);

  const confirmChoose = async () => {
    if (confirming.current) return;
    const choosedList = chooseBoxRef.current?.getCheckedList() ?? [];
    if (!choosedList?.length && type !== "SELECT_USER")
      return message.warning(t("toast.selectLeastOne"));

    if (!groupBaseInfo.groupName && type === "CRATE_GROUP")
      return message.warning(t("toast.inputGroupName"));

    confirming.current = true;
    setLoading(true);
    try {
      switch (type) {
        case "CRATE_GROUP":
          await IMSDK.createGroup({
            groupInfo: {
              groupType: GroupType.WorkingGroup,
              groupName: groupBaseInfo.groupName,
              faceURL: groupBaseInfo.groupAvatar,
            },
            memberUserIDs: choosedList.map((item) => item.userID!),
            adminUserIDs: [],
          });
          break;
        case "INVITE_TO_GROUP":
          await IMSDK.inviteUserToGroup({
            groupID: extraData as string,
            userIDList: choosedList.map((item) => item.userID!),
            reason: "",
          });
          break;
        case "KICK_FORM_GROUP":
          await IMSDK.kickGroupMember({
            groupID: extraData as string,
            userIDList: choosedList.map((item) => item.userID!),
            reason: "",
          });
          break;
        case "TRANSFER_IN_GROUP":
          await IMSDK.transferGroupOwner({
            groupID: extraData as string,
            newOwnerUserID: choosedList[0].userID!,
          });
          break;
        case "SELECT_CARD":
          sendMessage({
            message: (
              await IMSDK.createCardMessage({
                userID: choosedList[0].userID!,
                nickname: choosedList[0].nickname ?? "",
                faceURL: choosedList[0].faceURL ?? "",
                ex: choosedList[0].ex ?? "",
              })
            ).data,
          });
          break;
        case "FORWARD_MESSAGE":
        case "SHARE_CARD":
        case "MEETING_INVITE":
          const result = await forwardBatch({
            targets: choosedList,
            additional: chooseBoxRef.current?.getAdditional() ?? "",
            progress: forwardProgress.current,
            createMessage: getBatchMessage,
            createAdditional: async (text) => (await IMSDK.createTextMessage(text)).data,
            isJoinGroup: async (groupID) => (await IMSDK.isJoinGroup<boolean>(groupID)).data === true,
            send: (message, item) => sendMessage({
              message,
              recvID: item.userID ?? "",
              groupID: item.groupID ?? "",
              throwOnError: true,
            }),
          });
          if (result.failures.length) {
            chooseBoxRef.current?.updatePrevCheckList(result.failures.map((failure) => failure.target));
            feedbackToast({ error: true, msg: t("toast.forwardFailed", { succeeded: result.successful.length, failed: result.failures.length }) });
            return;
          }
          message.success(t("toast.sendSuccess"));
          break;
        case "SELECT_USER":
          emitter.emit("SELECT_USER", {
            notConversation: (extraData as SelectUserExtraData).notConversation,
            choosedList,
          });
          break;
        case "RTC_INVITE":
          emitter.emit("OPEN_RTC_MODAL", {
            invitation: {
              inviterUserID: useUserStore.getState().selfInfo.userID,
              inviteeUserIDList: choosedList.map((item) => item.userID!),
              groupID:
                useConversationStore.getState().currentConversation?.groupID ?? "",
              roomID:
                useConversationStore.getState().currentConversation?.groupID ??
                uuidV4(),
              timeout: 60,
              mediaType: extraData as string,
              sessionType: SessionType.WorkingGroup,
              platformID: window.electronAPI?.getPlatform() ?? 5,
            },
          });
          break;
        default:
          break;
      }
    } catch (error) {
      feedbackToast({ error });
      return;
    } finally {
      confirming.current = false;
      setLoading(false);
    }
    closeOverlay();
  };

  const getBatchMessage = async () => {
    if ((extraData as MergerMsgParams).title) {
      return (await IMSDK.createMergerMessage(extraData as MergerMsgParams)).data;
    }
    if ((extraData as ExMessageItem).clientMsgID) {
      return (await IMSDK.createForwardMessage(extraData as ExMessageItem)).data;
    }
    if ((extraData as CustomMsgParams).data) {
      return (await IMSDK.createCustomMessage(extraData as CustomMsgParams)).data;
    }
    return (await IMSDK.createCardMessage(extraData as CardElem)).data;
  };

  const getForwardContent = () => {
    if (!showConversationTypes.includes(type)) return undefined;
    if ((extraData as MergerMsgParams).title) {
      return t("messageDescription.forwardMessage", {
        additional: (extraData as MergerMsgParams).title,
      });
    }
    if ((extraData as ExMessageItem).clientMsgID) {
      return t("messageDescription.forwardMessage", {
        additional: formatMessageByType(extraData as ExMessageItem),
      });
    }
    if ((extraData as CustomMsgParams).data) {
      return t("messageDescription.meetingMessage");
    }
    return t("messageDescription.addtionalCardMessage", {
      additional: (extraData as CardElem).nickname,
    });
  };

  const resetState = () => {
    forwardProgress.current.clear();
    chooseBoxRef.current?.resetState();
    setGroupBaseInfo({
      groupName: "",
      groupAvatar: "",
    });
  };

  const customUpload = async ({ file }: { file: File }) => {
    try {
      const {
        data: { url },
      } = await IMSDK.uploadFile({
        name: file.name,
        contentType: file.type,
        uuid: uuidV4(),
        file,
      });
      setGroupBaseInfo((prev) => ({ ...prev, groupAvatar: url }));
    } catch (error) {
      feedbackToast({ error: t("toast.updateAvatarFailed") });
    }
  };

  const isCheckInGroup = type === "INVITE_TO_GROUP";
  const notConversation = !showConversationTypes.includes(type);

  return (
    <Modal
      title={null}
      footer={null}
      centered
      open={isOverlayOpen}
      closable={false}
      width={680}
      onCancel={() => { if (!confirming.current) closeOverlay(); }}
      destroyOnClose
      styles={{
        mask: {
          opacity: 0,
          transition: "none",
        },
      }}
      afterClose={resetState}
      className="no-padding-modal max-w-[80vw]"
      maskTransitionName=""
    >
      <div>
        <div className="app-no-drag flex h-16 items-center justify-between bg-[var(--gap-text)] px-7">
          <div>{titleMap[type]}</div>
          <CloseOutlined
            className="cursor-pointer text-[var(--sub-text)]"
            rev={undefined}
            onClick={() => { if (!confirming.current) closeOverlay(); }}
          />
        </div>
        {type === "CRATE_GROUP" ? (
          <div className="px-6 pt-4">
            <div className="mb-6 flex items-center">
              <div className="min-w-[60px] font-medium">
                {t("placeholder.groupName")}
              </div>
              <Input
                placeholder={t("placeholder.pleaseEnter")}
                spellCheck={false}
                value={groupBaseInfo.groupName}
                onChange={(e) =>
                  setGroupBaseInfo((state) => ({ ...state, groupName: e.target.value }))
                }
              />
            </div>
            <div className="mb-6 flex items-center">
              <div className="min-w-[60px] font-medium">
                {t("placeholder.groupAvatar")}
              </div>
              <div className="flex items-center">
                <OIMAvatar src={groupBaseInfo.groupAvatar} isgroup />
                <Upload
                  accept="image/*"
                  showUploadList={false}
                  customRequest={customUpload as any}
                >
                  <span className="ml-3 cursor-pointer text-xs text-[var(--primary)]">
                    {t("placeholder.clickToModify")}
                  </span>
                </Upload>
              </div>
            </div>
            <div className="flex">
              <div className="min-w-[60px] font-medium">
                {t("placeholder.groupMember")}
              </div>
              <ChooseBox
                className="!m-0 !h-[40vh] flex-1"
                ref={chooseBoxRef}
                notConversation={notConversation}
              />
            </div>
          </div>
        ) : (
          <ChooseBox
            className="!h-[60vh]"
            ref={chooseBoxRef}
            isCheckInGroup={isCheckInGroup}
            notConversation={
              (extraData as SelectUserExtraData)?.notConversation || notConversation
            }
            showGroupMember={onlyMemberTypes.includes(type)}
            chooseOneOnly={onlyOneTypes.includes(type)}
            checkMemberRole={type === "KICK_FORM_GROUP"}
            fowardContent={getForwardContent()}
          />
        )}
        <div className="flex justify-end px-9 py-6">
          <Button
            className="mr-6 border-0 bg-[var(--chat-bubble)] px-6"
            onClick={closeOverlay}
            disabled={loading}
          >
            {t("cancel")}
          </Button>
          <Button
            className="px-6"
            type="primary"
            loading={loading}
            onClick={confirmChoose}
          >
            {t("confirm")}
          </Button>
        </div>
      </div>
    </Modal>
  );
};

export default memo(forwardRef(ChooseModal));
