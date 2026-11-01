import { Empty, Spin } from "antd";
import clsx from "clsx";
import { SessionType } from "@openim/wasm-client-sdk";
import { FriendUserItem, GroupItem } from "@/im/types";
import { memo, useEffect } from "react";
import { Virtuoso } from "react-virtuoso";

import OIMAvatar from "@/components/OIMAvatar";
import { useConversationToggle } from "@/hooks/useConversationToggle";

import styles from "./index.module.scss";
import { useKeyPage } from "./useKeyPage";

export type ContactItem = Partial<FriendUserItem & GroupItem>;

export const ContactRender = memo(
  ({
    id,
    item,
    isActive,
    onClick,
  }: {
    id?: string;
    item: ContactItem;
    isActive?: boolean;
    onClick?: (item: ContactItem) => void;
  }) => {
    return (
      <div
        id={id}
        onClick={() => onClick?.(item)}
        className={clsx(
          "flex cursor-pointer items-center rounded px-3 py-2 hover:bg-[var(--primary-active)]",
          {
            "bg-[var(--primary-active)]": isActive,
          },
        )}
      >
        <OIMAvatar
          src={item.faceURL}
          text={item.nickname}
          isgroup={Boolean(item.groupID)}
        />
        <div className="ml-3 max-w-[200px] truncate">
          {item.nickname || item.groupName}
        </div>
      </div>
    );
  },
);

const ContactPanel = ({
  data,
  loading,
  isActive,
  closeOverlay,
}: {
  data: ContactItem[];
  loading: boolean;
  isActive: boolean;
  closeOverlay: () => void;
}) => {
  const { toSpecifiedConversation } = useConversationToggle();

  const contactType = data[0]?.userID ? "friend" : "group";
  const { activeIdx, updateIdx } = useKeyPage({
    isActive,
    maxIndex: data.length,
    elPrefix: `#${contactType}-item-`,
    callback: (idx) => {
      const item = data[idx];
      if (item) {
        jumpToConversation(item);
      }
    },
  });

  useEffect(() => {
    if (loading) {
      updateIdx(-1);
    }
  }, [loading]);

  const jumpToConversation = (item: ContactItem) => {
    toSpecifiedConversation({
      sourceID: item.userID || item.groupID || "",
      sessionType: item.groupID ? SessionType.WorkingGroup : SessionType.Single,
    });
    closeOverlay();
  };

  return (
    <Spin wrapperClassName="h-full" spinning={loading}>
      <Virtuoso
        className={clsx("mx-3 h-full overflow-x-hidden", styles["virtuoso-wrapper"])}
        data={data}
        components={{
          EmptyPlaceholder: () =>
            loading ? null : (
              <Empty className="mt-[30%]" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ),
        }}
        computeItemKey={(index, item) =>
          item.userID || item.groupID || index.toString()
        }
        itemContent={(index, item) => (
          <ContactRender
            item={item}
            id={`${contactType}-item-${index}`}
            isActive={activeIdx === index}
            onClick={jumpToConversation}
          />
        )}
      />
    </Spin>
  );
};

export default ContactPanel;
