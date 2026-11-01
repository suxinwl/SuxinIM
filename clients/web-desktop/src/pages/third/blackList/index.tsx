import { BlackListContent } from "@/layout/LeftNavBar/BlackList";

export const BlackList = () => {
  const closeWindow = () => {
    window.electronAPI?.ipcInvoke("closeWindow", "black-list");
  };
  return (
    <div className="h-full overflow-hidden">
      <BlackListContent closeOverlay={closeWindow} />
    </div>
  );
};
