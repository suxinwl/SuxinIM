import { ArrowLeftOutlined, CheckOutlined, RightOutlined } from "@ant-design/icons";
import { useRequest } from "ahooks";
import { Button, Divider, Drawer, Modal, Select, Space, Spin } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { MessageReceiveOptType } from "@openim/wasm-client-sdk";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useEffect,
  useRef,
  useState,
} from "react";

import { modal } from "@/AntdGlobalComp";
import OIMAvatar from "@/components/OIMAvatar";
import SettingRow from "@/components/SettingRow";
import { useConversationSettings } from "@/hooks/useConversationSettings";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useContactStore } from "@/store/contact";
import { feedbackToast } from "@/utils/common";
import emitter from "@/utils/events";

import MsgDestructSetting from "../MsgDestructSetting";

// export interface SingleSettingProps {}

const SingleSetting: ForwardRefRenderFunction<OverlayVisibleHandle, unknown> = (
  _,
  ref,
) => {
  const durationModalRef = useRef<OverlayVisibleHandle>(null);
  const destructModalRef = useRef<OverlayVisibleHandle>(null);

  const {
    currentConversation,
    updateBurnDuration,
    updateDestructDuration,
    updateConversationPin,
    updateConversationMessageRemind,
    updateConversationPrivateState,
    updateConversationMsgDestructState,
    clearConversationMessages,
  } = useConversationSettings();

  const isBlack = useContactStore((state) => state.blackList).some(
    (black) => currentConversation?.userID === black.userID,
  );
  const isFriend = useContactStore((state) => state.friendList).some(
    (friend) => currentConversation?.userID === friend.userID,
  );

  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  const updateBlack = async () => {
    if (!currentConversation) return;
    const execFunc = async () => {
      const funcName = isBlack ? "removeBlack" : "addBlack";
      try {
        await IMSDK[funcName](currentConversation?.userID);
      } catch (error) {
        feedbackToast({ error, msg: t("toast.updateBlackStateFailed") });
      }
    };
    if (!isBlack) {
      modal.confirm({
        title: t("placeholder.moveBlacklist"),
        content: (
          <div className="flex items-baseline">
            <div>{t("toast.confirmMoveBlacklist")}</div>
            <span className="text-xs text-[var(--sub-text)]">
              {t("placeholder.willFilterThisUserMessage")}
            </span>
          </div>
        ),
        onOk: execFunc,
      });
    } else {
      await execFunc();
    }
  };

  const tryUnfriend = () => {
    if (!currentConversation) return;
    modal.confirm({
      title: t("placeholder.unfriend"),
      content: t("toast.confirmUnfriend"),
      onOk: async () => {
        try {
          await IMSDK.deleteFriend(currentConversation.userID);
        } catch (error) {
          feedbackToast({ error, msg: t("toast.unfriendFailed") });
        }
      },
    });
  };

  const openUserCard = () => {
    emitter.emit("OPEN_USER_CARD", { userID: currentConversation?.userID });
  };

  return (
    <Drawer
      title={t("placeholder.setting")}
      placement="right"
      rootClassName="chat-drawer"
      destroyOnClose
      onClose={closeOverlay}
      open={isOverlayOpen}
      maskClassName="opacity-0"
      maskMotion={{
        visible: false,
      }}
      width={450}
      getContainer={"#chat-container"}
    >
      <div
        className="flex cursor-pointer items-center justify-between p-4"
        onClick={openUserCard}
      >
        <div className="flex items-center">
          <OIMAvatar
            src={currentConversation?.faceURL}
            text={currentConversation?.showName}
          />
          <div className="ml-3">{currentConversation?.showName}</div>
        </div>
        <RightOutlined rev={undefined} />
      </div>
      <Divider className="m-0 border-4 border-[#F4F5F7]" />
      <SettingRow
        className="pb-2"
        title={t("placeholder.sticky")}
        value={currentConversation?.isPinned}
        tryChange={updateConversationPin}
      />
      <SettingRow
        className="pb-2"
        title={t("placeholder.notNotify")}
        value={currentConversation?.recvMsgOpt === MessageReceiveOptType.NotNotify}
        tryChange={(checked) =>
          updateConversationMessageRemind(checked, MessageReceiveOptType.NotNotify)
        }
      />
      {/* <SettingRow
        className="pb-2"
        title={t("placeholder.shieldConversation")}
        value={currentConversation?.recvMsgOpt === MessageReceiveOptType.NotReceive}
        tryChange={(checked) =>
          updateConversationMessageRemind(checked, MessageReceiveOptType.NotReceive)
        }
      /> */}
      <SettingRow
        title={t("placeholder.moveBlacklist")}
        value={isBlack}
        tryChange={updateBlack}
      />
      <Divider className="m-0 border-4 border-[#F4F5F7]" />
      <SettingRow
        className="pb-2"
        title={t("placeholder.privateChat")}
        value={currentConversation?.isPrivateChat}
        tryChange={updateConversationPrivateState}
      />
      {currentConversation?.isPrivateChat && (
        <SettingRow
          className="cursor-pointer"
          title={t("placeholder.privateChatTime")}
          value={false}
          rowClick={() => durationModalRef.current?.openOverlay()}
        >
          <div className="flex items-center">
            <span className="mr-1 text-xs text-[var(--sub-text)]">
              {burnDurationMap[currentConversation?.burnDuration ?? 30]}
            </span>
            <RightOutlined rev={undefined} />
          </div>
        </SettingRow>
      )}
      <Divider className="m-0 border-4 border-[#F4F5F7]" />
      <MsgDestructSetting
        currentConversation={currentConversation}
        updateDestructDuration={updateDestructDuration}
        updateConversationMsgDestructState={updateConversationMsgDestructState}
      />
      <Divider className="m-0 border-4 border-[#F4F5F7]" />
      <SettingRow
        className="cursor-pointer"
        title={t("toast.clearChatHistory")}
        rowClick={clearConversationMessages}
      >
        <RightOutlined rev={undefined} />
      </SettingRow>

      <div className="flex-1" />
      {isFriend && (
        <div className="flex w-full justify-center pb-3 pt-24">
          <Button type="primary" danger onClick={tryUnfriend}>
            {t("placeholder.unfriend")}
          </Button>
        </div>
      )}
      <ForwardBurnDurationModal
        ref={durationModalRef}
        burnDuration={currentConversation?.burnDuration ?? 30}
        updateBurnDuration={updateBurnDuration}
      />
    </Drawer>
  );
};

