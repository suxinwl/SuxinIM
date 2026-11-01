type ActivityDocument = Pick<
  Document,
  "visibilityState" | "hasFocus" | "addEventListener" | "removeEventListener"
>;
type ActivityWindow = Pick<Window, "addEventListener" | "removeEventListener">;

// IntersectionObserver can keep reporting an intersecting message after the
// Electron window is hidden to the tray. A receipt also needs foreground focus.
export const isReadWindowActive = (document: ActivityDocument) =>
  document.visibilityState === "visible" && document.hasFocus();

export function createReadActivityStore(
  window: ActivityWindow,
  document: ActivityDocument,
) {
  const listeners = new Set<() => void>();
  let pageShown = true;
  const notify = () => listeners.forEach((listener) => listener());
  const pageHide = () => {
    pageShown = false;
    notify();
  };
  const pageShow = () => {
    pageShown = true;
    notify();
  };
  return {
    getSnapshot: () => pageShown && isReadWindowActive(document),
    subscribe(listener: () => void) {
      if (listeners.size === 0) {
        window.addEventListener("focus", notify);
        window.addEventListener("blur", notify);
        window.addEventListener("pagehide", pageHide);
        window.addEventListener("pageshow", pageShow);
        document.addEventListener("visibilitychange", notify);
      }
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
        if (listeners.size === 0) {
          window.removeEventListener("focus", notify);
          window.removeEventListener("blur", notify);
          window.removeEventListener("pagehide", pageHide);
          window.removeEventListener("pageshow", pageShow);
          document.removeEventListener("visibilitychange", notify);
        }
      };
    },
  };
}

type ConversationReadState = {
  conversationID: string;
  unreadCount: number;
  resetGroupAt: boolean;
};

export function scheduleConversationRead(options: {
  conversationID: string;
  canRead: () => boolean;
  getConversation: () => ConversationReadState | undefined;
  markRead: (conversationID: string) => Promise<unknown>;
  resetGroupAt: (conversationID: string) => Promise<unknown>;
}) {
  let cancelled = false;
  const timer = setTimeout(() => {
    if (cancelled || !options.canRead()) return;
    const conversation = options.getConversation();
    if (conversation?.conversationID !== options.conversationID) return;
    if (conversation.unreadCount > 0) {
      try {
        void options.markRead(conversation.conversationID).catch(() => undefined);
      } catch {
        // A failed SDK request stays unread and can be retried on the next visit.
      }
    }
    if (
      !cancelled &&
      options.canRead() &&
      options.getConversation()?.conversationID === options.conversationID &&
      conversation.resetGroupAt
    ) {
      try {
        void options.resetGroupAt(conversation.conversationID).catch(() => undefined);
      } catch {
        // Keep the mention marker if the SDK cannot reset it.
      }
    }
  }, 150);
  return () => {
    cancelled = true;
    clearTimeout(timer);
  };
}

export class ReadReceiptCancelled extends Error {
  constructor() {
    super("The message is no longer visible in the active conversation");
  }
}

export async function sendVisibleReadReceipt<T, R>(
  canRead: () => boolean,
  prepare: () => Promise<T>,
  send: (payload: T) => Promise<R>,
): Promise<R> {
  if (!canRead()) throw new ReadReceiptCancelled();
  const payload = await prepare();
  // Finding a message in the WASM SQLite cache is asynchronous. Recheck after
  // that lookup so hiding the window or switching chats cancels the receipt.
  if (!canRead()) throw new ReadReceiptCancelled();
  return send(payload);
}
