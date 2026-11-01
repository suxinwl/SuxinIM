import { AboutContent } from "@/layout/LeftNavBar/About";

export const About = () => {
  const closeWindow = () => {
    window.electronAPI?.ipcInvoke("closeWindow", "about");
  };
  return (
    <div className="h-full bg-[var(--chat-bubble)]">
      <AboutContent closeOverlay={closeWindow} />
    </div>
  );
};