export default memo(forwardRef(SingleSetting));

const durationOptions = [
  {
    title: "30s",
    value: 30,
  },
  {
    title: t("date.minute", { num: 5 }),
    value: 60 * 5,
  },
  {
    title: t("date.hour", { num: 1 }),
    value: 60 * 60,
  },
];

const burnDurationMap = durationOptions.reduce((acc, option) => {
  acc[option.value] = option.title;
  return acc;
}, {} as Record<number, string>);

const BurnDurationModal: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  {
    burnDuration: number;
    updateBurnDuration: (seconds: number) => Promise<void>;
  }
> = ({ burnDuration, updateBurnDuration }, ref) => {
  const [selectedOption, setSelectedOption] = useState<number>(burnDuration);
  const { closeOverlay, isOverlayOpen } = useOverlayVisible(ref);

  const { loading, runAsync } = useRequest(updateBurnDuration, { manual: true });

  const saveMute = async () => {
    if (selectedOption === undefined) {
      return;
    }
    await runAsync(selectedOption);
    closeOverlay();
  };

  return (
    <Modal
      title={null}
      footer={null}
      closable={false}
      open={isOverlayOpen}
      destroyOnClose
      centered
      onCancel={closeOverlay}
      width={320}
      className="no-padding-modal"
    >
      <Spin spinning={loading}>
        <div className="py-6">
          <div className="flex items-center justify-between px-5">
            <ArrowLeftOutlined
              className="cursor-pointer !text-[#8e9aaf]"
              onClick={closeOverlay}
            />
            <div>{t("placeholder.privateChatTime")}</div>
            <span className="cursor-pointer text-[var(--primary)]" onClick={saveMute}>
              {t("placeholder.save")}
            </span>
          </div>
          <div className="mt-5">
            {durationOptions.map((option) => (
              <div
                key={option.value}
                className={clsx(
                  "flex cursor-pointer items-center justify-between px-6 py-4 hover:bg-[var(--primary-active)]",
                  { "bg-[var(--primary-active)]": selectedOption === option.value },
                )}
                onClick={() => setSelectedOption(option.value)}
              >
                <span>{option.title}</span>
                {selectedOption === option.value && (
                  <CheckOutlined className="!text-[var(--primary)]" />
                )}
              </div>
            ))}
          </div>
        </div>
      </Spin>
    </Modal>
  );
};

