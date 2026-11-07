import { SearchOutlined } from "@ant-design/icons";
import { useDebounceFn, useKeyPress, useLatest } from "ahooks";
import { Input, InputRef, Modal, Tabs } from "antd";
import clsx from "clsx";
import { t } from "i18next";
import { MessageType, SessionType } from "@openim/wasm-client-sdk";
import {
  FriendUserItem,
  GroupItem,
  MessageItem,
  SearchMessageResult,
  SearchMessageResultItem,
} from "@/im/types";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";

import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { IMSDK } from "@/layout/MainContentWrap";
import { useMessageStore } from "@/store";
import { getConversationContent } from "@/utils/imCommon";
import { DirectoryPerson, getDirectory } from "@/api/directory";
import emitter from "@/utils/events";
import DirectoryPanel from "./DirectoryPanel";

import ChatLogsPanel from "./ChatLogsPanel";
import ContactPanel from "./ContactPanel";
import DashboardPanel from "./DashboardPanel";
import FilePanel from "./FilePanel";
import styles from "./index.module.scss";

export interface SearchData<T> {
  data: T[];
  loading: boolean;
}

export type ChatLogsItem = SearchMessageResultItem & {
  sendTime: number;
  description: string;
};

const TabKeys = ["DashBoard", "Colleagues", "Friends", "Groups", "ChatLogs", "HistoryFiles"] as const;

export type TabKey = (typeof TabKeys)[number];

