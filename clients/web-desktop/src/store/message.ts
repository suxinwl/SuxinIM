import { toLocalFileURL } from "@/im/local-files";
import { t } from "i18next";
import { MessageType, SessionType } from "@openim/wasm-client-sdk";
import { MessageItem, SearchMessageResult } from "@/im/types";
import { create } from "zustand";

import { IMSDK } from "@/layout/MainContentWrap";
import { feedbackToast } from "@/utils/common";

import { useConversationStore } from "./conversation";
import { DownloadData, MessageStore, UpdateMessaggeBaseInfoParams } from "./type";
import { useUserStore } from "./user";

const GET_HISTORY_MESSAGE_COUNT = 20;

export interface ExType {
  checked?: boolean;
  isAppend?: boolean;
  gapTime?: boolean;
  jump?: boolean;
  errCode?: number;
}

export type ExMessageItem = MessageItem & ExType;

interface IAdvancedMessageResponse {
  lastMinSeq: number;
  isEnd: boolean;
  messageList: ExMessageItem[];
}

export const useMessageStore = create<MessageStore>()((set, get) => ({
  historyMessageList: [],
  previewImgList: [],
  jumpLoading: false,
  jumpClientMsgID: undefined,
  lastMinSeq: 0,
  hasMore: true,
  laterHasMore: false,
  isCheckMode: false,
  downloadMap: {},
  getHistoryMessageListByReq: async (loadMore = false) => {
    const conversationID =
      useConversationStore.getState().currentConversation?.conversationID;
    if (!conversationID) return;
    try {
      const prevList = [...get().historyMessageList];
      const { data } = await IMSDK.getAdvancedHistoryMessageList({
        userID: "",
        groupID: "",
        count: GET_HISTORY_MESSAGE_COUNT,
        lastMinSeq: loadMore ? get().lastMinSeq : 0,
        startClientMsgID: loadMore ? prevList[prevList.length - 1]?.clientMsgID : "",
        conversationID,
      });

      if (
        conversationID !==
        useConversationStore.getState().currentConversation?.conversationID
      )
        return;

      (data.messageList as ExMessageItem[]).map((message, idx) => {
        if (!idx) return;
        const prevTime = data.messageList[idx - 1]?.sendTime;
        if (message.sessionType === SessionType.Notification) {
          (data.messageList[idx - 1] as ExMessageItem).gapTime =
            message.sendTime - prevTime > 300000;
        } else {
          message.gapTime = message.sendTime - prevTime > 300000;
        }
      });
      const nextList = [...(loadMore ? prevList : []), ...data.messageList.reverse()];

      set(() => ({
        lastMinSeq: data.messageList.reduce((min,m) => m.seq > 0 && (!min || m.seq < min) ? m.seq : min, 0),
        hasMore: data.messageList.length !== 0,
        historyMessageList: nextList,
      }));
      console.info(get().historyMessageList);
    } catch (error) {
      feedbackToast({ error, msg: t("toast.getHistoryMessageFailed") });
      set(() => ({
        lastMinSeq: 0,
        hasMore: false,
        historyMessageList: [],
      }));
    }
  },
  getHistoryMessageListReverseByReq: async () => {
    const conversationID =
      useConversationStore.getState().currentConversation?.conversationID;
    if (!conversationID) return;
    try {
      const prevList = [...get().historyMessageList];
      const lastMinSeq = prevList.find((message) => Boolean(message.seq))?.seq ?? 0;
      const { data } = await IMSDK.getAdvancedHistoryMessageListReverse({
        userID: "",
        groupID: "",
        count: GET_HISTORY_MESSAGE_COUNT,
        lastMinSeq,
        startClientMsgID: prevList[0]?.clientMsgID ?? "",
        conversationID,
      });

      if (
        conversationID !==
        useConversationStore.getState().currentConversation?.conversationID
      )
        return;

      (data.messageList as ExMessageItem[]).map((message, idx) => {
        if (!idx) return;
        const prevTime = data.messageList[idx - 1]?.sendTime;
        if (message.sessionType === SessionType.Notification) {
          (data.messageList[idx - 1] as ExMessageItem).gapTime =
            message.sendTime - prevTime > 300000;
        } else {
          message.gapTime = message.sendTime - prevTime > 300000;
        }
      });
      const nextList = [...data.messageList.reverse(), ...prevList];

      set(() => ({
        laterHasMore: data.messageList.length !== 0,
        historyMessageList: nextList,
      }));
      console.info(get().historyMessageList);
    } catch (error) {
      feedbackToast({ error, msg: t("toast.getHistoryMessageFailed") });
      set(() => ({
        laterHasMore: false,
        historyMessageList: [],
      }));
    }
  },
  getTwoWayHistoryMessage: async ({ message, conversationID }) => {
    set(() => ({ jumpLoading: true }));
    const loadOptions = {
      userID: "",
      groupID: "",
      count: GET_HISTORY_MESSAGE_COUNT,
      lastMinSeq: message.seq,
      startClientMsgID: message.clientMsgID,
      conversationID,
    };
    const [{ data: earlierData }, { data: laterData }] = await Promise.all([
      IMSDK.getAdvancedHistoryMessageList(loadOptions),
      IMSDK.getAdvancedHistoryMessageListReverse(loadOptions),
    ]);
    const totalList = [
      ...earlierData.messageList,
      message,
      ...laterData.messageList,
    ].reverse();
    totalList.map((message, idx) => {
      if (!idx) return;
      const prevTime = totalList[idx - 1]?.sendTime;
      (totalList[idx - 1] as ExMessageItem).gapTime =
        message.sendTime - prevTime > 300000;
    });
    console.log(earlierData.messageList);
    console.log(laterData.messageList);
    console.log(totalList);

    set(() => ({
      lastMinSeq: earlierData.messageList.reduce((min,m) => m.seq > 0 && (!min || m.seq < min) ? m.seq : min, 0),
      jumpLoading: false,
      jumpClientMsgID: message.clientMsgID,
      hasMore: earlierData.messageList.length !== 0,
      laterHasMore: laterData.messageList.length !== 0,
      historyMessageList: totalList,
    }));
  },
  clearAppendState: () => {
    set((state) => ({
      historyMessageList: state.historyMessageList.map((message) => ({
        ...message,
        isAppend: false,
      })),
    }));
  },
  updateJumpClientMsgID: (jumpClientMsgID?: string) => set(() => ({ jumpClientMsgID })),
  pushNewMessage: (message: ExMessageItem) => {
    set((state) => ({
      historyMessageList: [message, ...state.historyMessageList],
    }));
  },
  updateOneMessage: (message: ExMessageItem, fromMediaDownload = false) => {
    // update download state
    if (fromMediaDownload) {
      const tmpPreviewList = [...get().previewImgList];
      const previewIdx = tmpPreviewList.findIndex(
        (item) => item.clientMsgID === message.clientMsgID,
      );
      if (previewIdx > -1) {
        const field = tmpPreviewList[previewIdx].videoUrl ? "videoUrl" : "url";
        tmpPreviewList[previewIdx][field] = toLocalFileURL(message.localEx);
        set(() => ({ previewImgList: tmpPreviewList }));
      }
    }

    const tmpList = [...get().historyMessageList];
    const idx = tmpList.findIndex((msg) => msg.clientMsgID === message.clientMsgID);
    if (idx < 0) {
      return;
    }
    tmpList[idx] = { ...tmpList[idx], ...message };

    set(() => ({ historyMessageList: tmpList }));
  },
  deleteOneMessage: (clientMsgID: string) => {
    const tmpList = get().historyMessageList;
    const idx = tmpList.findIndex((msg) => msg.clientMsgID === clientMsgID);
    if (idx < 0) {
      return;
    }
    tmpList.splice(idx, 1);
    set(() => ({ historyMessageList: [...tmpList] }));
  },
  deleteAndPushOneMessage: (message: ExMessageItem) => {
    const tmpList = get().historyMessageList;
    const idx = tmpList.findIndex((msg) => msg.clientMsgID === message.clientMsgID);
    if (idx < 0) {
      return;
    }
    tmpList.splice(idx, 1);
    set(() => ({ historyMessageList: [message, ...tmpList] }));
  },
  updateMessageNicknameAndFaceUrl: ({
    sendID,
    senderFaceUrl,
    senderNickname,
  }: UpdateMessaggeBaseInfoParams) => {
    const tmpList = [...get().historyMessageList].map((message) => {
      if (message.sendID === sendID) {
        message.senderFaceUrl = senderFaceUrl;
        message.senderNickname = senderNickname;
      }
      return message;
    });
    set(() => ({ historyMessageList: tmpList }));
  },
  clearHistoryMessage: () => {
    set(() => ({ historyMessageList: [], previewImgList: [], hasMore: false }));
  },
  updateCheckMode: (isCheckMode: boolean) => {
    if (!isCheckMode) {
      const tmpList = [...get().historyMessageList].map((message) => {
        message.checked = false;
        return message;
      });
      set(() => ({ historyMessageList: tmpList }));
    }
    set(() => ({ isCheckMode }));
  },
  getConversationPreviewImgList: async () => {
    const conversationID =
      useConversationStore.getState().currentConversation?.conversationID;

    if (!conversationID) return;
    const {
      data: { searchResultItems },
    } = await IMSDK.searchLocalMessages({
      conversationID,
      keywordList: [],
      keywordListMatchType: 0,
      senderUserIDList: [],
      messageTypeList: [MessageType.PictureMessage, MessageType.VideoMessage],
      searchTimePosition: 0,
      searchTimePeriod: 0,
      pageIndex: 1,
      count: 200,
    });
    if (!searchResultItems?.[0].messageCount) return;
    console.log(searchResultItems[0].messageList);
    const newPreviewImgList = searchResultItems[0].messageList.map((item) => ({
      url: getImageMessageSourceUrl(item) ?? "",
      clientMsgID: item.clientMsgID,
      videoUrl: getVideoMessageSourceUrl(item),
      thumbUrl: item.pictureElem?.snapshotPicture.url ?? "",
    }));
    set(() => ({ previewImgList: [...newPreviewImgList] }));
  },
  tryAddPreviewImg: (mesageList: ExMessageItem[]) => {
    const previews = mesageList
      .filter((message) => MediaMessageTypes.includes(message.contentType))
      .map((message) => ({
        url: getImageMessageSourceUrl(message) ?? "",
        clientMsgID: message.clientMsgID,
        videoUrl: getVideoMessageSourceUrl(message),
        thumbUrl: message.pictureElem?.snapshotPicture.url ?? "",
      }));
    if (previews.length === 0) return;
    set((state) => ({
      previewImgList: [...previews, ...state.previewImgList],
    }));
  },
  addDownloadTask: (url: string, data: DownloadData) => {
    set((state) => ({
      downloadMap: { ...state.downloadMap, [url]: { ...data } },
    }));
  },
  updateDownloadTask: (url: string, data: DownloadData) => {
    const tmpMap = { ...get().downloadMap };
    tmpMap[url] = {
      ...tmpMap[url],
      ...data,
    };
    set(() => ({ downloadMap: tmpMap }));
  },
  removeDownloadTask: (url: string) => {
    const tmpMap = { ...get().downloadMap };
    if (!tmpMap[url]) return;
    delete tmpMap[url];
    set(() => ({ downloadMap: tmpMap }));
  },
}));

