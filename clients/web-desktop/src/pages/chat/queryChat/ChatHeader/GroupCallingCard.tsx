import clsx from "clsx";
import { t } from "i18next";
import {
  GroupItem,
  GroupMemberItem,
  RtcInvite,
} from "@/im/types";
import { useEffect, useState } from "react";

import arrow_down from "@/assets/images/chatHeader/arrow_down.png";
import arrow_up from "@/assets/images/chatHeader/arrow_up.png";
import member_etc from "@/assets/images/chatHeader/member_etc.png";
import OIMAvatar from "@/components/OIMAvatar";
import { IMSDK } from "@/layout/MainContentWrap";
import { ParticipantInfo } from "@/pages/common/RtcCallModal/data";
import emitter from "@/utils/events";

import styles from "./chat-header.module.scss";

export interface CallingRoomData {
  participant?: ParticipantInfo[];
  invitation?: RtcInvite;
  roomID: string;
}

const GroupCallingCard = ({ currentGroupInfo }: { currentGroupInfo?: GroupItem }) => {
  const [expanded, setExpanded] = useState(false);
  const [roomData, setRoomData] = useState<CallingRoomData>();
  const [memberList, setMemberList] = useState<GroupMemberItem[]>([]);

  useEffect(() => {
    if (!currentGroupInfo?.groupID) {
      resetData();
      return;
    }
    IMSDK.signalingGetRoomByGroupID<CallingRoomData>(currentGroupInfo.groupID).then(
      ({ data }) => {
        if (data.invitation) {
          setRoomData(data);
          getMemberList(data.invitation.inviteeUserIDList);
        } else {
          resetData();
        }
      },
    );
  }, [currentGroupInfo?.groupID]);

  const getMemberList = (userIDList: string[]) => {
    IMSDK.getSpecifiedGroupMembersInfo({
      groupID: currentGroupInfo!.groupID,
      userIDList: userIDList.slice(0, 5),
    }).then(({ data }) => setMemberList(data));
  };

  const resetData = () => {
    setRoomData(undefined);
    setMemberList([]);
  };

  if (!roomData) return null;

  const updateExpanded = () => {
    setExpanded((v) => !v);
  };

  const joinCalling = () => {
    emitter.emit("OPEN_RTC_MODAL", {
      invitation: roomData.invitation,
      participant: roomData.participant?.[0],
      isJoin: true,
    });
  };

  const isVideoCall = roomData.invitation?.mediaType === "video";
  const callingTitle = `${roomData.participant?.[0]?.groupMemberInfo?.nickname}${t(
    "placeholder.launch",
  )}${isVideoCall ? t("placeholder.videoCall") : t("placeholder.voiceCall")}${t(
    "placeholder.underWay",
  )}`;

  return (
    <div className={clsx(styles["calling-card"])}>
      <div className="flex items-center">
        <div className="mr-2 flex-1 truncate text-xs text-[var(--sub-text)]">
          {callingTitle}
        </div>
        <img
          className="cursor-pointer"
          onClick={updateExpanded}
          src={expanded ? arrow_up : arrow_down}
          width={16}
          alt=""
        />
      </div>
      <div
        className={clsx(styles["can-expanded-content"], {
          "!max-h-96": expanded,
        })}
      >
        <div className="my-2">
          {memberList.map((member, idx) =>
            idx < 4 ? (
              <OIMAvatar
                key={member.userID}
                className="mr-2"
                src={member.faceURL}
                text={member.nickname}
                size={36}
              />
            ) : (
              <OIMAvatar key={member.userID} src={member_etc} size={36} />
            ),
          )}
        </div>
        <div className="flex items-center justify-center">
          <span
            className="cursor-pointer text-xs text-[var(--primary)]"
            onClick={joinCalling}
          >
            {t("placeholder.joinCall")}
          </span>
        </div>
      </div>
    </div>
  );
};

export default GroupCallingCard;
