export * from "@openim/wasm-client-sdk/lib/types/entity";
import type { SearchMessageResult as CoreSearchMessageResult, SearchMessageResultItem as CoreSearchMessageResultItem, AtTextElem as CoreAtTextElem, MergeElem as CoreMergeElem, QuoteElem as CoreQuoteElem, MessageItem as CoreMessageItem, PublicUserItem,FriendUserItem,GroupMemberItem } from "@openim/wasm-client-sdk";
export type FullUserItemWithCache={publicInfo:PublicUserItem;friendInfo?:FriendUserItem;groupMemberInfo?:GroupMemberItem};

/** Original enterprise UI contract; payload adaptation is centralized in ImFacade. */
export type MessageItem = Omit<Required<CoreMessageItem>,"quoteElem"|"mergeElem"|"atTextElem"> & {quoteElem:QuoteElem;mergeElem:MergeElem;atTextElem:AtTextElem};
export type QuoteElem=Omit<CoreQuoteElem,"quoteMessage"> & {quoteMessage:MessageItem};
export type MergeElem=Omit<CoreMergeElem,"multiMessage"> & {multiMessage:MessageItem[]};
export type AtTextElem=Omit<CoreAtTextElem,"quoteMessage"> & {quoteMessage?:MessageItem};

export type SearchMessageResultItem=Omit<CoreSearchMessageResultItem,"messageList"> & {messageList:MessageItem[]};
export type SearchMessageResult=Omit<CoreSearchMessageResult,"searchResultItems"|"findResultItems"> & {searchResultItems?:SearchMessageResultItem[];findResultItems?:SearchMessageResultItem[]};