const MediaMessageTypes = [MessageType.PictureMessage, MessageType.VideoMessage];

export const getImageMessageSourceUrl = (message: ExMessageItem) => {
  if (message.contentType === MessageType.VideoMessage) {
    const snapshotPath = message.videoElem.snapshotPath;
    if (snapshotPath && window.electronAPI?.fileExists(snapshotPath)) {
      return toLocalFileURL(snapshotPath);
    }
    const snapshotUrl = message.videoElem.snapshotUrl;
    const cachePath = useUserStore.getState().imageCache[snapshotUrl];
    if (cachePath && window.electronAPI?.fileExists(cachePath)) {
      return toLocalFileURL(cachePath);
    }
    return snapshotUrl;
  }

  if (message.localEx && window.electronAPI?.fileExists(message.localEx)) {
    return toLocalFileURL(message.localEx);
  }
  if (window.electronAPI?.fileExists(message.pictureElem.sourcePath)) {
    return toLocalFileURL(message.pictureElem.sourcePath);
  }
  return message.pictureElem.sourcePicture.url;
};

export const getVideoMessageSourceUrl = (message: ExMessageItem) => {
  if (message.contentType !== MessageType.VideoMessage) return undefined;
  if (message.localEx && window.electronAPI?.fileExists(message.localEx)) {
    return toLocalFileURL(message.localEx);
  }
  if (window.electronAPI?.fileExists(message.videoElem.videoPath)) {
    return toLocalFileURL(message.videoElem.videoPath);
  }
  return message.videoElem.videoUrl;
};
