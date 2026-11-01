import type {
  TrackReference,
  TrackReferencePlaceholder,
} from "@livekit/components-core";
import { useIsMuted } from "@livekit/components-react";
import { Track } from "livekit-client";
import { PublicUserItem } from "@/im/types";
import { memo } from "react";

import meeting_member_host from "@/assets/images/rtc/meeting_member_host.png";
import meeting_member_mute from "@/assets/images/rtc/meeting_member_mute.png";
import meeting_member_muted from "@/assets/images/rtc/meeting_member_muted.png";
import OIMAvatar from "@/components/OIMAvatar";

type VoiceMemberItemProps = {
  hostUserID: string;
  track: TrackReference | TrackReferencePlaceholder;
};
export const VoiceMemberItem = memo(({ hostUserID, track }: VoiceMemberItemProps) => {
  const isAudioMuted = useIsMuted({
    participant: track.participant,
    source: Track.Source.Microphone,
  });

  const userInfo: PublicUserItem | undefined = JSON.parse(
    track.participant?.metadata ?? "{}",
  ).userInfo;

  return (
    <div className="relative mx-10 flex flex-col items-center">
      <OIMAvatar
        size={72}
        shape="circle"
        src={userInfo?.faceURL}
        text={userInfo?.nickname}
      />
      <div className="mt-2 flex items-center">
        <div className="mr-1 max-w-[80px] truncate">{userInfo?.nickname}</div>
        <img
          className="h-[13px] w-[13px]"
          src={!isAudioMuted ? meeting_member_mute : meeting_member_muted}
          alt=""
        />
      </div>
      {hostUserID === track.participant.identity && (
        <img
          className="absolute top-16 h-[13px] w-[13px]"
          src={meeting_member_host}
          alt=""
        />
      )}
    </div>
  );
});
