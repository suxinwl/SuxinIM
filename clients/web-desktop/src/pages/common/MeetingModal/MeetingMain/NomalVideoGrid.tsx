import type { TrackReferenceOrPlaceholder } from "@livekit/components-core";
import { useTracks } from "@livekit/components-react";
import clsx from "clsx";
import { Track } from "livekit-client";
import { memo } from "react";

import { sortTrackReferences } from "../sorting";
import { VideoMemberItem } from "./VideoMemberItem";
import { VoiceMemberItem } from "./VoiceMemberItem";

type NomalVideoGridProps = {
  hostUserID: string;
  wrapHeight?: number;
  isOnlyAudio: boolean;
  toggleAllInOne: (identity: string) => void;
};
export const NomalVideoGrid = memo(
  ({ wrapHeight, hostUserID, isOnlyAudio, toggleAllInOne }: NomalVideoGridProps) => {
    const tracks = useTracks([
      { source: Track.Source.Camera, withPlaceholder: true },
      { source: Track.Source.ScreenShare, withPlaceholder: false },
    ]).reduce((acc: TrackReferenceOrPlaceholder[], cur) => {
      const idx = acc.findIndex(
        (track) => track.participant.identity === cur.participant.identity,
      );
      if (idx > -1 && cur.source === Track.Source.ScreenShare) {
        acc[idx] = cur;
      }
      if (idx < 0) {
        acc.push(cur);
      }
      return acc;
    }, []);

    const audioTracks = useTracks([
      { source: Track.Source.Microphone, withPlaceholder: true },
    ]);
    const sortedTracks = sortTrackReferences(tracks);

    return (
      <div
        className={clsx("flex w-full flex-wrap overflow-y-auto p-1", {
          "flex-nowrap items-center justify-center": isOnlyAudio,
        })}
      >
        {isOnlyAudio
          ? audioTracks.map((track) => (
              <VoiceMemberItem
                key={track.participant.identity}
                hostUserID={hostUserID}
                track={track}
              />
            ))
          : sortedTracks.map((track) => (
              <VideoMemberItem
                key={track.participant.identity}
                hostUserID={hostUserID}
                wrapHeight={wrapHeight}
                totalLength={tracks.length}
                track={track}
                toggleAllInOne={toggleAllInOne}
              />
            ))}
      </div>
    );
  },
);
