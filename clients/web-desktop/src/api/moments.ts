import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "react-query";

import { getChatUrl } from "@/config";
import { WorkMoments, WorkMomentsResponse } from "@/types/moment";
import createAxiosInstance from "@/utils/request";

import {
  cancelMomentQueries,
  momentKeys,
  momentMutationOptions,
  refreshMomentQueries,
  removeMomentCaches,
} from "./moments-cache";

const request = createAxiosInstance(getChatUrl(), false);

// publish moments
export const usePublishMoments = (queryKey?: string) => {
  const queryClient = useQueryClient();

  return useMutation(
    (params: API.Moments.PublishMomentsParams) =>
      request.post<{ workMoment: WorkMoments }>("/office/work_moment/add", {
        ...params,
      }),
    momentMutationOptions(queryClient),
  );
};

export const fetchUserMoments = async (
  { pageParam = 1 },
  userID?: string,
): Promise<WorkMoments[]> => {
  const url = `/office/work_moment/find/${!userID ? "recv" : "send"}`;
  const { data } = await request.post<WorkMomentsResponse>(url, {
    userID,
    pagination: { pageNumber: pageParam, showNumber: 20 },
  });
  // Preserve previous pages on error instead of reporting a false empty feed.
  return data.workMoments ?? [];
};
export const useUserMoments = (enabled: boolean, queryKey?: string) => {
  return useInfiniteQuery<WorkMoments[]>(
    momentKeys.list(queryKey),
    (context) => fetchUserMoments(context, queryKey),
    {
      getNextPageParam: (lastPage, pages) =>
        lastPage?.length < 20 ? undefined : pages.length + 1,
      enabled,
      // Unlike/deletion notifications are incomplete on older servers. Poll only
      // while this circle is open and the document is visible.
      refetchInterval: enabled ? 5000 : false,
      refetchIntervalInBackground: false,
    },
  );
};

export const getMomentsByID = (workMomentID: string) =>
  request.post<{ workMoment: WorkMoments }>("/office/work_moment/get", {
    workMomentID,
  });

export const useMomentDetails = (enabled: boolean, initial?: WorkMoments) =>
  useQuery(
    momentKeys.detail(initial?.workMomentID ?? ""),
    async () => (await getMomentsByID(initial!.workMomentID)).data.workMoment,
    {
      enabled: enabled && Boolean(initial?.workMomentID),
      initialData: initial,
      refetchInterval: enabled ? 5000 : false,
      refetchIntervalInBackground: false,
    },
  );

export const useDeleteMoments = (queryKey?: string) => {
  const queryClient = useQueryClient();
  return useMutation(
    (workMomentID: string) =>
      request.post("/office/work_moment/del", {
        workMomentID,
      }),
    {
      onMutate: () => cancelMomentQueries(queryClient),
      onSuccess: async (_, id) => {
        await cancelMomentQueries(queryClient);
        removeMomentCaches(queryClient, id);
      },
      onSettled: () => refreshMomentQueries(queryClient),
    },
  );
};

export const useCreateComment = () => {
  const queryClient = useQueryClient();
  return useMutation(
    (params: API.Moments.CreateCommentParams) =>
      request.post<{ workMoment: WorkMoments }>("/office/work_moment/comment/add", {
        ...params,
      }),
    momentMutationOptions(queryClient),
  );
};

// delete comment
export const useDeleteComment = () => {
  const queryClient = useQueryClient();
  return useMutation(
    (params: API.Moments.DeleteCommentParams) =>
      request.post<{ workMoment: WorkMoments }>("/office/work_moment/comment/del", {
        ...params,
      }),
    momentMutationOptions(queryClient),
  );
};

// like
export const useLikeMoments = () => {
  const queryClient = useQueryClient();
  return useMutation(
    (params: { workMomentID: string; like: boolean }) =>
      request.post<{ workMoment: WorkMoments }>("/office/work_moment/like", {
        ...params,
      }),
    momentMutationOptions(queryClient),
  );
};

// Querying for no readings
export const getMomentsUnreadCount = () =>
  request.post("/office/work_moment/unread/count", {});

// Query message list
export const useLogs = () => {
  return useInfiniteQuery<{ data: WorkMomentsResponse }>(
    ["MomentsMessageList"],
    ({ pageParam = 1 }) =>
      request.post("/office/work_moment/logs", {
        pagination: {
          pageNumber: pageParam as number,
          showNumber: 20,
        },
      }),
    {
      getNextPageParam: (lastPage, pages) => {
        if ((lastPage.data.workMoments?.length ?? 0) < 20) return null;
        return pages.length + 1;
      },
    },
  );
};

// Query message list
export enum MomentsClearType {
  Count = 1,
  List = 2,
  All = 3,
}
export const useClearUnreadMoments = () => {
  return useMutation((type: MomentsClearType) =>
    request.post("/office/work_moment/unread/clear", {
      type,
    }),
  );
};
