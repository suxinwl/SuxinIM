import type {
  TrackReference,
  TrackReferencePlaceholder,
} from "@livekit/components-core";
import { TrackMutedIndicator, useIsMuted, VideoTrack } from "@livekit/components-react";
import clsx from "clsx";
import { Track } from "livekit-client";
import { PublicUserItem } from "@/im/types";
import { useMemo } from "react";

import OIMAvatar from "@/components/OIMAvatar";

export const GroupTrackItem = ({
  track,
  isSingle,
  className,
}: {
  track: TrackReference | TrackReferencePlaceholder;
  isSingle?: boolean;
  className?: string;
}) => {
  const isVideoMuted = useIsMuted({
    participant: track.participant,
    source: Track.Source.Camera,
  });

  const itemInfo: PublicUserItem = useMemo(() => {
    const metadata = track.participant.metadata;
    if (!metadata) {
      return {} as PublicUserItem;
    }
    const parsedData = JSON.parse(metadata);
    return (parsedData[isSingle ? "userInfo" : "groupMemberInfo"] ??
      {}) as PublicUserItem;
  }, [track.participant.metadata, isSingle]);

  return (
    <div className={clsx("relative h-[92px] w-full", className)}>
      {!isVideoMuted && track.publication ? (
        <VideoTrack trackRef={{...track,publication:track.publication!}} />
      ) : (
        <OIMAvatar
          className="h-full w-full border-none"
          src={itemInfo.faceURL}
          text={itemInfo.nickname}
        />
      )}
      <div className="absolute bottom-2 left-2 flex items-center">
        <TrackMutedIndicator
          className="text-white"
          trackRef={{
            participant: track.participant,
            source: Track.Source.Microphone,
          }}
        />
        <div className="max-w-[50px] truncate rounded bg-[rgba(12,28,51,0.2)] px-1 py-1 text-xs text-white">
          {itemInfo.nickname}
        </div>
      </div>
    </div>
  );
};
