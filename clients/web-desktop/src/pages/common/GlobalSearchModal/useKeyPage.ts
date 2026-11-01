import { useKeyPress } from "ahooks";
import { useEffect, useState } from "react";

export function useKeyPage({
  isActive,
  elPrefix,
  maxIndex,
  callback,
}: {
  isActive: boolean;
  elPrefix: string;
  maxIndex: number;
  callback?: (idx: number) => void;
}) {
  const [activeIdx, setActiveIdx] = useState({
    idx: 0,
    needScroll: true,
  });

  useEffect(() => {
    if (!activeIdx.needScroll) return;
    const el = document.querySelector(`${elPrefix}${activeIdx.idx}`);
    el?.scrollIntoView({
      block: "end",
    });
  }, [activeIdx.idx]);

  useKeyPress("uparrow", () => {
    if (!isActive || activeIdx.idx === 0) return;
    setActiveIdx((value) => ({ idx: value.idx - 1, needScroll: true }));
  });

  useKeyPress("downarrow", () => {
    if (!isActive || activeIdx.idx >= maxIndex - 1) return;
    setActiveIdx((value) => ({ idx: value.idx + 1, needScroll: true }));
  });

  useKeyPress("enter", () => {
    if (!isActive) return;
    callback?.(activeIdx.idx);
  });

  return {
    activeIdx: activeIdx.idx,
    updateIdx: (idx: number) => setActiveIdx({ idx, needScroll: false }),
  };
}
