import { FriendUserItem } from "@/im/types";

import OIMAvatar from "@/components/OIMAvatar";

const FriendListItem = ({
  friend,
  showUserCard,
}: {
  friend: FriendUserItem;
  showUserCard: (userID: string) => void;
}) => {
  return (
    <div
      className="flex items-center rounded-md px-3.5 pb-3 pt-2.5 transition-colors hover:bg-[var(--primary-active)]"
      onClick={() => showUserCard(friend.userID)}
    >
      <OIMAvatar src={friend.faceURL} text={friend.nickname} />
      <div className="ml-3 truncate text-sm">{friend.nickname}</div>
    </div>
  );
};

export default FriendListItem;
