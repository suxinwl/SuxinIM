import { LeftOutlined, RightOutlined } from "@ant-design/icons";
import type { TrackReferenceOrPlaceholder } from "@livekit/components-core";
import { useTracks } from "@livekit/components-react";
import clsx from "clsx";
import { Track } from "livekit-client";
import { memo, useState } from "react";

import styles from "./meeting-main.module.scss";
import { VideoMemberItem } from "./VideoMemberItem";

type AllInOneVideoGridProps = {
  allInOneUserID: string;
  hostUserID: string;
  toggleAllInOne: (identity: string) => void;
};
export const AllInOneVideoGrid = memo(
  ({ hostUserID, allInOneUserID, toggleAllInOne }: AllInOneVideoGridProps) => {
    const [showOther, setShowOther] = useState(true);
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

    const allInOneTrack =
      tracks.find((track) => track.participant.identity === allInOneUserID) ??
      tracks[0];

    return (
      <>
        <div className="flex flex-1 items-center">
          {allInOneTrack && allInOneTrack.participant ? (
            <VideoMemberItem
              className="!m-0 !ml-3 !h-[80%]"
              hostUserID={hostUserID}
              track={allInOneTrack}
            />
          ) : null}
        </div>
        <div
          className={clsx(
            "w-[240px] overflow-y-auto px-3 pl-8 pt-2",
            showOther ? styles["sidebar-visible"] : styles["sidebar-hidden"],
          )}
        >
          {tracks.map((track) => (
            <VideoMemberItem
              className="!mx-0 !my-1.5 !h-[120px]"
              key={track.participant.identity}
              hostUserID={hostUserID}
              track={track}
              toggleAllInOne={toggleAllInOne}
            />
          ))}
        </div>
        <div
          className={styles["expand-col"]}
          style={{ right: showOther ? "210px" : "0" }}
          onClick={() => setShowOther((v) => !v)}
        >
          {showOther ? (
            <RightOutlined className="text-[var(--gap-text)]" />
          ) : (
            <LeftOutlined className="text-[var(--gap-text)]" />
          )}
        </div>
      </>
    );
  },
);
