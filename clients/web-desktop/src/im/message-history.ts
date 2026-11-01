import { MessageViewType } from "@openim/wasm-client-sdk";
import type { GetAdvancedHistoryMessageListParams } from "@openim/wasm-client-sdk";
import type { MessageItem } from "./types";

type HistoryResult = { isEnd: boolean; errCode?: number; errMsg?: string; messageList: MessageItem[] };
type HistorySDK = {
  getAdvancedHistoryMessageList: (request: GetAdvancedHistoryMessageListParams) => Promise<{ data: HistoryResult }>;
  getAdvancedHistoryMessageListReverse: (request: GetAdvancedHistoryMessageListParams) => Promise<{ data: HistoryResult }>;
};

/** Search and ordinary history use different sequence cursors in the pinned SDK. */
export async function loadMessageContext(
  sdk: HistorySDK,
  { conversationID, message, count = 20 }: { conversationID: string; message: MessageItem; count?: number },
) {
  if (!conversationID || !message.clientMsgID) throw new Error("A valid conversation and anchor message are required");
  const request = { conversationID, startClientMsgID: message.clientMsgID, count, viewType: MessageViewType.Search };
  const [{ data: before }, { data: after }] = await Promise.all([
    sdk.getAdvancedHistoryMessageList(request),
    sdk.getAdvancedHistoryMessageListReverse(request),
  ]);
  for (const page of [before, after]) {
    if (page.errCode) throw Object.assign(new Error(page.errMsg || "Message context could not be loaded"), { errCode: page.errCode });
  }
  const byID = new Map<string, MessageItem>();
  for (const item of [...before.messageList, ...after.messageList, message]) {
    if (item.clientMsgID) byID.set(item.clientMsgID, item);
  }
  const messageList = [...byID.values()].sort((left, right) => {
    if (left.seq > 0 && right.seq > 0 && left.seq !== right.seq) return right.seq - left.seq;
    return right.sendTime - left.sendTime;
  });
  return { messageList, hasMore: !before.isEnd, laterHasMore: !after.isEnd, lastMinSeq: messageList.reduce((min, item) => item.seq > 0 && (!min || item.seq < min) ? item.seq : min, 0) };
}