const GlobalSearchModal: ForwardRefRenderFunction<OverlayVisibleHandle, unknown> = (
  _,
  ref,
) => {
  const [activeKey, setActiveKey] = useState<TabKey>("DashBoard");
  const [employees, setEmployees] = useState<DirectoryPerson[]>([]);
  const [employeeLoading, setEmployeeLoading] = useState(false);
  const [employeeError, setEmployeeError] = useState("");
  const [employeeTotal, setEmployeeTotal] = useState(0);
  const [employeePage, setEmployeePage] = useState(1);
  const employeeGeneration = useRef(0);
  const [friends, setFriends] = useState<SearchData<FriendUserItem>>({
    data: [],
    loading: false,
  });
  const [groups, setGroups] = useState<SearchData<GroupItem>>({
    data: [],
    loading: false,
  });
  const [chatLogs, setChatLogs] = useState<SearchData<ChatLogsItem>>({
    data: [],
    loading: false,
  });
  const [historyFiles, setHistoryFiles] = useState<SearchData<MessageItem>>({
    data: [],
    loading: false,
  });
  const latestHistoryFiles = useLatest(historyFiles);
  const searchBarRef = useRef<SearchBarHandle>(null);
  const chatLogRef = useRef<{ updateIdx: (idx: number) => void }>(null);
  const latestKeyword = useRef("");

  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);

  useEffect(() => {
    if (isOverlayOpen && location.hash.startsWith("#/contact")) {
      setActiveKey("Colleagues");
    }
  }, [isOverlayOpen]);

  useEffect(() => {
    const downloadSuccessHandler = (url: string, filePath: string) => {
      const { clientMsgID } = useMessageStore.getState().downloadMap[url];

      const index = latestHistoryFiles.current.data.findIndex(
        (message) => message.clientMsgID === clientMsgID,
      );
      if (index > -1) {
        setHistoryFiles((state) => {
          const tmpMessage = [...state.data];
          tmpMessage[index].localEx = filePath;
          return {
            ...state,
            data: tmpMessage,
          };
        });
      }
    };

    const unsubscribeSuccess = window.electronAPI?.subscribe(
      "downloadSuccess",
      downloadSuccessHandler,
    );

    return () => {
      unsubscribeSuccess?.();
    };
  }, []);

  useKeyPress("leftarrow", () => {
    const currentIndex = TabKeys.indexOf(activeKey);
    if (currentIndex > 0 && isOverlayOpen) {
      setActiveKey(TabKeys[currentIndex - 1]);
    }
  });

  useKeyPress("rightarrow", () => {
    const currentIndex = TabKeys.indexOf(activeKey);
    if (currentIndex < TabKeys.length - 1 && isOverlayOpen) {
      setActiveKey(TabKeys[currentIndex + 1]);
    }
  });

  const toggleTab = useCallback((tab: TabKey) => {
    setActiveKey(tab);
  }, []);

  const toggleChatLogActive = useCallback(
    (idx: number) => chatLogRef.current?.updateIdx(idx),
    [],
  );

  const searchEmployees = async (keyword: string, page = 1) => {
    const attempt = ++employeeGeneration.current;
    setEmployees([]); setEmployeeLoading(true); setEmployeeError(""); setEmployeePage(page);
    try {
      const result = await getDirectory({ keyword, page, pageSize: 20 });
      if (attempt !== employeeGeneration.current || latestKeyword.current !== keyword) return;
      setEmployees(result.items ?? []); setEmployeeTotal(result.total ?? 0);
    } catch (_) {
      if (attempt === employeeGeneration.current) setEmployeeError("员工搜索暂不可用，请重试");
    } finally { if (attempt === employeeGeneration.current) setEmployeeLoading(false); }
  };
  const employeePanel = (summary = false) => <DirectoryPanel people={employees} loading={employeeLoading} error={employeeError} total={employeeTotal} page={employeePage} searched={Boolean(latestKeyword.current)} summary={summary}
    onPage={page => searchEmployees(latestKeyword.current, page)} onMore={() => setActiveKey("Colleagues")}
    onOpen={person => { closeOverlay(); emitter.emit("OPEN_USER_CARD", { userID: person.userID }); }} />;
  const items = [
    {
      key: "DashBoard",
      label: t("placeholder.overview"),
      children: (
        <div className="flex h-full flex-col">{employeePanel(true)}<div className="min-h-0 flex-1"><DashboardPanel
          isActive={activeKey === "DashBoard"}
          friends={friends}
          groups={groups}
          chatLogs={chatLogs}
          historyFiles={historyFiles}
          closeOverlay={closeOverlay}
          toggleTab={toggleTab}
          toggleChatLogActive={toggleChatLogActive}
        /></div></div>
      ),
    },
    { key: "Colleagues", label: "企业同事", children: employeePanel() },
    {
      key: "Friends",
      label: t("placeholder.contacts"),
      children: (
        <ContactPanel
          isActive={activeKey === "Friends"}
          {...friends}
          closeOverlay={closeOverlay}
        />
      ),
    },
    {
      key: "Groups",
      label: t("placeholder.myGroup"),
      children: (
        <ContactPanel
          isActive={activeKey === "Groups"}
          {...groups}
          closeOverlay={closeOverlay}
        />
      ),
    },
    {
      key: "ChatLogs",
      label: t("placeholder.messageHistory"),
      children: (
        <ChatLogsPanel
          ref={chatLogRef}
          isActive={activeKey === "ChatLogs"}
          {...chatLogs}
          closeOverlay={closeOverlay}
        />
      ),
    },
    {
      key: "HistoryFiles",
      label: t("placeholder.document"),
      children: <FilePanel isActive={activeKey === "HistoryFiles"} {...historyFiles} />,
    },
  ];

  const searchFriend = async (keyword: string) => {
    setFriends({
      data: [],
      loading: true,
    });
    let friendlist: FriendUserItem[] = [];
    try {
      const { data } = await IMSDK.searchFriends({
        keywordList: [keyword],
        isSearchNickname: true,
        isSearchRemark: true,
        isSearchUserID: true,
      });
      friendlist = data;
    } catch (error) {
      console.error(error);
    }
    if (latestKeyword.current !== keyword) return;
    setFriends({
      data: friendlist,
      loading: false,
    });
  };

  const searchGroup = async (keyword: string) => {
    setGroups({
      data: [],
      loading: true,
    });
    let groupList: GroupItem[] = [];
    try {
      const { data } = await IMSDK.searchGroups({
        keywordList: [keyword],
        isSearchGroupID: true,
        isSearchGroupName: true,
      });
      groupList = data;
    } catch (error) {
      console.error(error);
    }

    if (latestKeyword.current !== keyword) return;
    setGroups({
      data: groupList,
      loading: false,
    });
  };

  const searchChatLogs = async (keyword: string) => {
    setChatLogs({
      data: [],
      loading: true,
    });
    let chatLogList: ChatLogsItem[] = [];
    try {
      const { data } = await IMSDK.searchLocalMessages({
        conversationID: "",
        keywordList: [keyword],
        messageTypeList: [
          MessageType.TextMessage,
          MessageType.AtTextMessage,
          MessageType.FileMessage,
          MessageType.QuoteMessage,
          MessageType.CardMessage,
          MessageType.LocationMessage,
          MessageType.MergeMessage,
        ],
      });

      data.searchResultItems?.map((result) => {
        (result as ChatLogsItem).sendTime = result.messageList[0].sendTime;
        if (result.messageCount > 1) {
          (result as ChatLogsItem).description = t("placeholder.relevantMessage", {
            count: result.messageCount,
          });
        } else {
          (result as ChatLogsItem).description = getConversationContent(
            result.messageList[0],
          );
        }
      });
      chatLogList = (data.searchResultItems as ChatLogsItem[]) ?? [];
      console.log(chatLogList);
    } catch (error) {
      console.error(error);
    }
    if (latestKeyword.current !== keyword) return;
    setChatLogs({
      data: chatLogList,
      loading: false,
    });
  };

  const searchHistoryFiles = async (keyword: string) => {
    setHistoryFiles({
      data: [],
      loading: true,
    });
    let historyFileList: MessageItem[] = [];
    try {
      const { data } = await IMSDK.searchLocalMessages({
        conversationID: "",
        keywordList: [keyword],
        messageTypeList: [MessageType.FileMessage],
      });
      console.log(data);
      data.searchResultItems?.map((result) => {
        if (result.conversationType === SessionType.WorkingGroup) {
          result.messageList.map(
            (message) => (message.senderNickname = result.showName),
          );
        }
      });
      historyFileList =
        data.searchResultItems?.map((result) => result.messageList).flat() ?? [];
    } catch (error) {
      console.error(error);
    }
    if (latestKeyword.current !== keyword) return;
    setHistoryFiles({
      data: historyFileList,
      loading: false,
    });
  };

  const resetState = () => {
    employeeGeneration.current++;
    setEmployees([]); setEmployeeLoading(false); setEmployeeError(""); setEmployeeTotal(0); setEmployeePage(1);
    latestKeyword.current = "";
    setFriends({
      data: [],
      loading: false,
    });
    setGroups({
      data: [],
      loading: false,
    });
    setChatLogs({
      data: [],
      loading: false,
    });
    setHistoryFiles({
      data: [],
      loading: false,
    });
    setActiveKey("DashBoard");
    searchBarRef.current?.clearKeyword();
  };

  const triggerSearch = (keyword: string) => {
    keyword = keyword.trim();
    latestKeyword.current = keyword;
    if (!keyword) {
      employeeGeneration.current++;
      setEmployees([]); setEmployeeLoading(false); setEmployeeError(""); setEmployeeTotal(0);
      setFriends({ data: [], loading: false });
      setGroups({ data: [], loading: false });
      setChatLogs({ data: [], loading: false });
      setHistoryFiles({ data: [], loading: false });
      return;
    }
    searchFriend(keyword);
    searchEmployees(keyword);
    searchGroup(keyword);
    searchChatLogs(keyword);
    searchHistoryFiles(keyword);
  };

  return (
    <Modal
      title={null}
      footer={null}
      centered
      open={isOverlayOpen}
      closable={false}
      width={"70%"}
      destroyOnClose
      afterClose={resetState}
      onCancel={closeOverlay}
      afterOpenChange={(open) => {
        if (open) {
          searchBarRef.current?.focus();
        }
      }}
      styles={{
        mask: {
          opacity: 0,
          transition: "none",
        },
      }}
      className={clsx("no-padding-modal", styles["global-search-modal"])}
      maskTransitionName=""
    >
      <ForWardSearchBar ref={searchBarRef} triggerSearch={triggerSearch} />
      <Tabs
        className={styles["search-tab"]}
        defaultActiveKey="DashBoard"
        activeKey={activeKey}
        items={items}
        onChange={toggleTab as (key: string) => void}
      />
    </Modal>
  );
};

