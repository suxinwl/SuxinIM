import WindowControlBar from "@/components/WindowControlBar";
import { APP_VERSION } from "@/config";
import LoginForm from "./LoginForm";
import logo from "@/assets/images/puty-im.png";
import { CLIENT_BRAND } from "@/config/brand";
import { useState } from "react";
export type FormType = 0 | 1 | 2;
export const Login = () => {
  const [repairActionsContainer, setRepairActionsContainer] = useState<HTMLDivElement | null>(null);
  return <div className="relative flex h-full min-h-0 flex-col">
    <div className="app-drag h-10 shrink-0"><WindowControlBar/></div>
    <div className="min-h-0 flex-1 overflow-auto">
      <div className="flex min-h-full items-center justify-center gap-8 p-6 lg:gap-16">
        <div className="hidden shrink-0 text-center sm:block"><img src={logo} width={200} alt={CLIENT_BRAND.displayName}/><p>企业统一通讯</p></div>
        <div className="w-[360px] max-w-full shrink-0 rounded-lg bg-white p-8 shadow-lg"><LoginForm repairActionsContainer={repairActionsContainer}/></div>
      </div>
    </div>
    <div className="flex shrink-0 items-center justify-end gap-3 px-3 pb-3 pt-2">
      <div ref={setRepairActionsContainer}/>
      <span className="whitespace-nowrap text-xs text-gray-500">{APP_VERSION}</span>
    </div>
  </div>;
};
