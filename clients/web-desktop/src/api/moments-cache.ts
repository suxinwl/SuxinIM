import type { InfiniteData, QueryClient } from "react-query";

import type { WorkMoments } from "@/types/moment";

// A common prefix updates the feed, author pages and detail together.
export const momentKeys = {
  lists: ["WorkMoments"] as const,
  list: (userID?: string) => ["WorkMoments", userID ? `user:${userID}` : "feed"],
  details: ["WorkMoment"] as const,
  detail: (id: string) => ["WorkMoment", id],
};

export function updateMomentCaches(client: QueryClient, moment: WorkMoments) {
  if (!moment?.workMomentID || !moment.content) return;
  client.setQueriesData<InfiniteData<WorkMoments[]> | undefined>(momentKeys.lists, (old) => {
    if (!old) return old;
    return {
      ...old,
      pages: old.pages.map((page) =>
        page.map((item) => (item.workMomentID === moment.workMomentID ? moment : item)),
      ),
    };
  });
  client.setQueryData(momentKeys.detail(moment.workMomentID), moment);
}

export function removeMomentCaches(client: QueryClient, id: string) {
  client.setQueriesData<InfiniteData<WorkMoments[]> | undefined>(momentKeys.lists, (old) => {
    if (!old) return old;
    return {
      ...old,
      pages: old.pages.map((page) => page.filter((item) => item.workMomentID !== id)),
    };
  });
  client.removeQueries(momentKeys.detail(id), { exact: true });
}

export async function cancelMomentQueries(client: QueryClient) {
  await Promise.all([
    client.cancelQueries(momentKeys.lists),
    client.cancelQueries(momentKeys.details),
  ]);
}

export function refreshMomentQueries(client: QueryClient) {
  // Logs contain event-specific actors, so refetch instead of replacing them.
  void client.invalidateQueries(momentKeys.lists);
  void client.invalidateQueries(momentKeys.details);
  void client.invalidateQueries(["MomentsMessageList"]);
}

export function momentMutationOptions(client: QueryClient) {
  return {
    onMutate: () => cancelMomentQueries(client),
    onSuccess: async (response: { data: { workMoment: WorkMoments } }) => {
      // Cancel interval requests begun during the mutation, so an older response
      // cannot erase the like/comment which the server just confirmed.
      await cancelMomentQueries(client);
      updateMomentCaches(client, response.data.workMoment);
    },
    onSettled: () => refreshMomentQueries(client),
  };
}

export function uniqueMoments(pages: WorkMoments[][] = []) {
  const seen = new Set<string>();
  return pages.flat().filter((item) => {
    if (!item || seen.has(item.workMomentID)) return false;
    seen.add(item.workMomentID);
    return true;
  });
}
