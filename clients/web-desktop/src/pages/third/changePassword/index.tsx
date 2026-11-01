import { ChangePasswordContent } from "@/layout/LeftNavBar/ChangePassword";

export const ChangePassword = () => {
  const closeWindow = () => {
    window.electronAPI?.ipcInvoke("closeWindow", "change-password");
  };
  return (
    <div className="h-full border-none">
      <ChangePasswordContent closeOverlay={closeWindow} />
    </div>
  );
};
