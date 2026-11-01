import { Drawer, Tabs, TabsProps } from "antd";
import { t } from "i18next";
import { forwardRef, ForwardRefRenderFunction, memo, useEffect, useState } from "react";
import { useParams } from "react-router-dom";

import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";

import FileMessagePane from "./FileMessagePane";
import MediaMessagePane from "./MediaMessagePane";
import NomalMessagePane from "./NomalMessagePane";

const SearchMessageDrawer: ForwardRefRenderFunction<OverlayVisibleHandle, unknown> = (
  _,
  ref,
) => {
  const { conversationID } = useParams();
  const [activeTab, setActiveTab] = useState("0");

  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  useEffect(() => {
    if (!isOverlayOpen) {
      setActiveTab("0");
    }
  }, [isOverlayOpen]);

  const items: TabsProps["items"] = [
    {
      key: "0",
      label: t("placeholder.chat"),
      children: (
        <NomalMessagePane
          conversationID={conversationID}
          isOverlayOpen={isOverlayOpen}
          closeOverlay={closeOverlay}
        />
      ),
    },
    {
      key: "1",
      label: t("placeholder.image"),
      children: (
        <MediaMessagePane
          isActive={activeTab === "1"}
          conversationID={conversationID}
        />
      ),
    },
    {
      key: "2",
      label: t("placeholder.video"),
      children: (
        <MediaMessagePane
          isVideo
          isActive={activeTab === "2"}
          conversationID={conversationID}
        />
      ),
    },
    {
      key: "3",
      label: t("placeholder.file"),
      children: (
        <FileMessagePane
          isActive={activeTab === "3"}
          isOverlayOpen={isOverlayOpen}
          conversationID={conversationID}
        />
      ),
    },
  ];

  const onTabChange = (key: string) => {
    setActiveTab(key);
  };

  return (
    <Drawer
      title={t("placeholder.messageHistory")}
      placement="right"
      rootClassName="chat-drawer"
      onClose={closeOverlay}
      open={isOverlayOpen}
      maskClassName="opacity-0"
      maskMotion={{
        visible: false,
      }}
      width={450}
      getContainer={"#chat-container"}
    >
      <Tabs
        activeKey={activeTab}
        className="message-drawer-tab"
        items={items}
        onChange={onTabChange}
      />
    </Drawer>
  );
};

export default memo(forwardRef(SearchMessageDrawer));
