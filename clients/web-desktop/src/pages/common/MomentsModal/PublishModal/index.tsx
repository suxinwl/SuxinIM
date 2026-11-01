import { Modal } from "antd";
import { t } from "i18next";
import React, {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useCallback,
  useEffect,
  useState,
} from "react";

import { OverlayVisibleHandle, useOverlayVisible } from "@/hooks/useOverlayVisible";
import { CheckListItem } from "@/pages/common/ChooseModal/ChooseBox/CheckItem";
import { MomentPermission } from "@/types/moment";
import emitter, { SelectUserParams } from "@/utils/events";

import styles from "./index.module.scss";
import PermissionSelect from "./PermissionSelect";
import PrepareContent from "./PrepareContent";

interface IPublishModalProps {
  activeUserID?: string;
  withVideo?: boolean;
}

const PublishModal: ForwardRefRenderFunction<
  OverlayVisibleHandle,
  IPublishModalProps
> = ({ activeUserID, withVideo = false }, ref) => {
  const [showPrepare, setShowPrepare] = useState(true);
  const [permission, setPermission] = useState(MomentPermission.Public);
  const [remainUserList, setRemainUserList] = useState<CheckListItem[]>([]);
  const [assignUserList, setAssignUserList] = useState<CheckListItem[]>([]);
  const [assignGroupList, setAssignGroupList] = useState<CheckListItem[]>([]);
  const { isOverlayOpen, closeOverlay } = useOverlayVisible(ref);
  const prepareContentRef = React.useRef<{ resetState: () => void }>(null);

  useEffect(() => {
    const handleData = (data: SelectUserParams) => {
      if (data.notConversation) {
        setRemainUserList(data.choosedList);
      }
    };
    emitter.on("SELECT_USER", handleData);
    return () => {
      emitter.off("SELECT_USER", handleData);
    };
  }, []);

  const updatePermisson = useCallback((per: MomentPermission) => {
    setAssignUserList([]);
    setAssignGroupList([]);
    setPermission(per);
  }, []);

  const updateAssignUserList = useCallback(
    (users: CheckListItem[]) => setAssignUserList(users),
    [],
  );
  const updateAssignGroupList = useCallback(
    (users: CheckListItem[]) => setAssignGroupList(users),
    [],
  );
  const updateShowPrepare = useCallback(
    (isPrepare: boolean) => setShowPrepare(isPrepare),
    [],
  );

  const resetState = () => {
    setShowPrepare(true);
    setPermission(MomentPermission.Public);
    setRemainUserList([]);
    setAssignUserList([]);
    setAssignGroupList([]);
    prepareContentRef.current?.resetState();
  };

  return (
    <Modal
      open={isOverlayOpen}
      mask={false}
      closable={false}
      footer={null}
      centered
      width={360}
      afterClose={resetState}
      wrapClassName="!z-[1000]"
      onCancel={closeOverlay}
      className={styles.modal}
    >
      <div className="flex min-h-[480px] flex-col bg-[#F4F5F7]">
        <div className=" flex min-h-[40px] items-center justify-center bg-white">
          {!withVideo ? t("placeholder.publishImages") : t("placeholder.publishImages")}
        </div>
        <PrepareContent
          ref={prepareContentRef}
          visible={showPrepare}
          withVideo={withVideo}
          activeUserID={activeUserID}
          permission={permission}
          assignUserList={assignUserList}
          assignGroupList={assignGroupList}
          remainUserList={remainUserList}
          updateShowPrepare={updateShowPrepare}
          closeOverlay={closeOverlay}
        />
        {!showPrepare && (
          <PermissionSelect
            permission={permission}
            assignUserList={assignUserList}
            assignGroupList={assignGroupList}
            updatePermisson={updatePermisson}
            updateShowPrepare={updateShowPrepare}
            updateAssignUserList={updateAssignUserList}
            updateAssignGroupList={updateAssignGroupList}
          />
        )}
      </div>
    </Modal>
  );
};

export default memo(forwardRef(PublishModal));
