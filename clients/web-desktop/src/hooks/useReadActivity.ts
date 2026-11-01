import { useSyncExternalStore } from "react";

import { createReadActivityStore } from "@/im/read-receipt-gate";

const readActivity = createReadActivityStore(window, document);

export const isReadActivityActive = readActivity.getSnapshot;

export function useReadActivity() {
  return useSyncExternalStore(
    readActivity.subscribe,
    readActivity.getSnapshot,
    () => false,
  );
}
