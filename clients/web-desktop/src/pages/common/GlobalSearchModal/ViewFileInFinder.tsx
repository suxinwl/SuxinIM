import { Popover, PopoverProps } from "antd";
import { t } from "i18next";
import { memo, useState } from "react";

const ViewFileInFinder = ({
  viewInFinder,
  children,
}: {
  viewInFinder?: () => void;
  children: JSX.Element;
}) => {
  const [visibleState, setVisibleState] = useState(false);

  const viewInFinderVithClose = () => {
    viewInFinder?.();
    setVisibleState(false);
  };

  const popProps: PopoverProps = {
    placement: "bottomRight",
    content: <MenuContent viewInFinder={viewInFinderVithClose} />,
    title: null,
    arrow: false,
    trigger: "contextMenu",
    open: window.electronAPI ? visibleState : false,
    onOpenChange: (visible) => setVisibleState(visible),
  };

  return <Popover {...popProps}>{children}</Popover>;
};

const MenuContent = ({ viewInFinder }: { viewInFinder?: () => void }) => {
  return (
    <div className="">
      <div
        className="cursor-pointer rounded px-4 py-2 text-xs hover:bg-[var(--primary-active)]"
        onClick={viewInFinder}
      >
        {t("placeholder.finder")}
      </div>
    </div>
  );
};

export default memo(ViewFileInFinder);