const ForwardBurnDurationModal = memo(forwardRef(BurnDurationModal));

export const getDestructStr = (seconds: number) => {
  const days = Math.floor(seconds / 86400);
  const weeks = Math.floor(days / 7);
  const month = Math.floor(weeks / 4);
  if (days <= 6) {
    return t("date.day", { num: days });
  } else if (weeks <= 6 && days % 7 === 0) {
    return t("date.weeks", { num: weeks });
  }
  return t("date.month", { num: month });
};

const timeUnitOptions = [
  {
    label: t("time.day"),
    value: 86400,
  },
  {
    label: t("time.week"),
    value: 86400 * 7,
  },
  {
    label: t("time.month"),
    value: 86400 * 30,
  },
];

const DestructDurationModal: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  {
    destructDuration: number;
    updateDestructDuration: (seconds: number) => Promise<void>;
  }
> = ({ destructDuration, updateDestructDuration }, ref) => {
  const [optionState, setOptionState] = useState({
    number: 1,
    unit: 86400,
  });
  const { closeOverlay, isOverlayOpen } = useOverlayVisible(ref);

  const { loading, runAsync, cancel } = useRequest(updateDestructDuration, {
    manual: true,
  });

  useEffect(() => {
    const days = Math.floor(destructDuration / 86400);
    const weeks = Math.floor(days / 7);
    const month = Math.floor(weeks / 4);
    if (days <= 6) {
      setOptionState({
        number: days,
        unit: 86400,
      });
    } else if (weeks <= 6 && days % 7 === 0) {
      setOptionState({
        number: weeks,
        unit: 86400 * 7,
      });
    } else {
      setOptionState({
        number: month,
        unit: 86400 * 30,
      });
    }
  }, [destructDuration]);

  const handleNumberChange = (value: number) => {
    setOptionState((prev) => ({ ...prev, number: value }));
  };

  const handleUnitChange = (value: number) => {
    setOptionState((prev) => ({ ...prev, unit: value }));
  };

  const saveChange = async () => {
    try {
      await runAsync(optionState.number * optionState.unit);
    } catch (error) {
      feedbackToast({ error });
    }
    closeOverlay();
  };

  const onCancel = () => {
    cancel();
    closeOverlay();
  };

  return (
    <Modal
      title={null}
      footer={null}
      closable={false}
      open={isOverlayOpen}
      destroyOnClose
      centered
      onCancel={onCancel}
      width={320}
      className="no-padding-modal"
    >
      <Spin spinning={loading}>
        <div className="px-5.5 py-6">
          <div className="mb-1">{t("placeholder.messageDestruct")}</div>
          <div className="text-xs text-[var(--sub-text)]">
            {t("placeholder.messageDestructToast")}
          </div>
          <div className="mt-5">
            <Space wrap>
              <Select
                value={optionState.number}
                style={{ width: 60 }}
                onChange={handleNumberChange}
                options={Array(7)
                  .fill(1)
                  .map((_, idx) => ({ label: idx + 1, value: idx + 1 }))}
              />
              <Select
                style={{ width: 60 }}
                value={optionState.unit}
                onChange={handleUnitChange}
                options={timeUnitOptions}
              />
            </Space>
          </div>
          <div className="mt-10 flex items-center justify-end">
            <div>
              <span className="mr-3 cursor-pointer" onClick={closeOverlay}>
                {t("cancel")}
              </span>
              <span
                className="cursor-pointer text-[var(--primary)]"
                onClick={saveChange}
              >
                {t("confirm")}
              </span>
            </div>
          </div>
        </div>
      </Spin>
    </Modal>
  );
};

export const ForwardDestructDurationModal = memo(forwardRef(DestructDurationModal));
