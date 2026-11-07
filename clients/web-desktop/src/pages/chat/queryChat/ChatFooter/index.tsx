import { useDebounceFn, useLatest, useThrottleFn } from "ahooks";
import { Button, Popover } from "antd";
import { t } from "i18next";
import { GroupMemberItem } from "@/im/types";
import { memo, RefObject, useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";

import cricle_cancel from "@/assets/images/chatFooter/cricle_cancel.png";
import { message as antdMessage } from "@/AntdGlobalComp";
import { fileMessageErrorKey } from "@/im/local-media";
import EditableDiv, {
  deleteBeforeAt,
  EditableDivEvent,
  insertAtCursor,
  restoreSelection,
} from "@/components/EditableDiv";
import { parseTwemoji } from "@/components/Twemoji";
import { IMSDK } from "@/layout/MainContentWrap";
import { ExMessageItem, useConversationStore } from "@/store";
import { getExtraStr } from "@/utils/common";
import emitter from "@/utils/events";
import { formatMessageByType } from "@/utils/imCommon";

import {
  AtSearchPanelHandle,
  ForwardAtSearchPanel,
  SnapPreviewGroup,
} from "./AtSearchPanel";
import SendActionBar from "./SendActionBar";
import { useFileMessage } from "./SendActionBar/useFileMessage";
import { useDropDomOnly } from "./useDropDomOnly";
import { useSendMessage } from "./useSendMessage";

const ChatFooter = () => {
  const [html, setHtml] = useState("");
  const [atPanelState, setAtPanelState] = useState({
    visible: false,
    originStr: "",
  });
  const latestPanelState = useLatest(atPanelState);
  const latestHtml = useLatest(html);
  const { conversationID } = useParams();

  const editableDivRef = useRef<{ el: RefObject<HTMLDivElement> }>(null);
  const atPanelRef = useRef<AtSearchPanelHandle>(null);
  const quoteMessage = useConversationStore((state) => state.quoteMessage);
  const latestQuoteMessage = useLatest(quoteMessage);
  const updateQuoteMessage = useConversationStore((state) => state.updateQuoteMessage);

  const { createFileMessage } = useFileMessage();
  const { sendMessage } = useSendMessage();

  useDropDomOnly({ domRef: editableDivRef.current?.el, sendMessage });

  const drft = useRef("");

  useEffect(() => {
    window.editRevoke = (clientMsgID: string) => {
      const item = useConversationStore.getState().revokeMap[clientMsgID];
      updateQuoteMessage(item.quoteMessage);
      setHtml(item.text);
      drft.current = item.text;
      inputFocus();
    };
    emitter.on("TRIGGER_GROUP_AT", atHandler);
    return () => {
      emitter.off("TRIGGER_GROUP_AT", atHandler);
    };
  }, []);

  useEffect(() => {
    if (quoteMessage) {
      inputFocus();
    }
  }, [quoteMessage]);

  useEffect(() => {
    const oldDraftText = useConversationStore.getState().currentConversation?.draftText;
    inputFocus();
    checkSavedDraft(oldDraftText);
    return () => {
      if (!editableDivRef.current?.el.current) return;
      checkDraftSave(drft.current, oldDraftText);
    };
  }, [conversationID]);

  const inputFocus = () => {
    restoreSelection();
    editableDivRef.current?.el.current?.focus();
  };

  const checkSavedDraft = (draftText?: string) => {
    if (draftText) {
      draftText = parseTwemoji(draftText);
    }
    setHtml(draftText ?? "");
    drft.current = draftText ?? "";
  };

  const checkDraftSave = (html: string, oldDraftText?: string) => {
    let cleanText = html;
    const atEls = getAtList();

    if (atEls.length > 0) {
      atEls.map((el) => (cleanText = cleanText.replace(el.tag, `@${el.userID} `)));
      const pattern = /@\S+\s/g;
      const arr = cleanText.match(pattern);
      arr?.map((item) => {
        const member = atEls.find((el) => el.userID === item.slice(1, -1));
        if (member) {
          const reg = new RegExp(item, "g");
          cleanText = cleanText.replace(
            reg,
            `<b class="at-el" contenteditable="false" data-id="${member.userID}" data-name="${member.nickname}">@${member.nickname} </b>`,
          );
        }
      });
      cleanText = replaceEmoji2Str(cleanText);
    } else {
      cleanText = getCleanTextExceptImg(html);
    }
    if (!conversationID) return;
    if (
      (cleanText.trim() && cleanText !== oldDraftText) ||
      (!cleanText && oldDraftText)
    ) {
      IMSDK.setConversationDraft({
        conversationID,
        draftText: cleanText,
      });
    }
  };

  const updateTyping = () => {
    const currentUserID = useConversationStore.getState().currentConversation?.userID;
    if (!currentUserID) return;
    IMSDK.typingStatusUpdate({
      recvID: currentUserID,
      msgTip: "yes",
    });
  };

  const { run: throttleTyping } = useThrottleFn(updateTyping, { wait: 1500 });

  const { run: debounceSearch } = useDebounceFn(
    (keyword: string) => {
      if (!latestPanelState.current.visible) return;
      const originStr = latestPanelState.current.originStr;
      const searKeyword = getExtraStr(getCleanText(originStr), getCleanText(keyword));
      atPanelRef.current?.searchMember(searKeyword);
    },
    { wait: 100 },
  );

  const closeAtPanel = () => {
    setAtPanelState({
      visible: false,
      originStr: "",
    });
  };

  const onChange = (e: EditableDivEvent) => {
    setHtml(e.target.value);
    drft.current = e.target.value;
    debounceSearch(e.target.value);
    if (e.target.value) {
      throttleTyping();
    }
    if (!e.target.value && latestPanelState.current.visible) {
      closeAtPanel();
    }
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (
      (e.key === "ArrowUp" || e.key === "ArrowDown") &&
      latestPanelState.current.visible
    ) {
      e.preventDefault();
    }

    if (
      (e.key === "ArrowLeft" || e.key === "ArrowRight") &&
      latestPanelState.current.visible
    ) {
      closeAtPanel();
    }

    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (latestPanelState.current.visible) return;

      debounceSend();
    }

    if (
      e.code === "Digit2" &&
      e.shiftKey &&
      useConversationStore.getState().currentConversation?.groupID &&
      !latestPanelState.current.visible
    ) {
      setTimeout(() => {
        atPanelRef.current?.searchMember("");
        editableDivRef.current?.el.current?.focus();
      }, 100);

      setTimeout(() => editableDivRef.current?.el.current?.blur());
      setAtPanelState({
        visible: true,
        originStr: latestHtml.current,
      });
    }

    if (e.key === "Backspace" || e.key === "Delete") {
      const selection = window.getSelection();
      const range = selection!.getRangeAt(0);
      if (
        latestPanelState.current.visible &&
        range.startOffset > 0 &&
        range.startContainer.nodeType === Node.TEXT_NODE
      ) {
        const charBeforeCursor = range.startContainer.textContent?.charAt(
          range.startOffset - 1,
        );
        if (charBeforeCursor === "@") {
          closeAtPanel();
        }
      }
    }
  };

  const getImageEl = () => {
    const editableDiv = editableDivRef.current?.el.current;
    if (!editableDiv) return;

    const imageEls = [
      ...editableDiv.querySelectorAll(".image-el"),
    ] as HTMLImageElement[];
    imageEls.map(async (el) => {
      try {
        const blob = await fetch(el.src).then((res) => res.blob());
        const file = new File([blob], `screenshot${Date.now()}.png`, { type: blob.type });
        const message = await createFileMessage(file);
        await sendMessage({ message, throwOnError: true });
      } catch (error) { antdMessage.error(t(fileMessageErrorKey(error))); }
    });
  };

  const getAtList = () => {
    const editableDiv = editableDivRef.current?.el.current;
    if (!editableDiv) return [];

    const atels = Array.from(editableDiv.querySelectorAll(".at-el"));
    return atels.map((at) => ({
      userID: at.attributes.getNamedItem("data-id")!.value,
      nickname: at.attributes.getNamedItem("data-name")!.value,
      tag: at.outerHTML,
    }));
  };

  const replaceEmoji2Str = (text: string) => {
    const parser = new DOMParser();
    const doc = parser.parseFromString(text, "text/html");

    const emojiEls: HTMLImageElement[] = Array.from(doc.querySelectorAll(".emojione"));
    emojiEls.map((face) => {
      // @ts-ignore
      const escapedOut = face.outerHTML.replace(/[-/\\^$*+?.()|[\]{}]/g, "\\$&");
      text = text.replace(new RegExp(escapedOut, "g"), face.alt);
    });
    return text;
  };

  const getCleanText = (html: string) => {
    html = replaceEmoji2Str(html);
    const regWithoutHtml = /(<([^>]+)>)/gi;
    return html.replace(regWithoutHtml, "");
  };

  const getCleanTextExceptImg = (html: string) => {
    html = replaceEmoji2Str(html);
    const regWithoutHtmlExceptImg = /<(?!img\s*\/?)[^>]+>/gi;
    return html.replace(regWithoutHtmlExceptImg, "");
  };

  const getTextMessage = async (cleanText: string) => {
    const atEls = getAtList();

    if (
      useConversationStore.getState().currentConversation?.groupID &&
      atEls.length > 0
    ) {
      let formatAtText = latestHtml.current;
      atEls.map(
        (el) => (formatAtText = formatAtText.replace(el.tag, `@${el.userID} `)),
      );
      return (
        await IMSDK.createTextAtMessage({
          text: getCleanText(formatAtText),
          atUserIDList: atEls.map((at) => at.userID),
          atUsersInfo: atEls.map((at) => ({
            atUserID: at.userID,
            groupNickname: at.nickname,
          })),
          message: latestQuoteMessage.current,
        })
      ).data;
    }
    if (latestQuoteMessage.current) {
      return (
        await IMSDK.createQuoteMessage({
          text: cleanText,
          message: JSON.stringify(latestQuoteMessage.current),
        })
      ).data;
    }

    return (await IMSDK.createTextMessage(cleanText)).data;
  };

  const enterToSend = async () => {
    const cleanText = getCleanText(latestHtml.current);
    getImageEl();
    const message = await getTextMessage(cleanText);
    setHtml("");
    drft.current = "";
    if (!cleanText.trim()) return;

    sendMessage({ message });
    if (latestQuoteMessage.current) {
      updateQuoteMessage();
    }
  };

  const { run: debounceSend } = useDebounceFn(enterToSend, { wait: 200 });

  const atHandler = (atUser: GroupMemberItem, needDelete = false) => {
    const atEls = getAtList();
    if (atEls.find((el) => el.userID === atUser.userID) || atEls.length > 9) return;

    if (latestPanelState.current.visible) {
      closeAtPanel();
    }
    const el = document.createElement("b");
    el.setAttribute("class", "at-el");
    el.setAttribute("contenteditable", "false");
    el.setAttribute("data-id", atUser.userID);
    el.setAttribute("data-name", atUser.nickname);
    el.innerText = `@${atUser.nickname} `;
    if (needDelete) deleteBeforeAt();
    insertAtCursor([el, document.createTextNode("\u200B")]);
    updateHtml();
  };

  const updateHtml = useCallback(() => {
    // @ts-ignore
    editableDivRef.current?.emitChange();
  }, []);

  return (
    <footer className="relative min-h-[25vh] bg-white py-px">
      <div className="flex h-full flex-col border-t border-t-[var(--gap-text)]">
        <SendActionBar
          inputFocus={inputFocus}
          updateHtml={updateHtml}
          sendMessage={sendMessage}
          createFileMessage={createFileMessage}
        />
        <Popover
          overlayClassName="conversation-popover"
          placement="topLeft"
          title={null}
          arrow={false}
          open={atPanelState.visible}
          destroyTooltipOnHide
          content={
            <ForwardAtSearchPanel
              ref={atPanelRef}
              open={atPanelState.visible}
              closePanel={closeAtPanel}
              atHandler={atHandler}
              enterToSend={debounceSend}
            />
          }
        >
          <div className="relative flex flex-1 flex-col overflow-y-auto">
            {quoteMessage && (
              <div className="mx-5.5 mt-3 flex w-fit items-start rounded-md bg-[var(--chat-bubble)] px-2.5 py-2">
                <img
                  className="mt-px cursor-pointer"
                  width={13}
                  src={cricle_cancel}
                  alt="cancel"
                  onClick={() => updateQuoteMessage()}
                />
                <div
                  className="ml-1.5 line-clamp-1 text-xs text-[var(--sub-text)]"
                  title=""
                >{`${t("placeholder.reply")}${
                  quoteMessage.senderNickname
                }：${formatMessageByType(quoteMessage)}`}</div>
              </div>
            )}
            <EditableDiv
              ref={editableDivRef as any}
              id="editable-div"
              className="flex-1"
              html={html}
              onChange={onChange}
              onKeyDown={onKeyDown}
              onContextMenu={() =>
                window.electronAPI?.ipcInvoke("showInputContextMenu")
              }
            />
            <div className="absolute bottom-6 right-6 flex w-fit items-center justify-end">
              <span className="mr-2.5 text-xs text-[var(--sub-text)]">
                {t("placeholder.sendShortcutkey")}
              </span>
              <Button className="px-6 py-1" type="primary" onClick={debounceSend}>
                {t("placeholder.send")}
              </Button>
            </div>
          </div>
        </Popover>
        <SnapPreviewGroup />
      </div>
    </footer>
  );
};

export default memo(ChatFooter);
