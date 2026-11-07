import { useInViewport } from "ahooks";
import clsx from "clsx";
import { FriendUserItem } from "@/im/types";
import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Virtuoso, VirtuosoHandle } from "react-virtuoso";

import { useContactStore } from "@/store";
import { formatContacts } from "@/utils/common";
import emitter from "@/utils/events";

import AlphabetIndex from "./AlphabetIndex";
import FriendListItem from "./FriendListItem";

export const MyFriends = () => {
  const { t } = useTranslation();
  const friendList = useContactStore((state) => state.friendList);
  const virtuoso = useRef<VirtuosoHandle>(null);
  const alphabetIndex = useRef<{ updateCurrentLetter: (letter: string) => void }>(null);

  const { dataList, indexList } = formatContacts(friendList);

  const scrollToLetter = useCallback((idx: number) => {
    virtuoso.current?.scrollToIndex({
      index: idx,
      behavior: "smooth",
    });
  }, []);

  const updateCurrentLetter = useCallback((letter: string) => {
    alphabetIndex.current?.updateCurrentLetter(letter);
  }, []);

  return (
    <div className="flex h-full w-full flex-col overflow-hidden bg-white">
      <div className="m-5.5 text-base font-extrabold">{t("placeholder.myFriend")}</div>
      <div className="relative ml-4 mt-4 min-h-0 flex-1 pr-4">
        {indexList.length > 1 && (
          <AlphabetIndex
            ref={alphabetIndex}
            indexList={indexList}
            scrollToLetter={scrollToLetter}
          />
        )}
        <Virtuoso
          id="alphabet-wrap"
          className="no-scrollbar h-full overflow-x-hidden"
          data={dataList}
          ref={virtuoso}
          computeItemKey={(idx) => indexList[idx]}
          itemContent={(index, friends) => (
            <LetterSection
              friends={friends}
              index={index}
              indexList={indexList}
              updateCurrentLetter={updateCurrentLetter}
            />
          )}
        />
      </div>
    </div>
  );
};

const LetterSection = ({
  indexList,
  friends,
  index,
  updateCurrentLetter,
}: {
  indexList: string[];
  friends: FriendUserItem[];
  index: number;
  updateCurrentLetter: (letter: string) => void;
}) => {
  const letter = useRef<HTMLDivElement>(null);
  const [_, ratio] = useInViewport(letter, {
    threshold: [0.99, 1],
  });
  const lastLetterRatio = useRef(ratio);

  useEffect(() => {
    if ((letter.current?.getBoundingClientRect().top ?? 999) < 130) {
      updateCurrentLetter(indexList[index]);
    }
    lastLetterRatio.current = ratio;
  }, [ratio]);

  const showUserCard = useCallback((userID: string) => {
    emitter.emit("OPEN_USER_CARD", {
      userID,
    });
  }, []);

  return (
    <div key={indexList[index]}>
      {indexList.length > 1 && (
        <>
          <div
            ref={letter}
            id={`letter${indexList[index]}`}
            className={clsx("my-alphabet px-3.5 pb-1 text-sm text-[#8E9AB0FF]", {
              "pt-4.5": Boolean(index),
            })}
          >
            {indexList[index]}
          </div>
          <div className="mx-3.5 mb-3 h-px w-full bg-[#E8EAEFFF] bg-white" />
        </>
      )}
      {friends.map((friend) => (
        <FriendListItem
          key={friend.userID}
          friend={friend}
          showUserCard={showUserCard}
        />
      ))}
    </div>
  );
};
