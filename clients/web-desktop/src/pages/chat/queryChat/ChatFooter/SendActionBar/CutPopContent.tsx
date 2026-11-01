import { t } from "i18next";
import { memo } from "react";

const CutPopConent = ({ cutWithoutWindow }: { cutWithoutWindow?: () => void }) => {
  return (
    <div className="p-1">
      <div
        className="cursor-pointer rounded px-2 py-1 text-xs hover:bg-[var(--primary-active)]"
        onClick={cutWithoutWindow}
      >
        {t("placeholder.screenshotHideWindow")}
      </div>
    </div>
  );
};

export default memo(CutPopConent);
