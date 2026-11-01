import { useClickAway, useKeyPress, useLatest, useRequest } from "ahooks";
import { Empty, Image, Spin } from "antd";
import clsx from "clsx";
import i18n, { t } from "i18next";
import { GroupMemberItem } from "@/im/types";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";

import OIMAvatar from "@/components/OIMAvatar";
import { useCurrentMemberRole } from "@/hooks/useCurrentMemberRole";
import { IMSDK } from "@/layout/MainContentWrap";
import { useConversationStore } from "@/store";

export interface AtSearchPanelHandle {
  searchMember: (keyword: string) => Promise<void>;
}

interface IAtSearchPanelProps {
  open: boolean;
  closePanel: () => void;
  enterToSend: () => void;
  atHandler: (atUser: GroupMemberItem, needDelete?: boolean) => void;
}

const AT_ALL_KEY = "AtAllTag";

const atAllItem = {
  nickname: t("placeholder.mentionAll"),
  userID: AT_ALL_KEY,
} as GroupMemberItem;

i18n.on("languageChanged", () => (atAllItem.nickname = t("placeholder.mentionAll")));

const AtSearchPanel: ForwardRefRenderFunction<
  AtSearchPanelHandle,
  IAtSearchPanelProps
> = ({ open, enterToSend, closePanel, atHandler }, ref) => {
  const wrapRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [activeUserID, setActiveUserID] = useState("");
  const [searchData, setSearchData] = useState<GroupMemberItem[]>([]);
  const latestOpen = useLatest(open);
  const { isAdmin, isOwner } = useCurrentMemberRole();

  const canAtAll = isAdmin || isOwner;

  const { runAsync: requestSearchMembers, loading: searchLoading } = useRequest(
    (p:any) => IMSDK.searchGroupMembers(p),
    {
      manual: true,
    },
  );
  const { runAsync: requestMemberList, loading } = useRequest(
    (p:any) => IMSDK.getGroupMemberList(p),
    {
      manual: true,
    },
  );

  useEffect(() => {
    if (listRef.current) {
      const activeItem = listRef.current.querySelector(
        `.at-panel-item-${activeUserID}`,
      ) as HTMLDivElement;
      if (activeItem) {
        const scrollTop = activeItem.offsetTop - listRef.current.offsetTop - 2 * 26;
        listRef.current.scrollTop = scrollTop;
      }
    }
  }, [activeUserID]);

  const searchMember = async (keyword: string) => {
    try {
      const options = {
        groupID: useConversationStore.getState().currentConversation?.groupID ?? "",
        offset: 0,
        count: 50,
        filter: 0,
        keywordList: [keyword],
        isSearchMemberNickname: true,
        isSearchUserID: false,
      };
      let memberList = [] as GroupMemberItem[];
      if (keyword) {
        memberList = (await requestSearchMembers(options)).data;
      } else {
        memberList = (await requestMemberList(options)).data;
      }
      setSearchData([...memberList]);
      setActiveUserID(canAtAll && !keyword ? AT_ALL_KEY : memberList[0]?.userID);
    } catch (error) {
      setSearchData([]);
    }
  };

  useKeyPress("uparrow", () => {
    const idx = searchData.findIndex((item) => item.userID === activeUserID);
    if (canAtAll && idx === 0) {
      setActiveUserID(AT_ALL_KEY);
      return;
    }
    if (idx < 1) return;
    setActiveUserID(searchData[idx - 1].userID);
  });

  useKeyPress("downarrow", () => {
    if (activeUserID === AT_ALL_KEY) {
      setActiveUserID(searchData[0].userID);
      return;
    }
    const idx = searchData.findIndex((item) => item.userID === activeUserID);
    if (idx < 0 || idx > searchData.length - 2) return;
    setActiveUserID(searchData[idx + 1].userID);
  });

  useKeyPress("enter", () => {
    if (listLoading || !latestOpen.current) return;

    if (!activeUserID) {
      closePanel();
      enterToSend();
      return;
    }
    if (activeUserID === AT_ALL_KEY) {
      atHandler(atAllItem, true);
      return;
    }
    const idx = searchData.findIndex((item) => item.userID === activeUserID);
    atHandler(searchData[idx], true);
  });

  useKeyPress("esc", () => {
    closePanel();
  });

  useClickAway(() => {
    closePanel();
  }, wrapRef.current);

  useImperativeHandle(
    ref,
    () => ({
      searchMember,
    }),
    [],
  );

  const listLoading = loading || searchLoading;

  return (
    <Spin spinning={listLoading}>
      <div ref={wrapRef} className="mx-1 flex h-[220px] min-w-[268px] flex-col py-1">
        {canAtAll && (
          <div>
            <AtPanelItem
              item={atAllItem}
              active={activeUserID === AT_ALL_KEY}
              atHandler={atHandler}
            />
          </div>
        )}
        <div className="mx-2">{t("placeholder.groupMember")}</div>
        <div className="flex-1 overflow-auto" ref={listRef}>
          {!listLoading && !searchData.length ? (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              className="m-0 flex h-full flex-col items-center justify-center"
              description={t("empty.noSearchResults")}
            />
          ) : (
            searchData.map((item) => (
              <AtPanelItem
                key={item.userID}
                item={item}
                active={activeUserID === item.userID}
                atHandler={atHandler}
              />
            ))
          )}
        </div>
      </div>
    </Spin>
  );
};

export const ForwardAtSearchPanel = forwardRef(AtSearchPanel);

const AtPanelItem = ({
  item,
  active,
  atHandler,
}: {
  item: GroupMemberItem;
  active?: boolean;
  atHandler: (atUser: GroupMemberItem, needDelete?: boolean) => void;
}) => {
  return (
    <div
      className={clsx(
        "flex items-center rounded-md px-2 py-2 hover:bg-[var(--primary-active)]",
        {
          "bg-[var(--primary-active)]": active,
        },
        `at-panel-item-${item.userID}`,
      )}
      onClick={() => atHandler(item, true)}
    >
      <OIMAvatar
        size={26}
        text={item.userID === AT_ALL_KEY ? "@" : item.nickname}
        src={item.faceURL}
      />
      <div className="ml-2">{item.nickname}</div>
    </div>
  );
};

export const SnapPreviewGroup = memo(() => {
  const [snapState, setSnapState] = useState({
    visible: false,
    idx: 0,
    list: [] as string[],
  });

  useEffect(() => {
    const showAlbum = (result: string) => {
      const screenshotEls = [
        ...document.getElementsByClassName("image-el"),
      ] as HTMLImageElement[];
      const list = screenshotEls.map((el) => el.src);
      setSnapState({
        visible: true,
        idx: list.findIndex((item) => item === result),
        list,
      });
    };

    window.screenshotPreview = showAlbum;
  }, []);

  return (
    <div style={{ display: "none" }}>
      <Image.PreviewGroup
        items={snapState.list}
        preview={{
          current: snapState.idx,
          visible: snapState.visible,
          onVisibleChange: (vis) =>
            setSnapState((state) => ({ ...state, visible: vis })),
        }}
      ></Image.PreviewGroup>
    </div>
  );
});
