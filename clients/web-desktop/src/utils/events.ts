import { ChooseModalState, ChooseModalType } from "@/pages/common/ChooseModal";
import { CheckListItem } from "@/pages/common/ChooseModal/ChooseBox/CheckItem";
import mitt from "mitt";
import { GroupItem, GroupMemberItem } from "@/im/types";
import { MeetingAuthData } from "@/pages/common/MeetingManageModal/data";
import { InviteData } from "@/pages/common/RtcCallModal/data";

type EmitterEvents = {
  OPEN_USER_CARD: OpenUserCardParams;
  OPEN_GROUP_CARD: GroupItem;
  OPEN_CHOOSE_MODAL: ChooseModalState;
  OPEN_VIDEO_PLAYER: string;
  OPEN_RTC_MODAL: InviteData;
  OPEN_MEETING_MODAL: MeetingAuthData;
  CHAT_LIST_SCROLL_TO_BOTTOM: boolean;
  REFRESH_CHAT_LIST: void;
  ADD_NEW_MESSAGE_COUNT: void;
  SELECT_USER: SelectUserParams;
  OPEN_MOMENTS: void;
  CLOSE_SEARCH_MODAL: void;
  ONLINE_STATE_CHECK: void;
  TYPING_UPDATE: void;
  TRY_JUMP_TO_UNREAD: void;
  TRIGGER_GROUP_AT: GroupMemberItem;
};

export type SelectUserParams = {
  notConversation: boolean;
  choosedList: CheckListItem[];
};

export type OpenUserCardParams = {
  userID?: string;
  groupID?: string;
  isSelf?: boolean;
  notAdd?: boolean;
};

const emitter = mitt<EmitterEvents>();

export default emitter;