export default memo(forwardRef(GlobalSearchModal));

type SearchBarHandle = { clearKeyword: () => void; focus: () => void };

const SearchBar: ForwardRefRenderFunction<
  SearchBarHandle,
  { triggerSearch: (value: string) => void }
> = ({ triggerSearch }, ref) => {
  const inputRef = useRef<InputRef>(null);
  const [keyword, setKeyword] = useState("");

  const { run: debounceSearch, cancel } = useDebounceFn(triggerSearch, { wait: 500 });

  const onChange = (value: string) => {
    setKeyword(value);
    debounceSearch(value);
  };

  const onPressEnter = (value: string) => {
    cancel();
    triggerSearch(value);
  };

  useImperativeHandle(
    ref,
    () => ({
      clearKeyword: () => setKeyword(""),
      focus: () => inputRef.current?.focus(),
    }),
    [],
  );

  return (
    <div className="px-6">
      <Input
        placeholder="搜索手机号、姓名、工号或聊天内容"
        allowClear
        prefix={<SearchOutlined />}
        value={keyword}
        ref={inputRef}
        onChange={(e) => onChange(e.target.value)}
        onPressEnter={(e) => onPressEnter(e.currentTarget.value)}
      />
    </div>
  );
};

const ForWardSearchBar = memo(forwardRef(SearchBar));
