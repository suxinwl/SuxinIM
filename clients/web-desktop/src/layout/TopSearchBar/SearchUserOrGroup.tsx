import { CloseOutlined } from "@ant-design/icons";
import { Button, Empty, Input, List, Pagination, Spin } from "antd";
import { t } from "i18next";
import {
  FullUserItemWithCache,
  GroupItem,
  WSEvent,
} from "@/im/types";
import { forwardRef, ForwardRefRenderFunction, memo, useEffect, useRef, useState } from "react";

import { message } from "@/AntdGlobalComp";
import { DirectoryPerson, getDirectory } from "@/api/directory";
import OIMAvatar from "@/components/OIMAvatar";
import DraggableModalWrap from "@/components/DraggableModalWrap";
import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { CardInfo } from "@/pages/common/UserCardModal";
import { feedbackToast } from "@/utils/common";

import { IMSDK } from "../MainContentWrap";

interface ISearchUserOrGroupProps {
  isSearchGroup: boolean;
  openUserCardWithData: (data: CardInfo) => void;
  openGroupCardWithData: (data: GroupItem) => void;
}

const SearchUserOrGroup: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  ISearchUserOrGroupProps
> = ({ isSearchGroup, openUserCardWithData, openGroupCardWithData }, ref) => {
  const [keyword, setKeyword] = useState("");
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);
  const [users, setUsers] = useState<DirectoryPerson[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState("");
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const generation = useRef(0);
  useEffect(() => { generation.current++; setUsers([]); setSearched(""); setTotal(0); setPage(1); setLoading(false); }, [isOverlayOpen, isSearchGroup]);

  const searchEmployees = async (query: string, nextPage = 1) => {
    const attempt = ++generation.current;
    setLoading(true); setUsers([]); setSearched(query); setPage(nextPage);
    try {
      const result = await getDirectory({ keyword: query, page: nextPage, pageSize: 20 });
      if (attempt !== generation.current) return;
      setUsers(result.items ?? []); setTotal(result.total ?? 0);
    } catch (error) {
      if (attempt === generation.current) feedbackToast({ error });
    } finally { if (attempt === generation.current) setLoading(false); }
  };

  const searchData = async () => {
    if (!keyword.trim()) return;

    if (isSearchGroup) {
      try {
        const { data } = await IMSDK.getSpecifiedGroupsInfo([keyword]);
        const groupInfo = data[0];
        if (!groupInfo) {
          message.warning(t("empty.noSearchResults"));
          return;
        }
        openGroupCardWithData(groupInfo);
      } catch (error) {
        if ((error as WSEvent).errCode === 1004) {
          message.warning(t("empty.noSearchResults"));
          return;
        }
        feedbackToast({ error });
      }
    } else {
      await searchEmployees(keyword.trim());
    }
  };

  return (
    <DraggableModalWrap
      title={null}
      footer={null}
      open={isOverlayOpen}
      closable={false}
      width={isSearchGroup ? 332 : 420}
      onCancel={closeOverlay}
      styles={{
        mask: {
          opacity: 0,
          transition: "none",
        },
      }}
      afterClose={() => {
        setKeyword("");
      }}
      ignoreClasses=".ignore-drag, .cursor-pointer"
      className="no-padding-modal"
      maskTransitionName=""
    >
      <div className="flex h-12 items-center justify-between bg-[var(--gap-text)] px-5.5">
        <div>
          {isSearchGroup ? t("placeholder.addGroup") : t("placeholder.addFriends")}
        </div>
        <CloseOutlined
          className="cursor-pointer text-[var(--sub-text)]"
          rev={undefined}
          onClick={closeOverlay}
        />
      </div>
      <div className="ignore-drag">
        <div className="border-b border-[var(--gap-text)] px-5.5 py-6">
          <Input.Search
            className="no-addon-search"
            placeholder={isSearchGroup ? t("placeholder.pleaseEnter") : "输入员工手机号、姓名或工号"}
            value={keyword}
            addonAfter={null}
            spellCheck={false}
            onChange={(e) => { setKeyword(e.target.value); generation.current++; setUsers([]); setSearched(""); setLoading(false); }}
            onSearch={searchData}
          />
        </div>
        {!isSearchGroup && searched && <Spin spinning={loading}>
          <List className="max-h-80 overflow-y-auto px-5" dataSource={users}
            locale={{ emptyText: loading ? "查询中…" : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有找到匹配手机号、姓名或工号的同事" /> }}
            renderItem={person => <List.Item key={person.userID}>
              <button type="button" className="flex w-full items-center gap-3 text-left" onClick={() => openUserCardWithData(person)}>
                <OIMAvatar src={person.faceURL} text={person.nickname} />
                <span><span className="block font-medium">{person.nickname}</span><span className="text-xs text-gray-500">工号：{person.employeeNo || "—"} · {person.department || "未分部门"}</span></span>
              </button>
            </List.Item>} />
          <Pagination className="px-5 py-3 text-right" size="small" current={page} total={total} pageSize={20} hideOnSinglePage showSizeChanger={false} onChange={next => searchEmployees(searched, next)} />
        </Spin>}
        <div className="flex justify-end px-5.5 py-2.5">
          <Button
            className="px-6"
            type="primary"
            disabled={!keyword}
            onClick={searchData}
          >
            {t("confirm")}
          </Button>
          <Button
            className="ml-3 border-0 bg-[var(--chat-bubble)] px-6"
            onClick={closeOverlay}
          >
            {t("cancel")}
          </Button>
        </div>
      </div>
    </DraggableModalWrap>
  );
};

export default memo(forwardRef(SearchUserOrGroup));
