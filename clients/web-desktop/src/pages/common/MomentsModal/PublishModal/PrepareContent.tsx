import { BellFilled, EyeFilled, PlusOutlined, RightOutlined } from "@ant-design/icons";
import { Button, Image, Spin, Upload, UploadFile } from "antd";
import TextArea from "antd/es/input/TextArea";
import clsx from "clsx";
import i18n, { t } from "i18next";
import { UploadRequestOption } from "rc-upload/lib/interface";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useImperativeHandle,
  useRef,
  useState,
} from "react";
import { v4 as uuidV4 } from "uuid";

import { usePublishMoments } from "@/api/moments";
import { publishedMomentMedia } from "@/api/moments-media";
import { IMSDK } from "@/layout/MainContentWrap";
import { useFileMessage } from "@/pages/chat/queryChat/ChatFooter/SendActionBar/useFileMessage";
import { CheckListItem } from "@/pages/common/ChooseModal/ChooseBox/CheckItem";
import { MomentContentType, MomentPermission } from "@/types/moment";
import { feedbackToast } from "@/utils/common";
import emitter from "@/utils/events";

import styles from "./index.module.scss";

const momentsPermissionMap = {
  [MomentPermission.Public]: t("placeholder.public"),
  [MomentPermission.Private]: t("placeholder.privacy"),
  [MomentPermission.AssignCanSee]: t("placeholder.partlyVisible"),
  [MomentPermission.AssignCanNotSee]: t("placeholder.hiddenSome"),
};

i18n.on("languageChanged", () => {
  momentsPermissionMap[MomentPermission.Public] = t("placeholder.public");
  momentsPermissionMap[MomentPermission.Private] = t("placeholder.privacy");
  momentsPermissionMap[MomentPermission.AssignCanSee] = t("placeholder.partlyVisible");
  momentsPermissionMap[MomentPermission.AssignCanNotSee] = t("placeholder.hiddenSome");
});

interface IPrepareContentProps {
  visible: boolean;
  withVideo: boolean;
  activeUserID?: string;
  permission: MomentPermission;
  assignUserList: CheckListItem[];
  assignGroupList: CheckListItem[];
  remainUserList: CheckListItem[];
  updateShowPrepare: (isPrepare: boolean) => void;
  closeOverlay: () => void;
}

const PrepareContent: ForwardRefRenderFunction<
  { resetState: () => void },
  IPrepareContentProps
> = (
  {
    visible,
    withVideo,
    activeUserID,
    permission,
    assignUserList,
    assignGroupList,
    remainUserList,
    updateShowPrepare,
    closeOverlay,
  },
  ref,
) => {
  const [text, setText] = useState<string>("");
  const [previewImage, setPreviewImage] = useState("");
  const [uploadFileList, setUploadFileList] = useState<
    (UploadFile & { width?: number; height?: number })[]
  >([]);
  const choosedCount = useRef(0);

  const { mutateAsync: publishMoments, isLoading } = usePublishMoments(activeUserID);

  const { getPicInfo, getVideoSnshotFile } = useFileMessage();

  const confirmPublish = async () => {
    const content = {
      text,
      type: withVideo
        ? MomentContentType.TextWithVideo
        : MomentContentType.TextWithImage,
      metas: uploadFileList.map((task) => ({
        ...publishedMomentMedia(task.url as string, task.width, task.height),
        original: task.response as string,
      })),
    };
    try {
      await publishMoments({
        content,
        permission,
        atUserIDs: remainUserList.map((user) => user.userID) as string[],
        permissionUserIDs: assignUserList.map((user) => user.userID) as string[],
        permissionGroupIDs: assignGroupList.map((group) => group.groupID) as string[],
      });
    } catch (error) {
      feedbackToast({ error, msg: t("toast.publishFailed") });
    }
    closeOverlay();
  };

  const customUpload = async (option: UploadRequestOption) => {
    if (choosedCount.current >= maxCount) return;
    choosedCount.current += 1;
    try {
      const fileData = option.file as File;
      const {
        data: { url },
      } = await IMSDK.uploadFile({
        name: fileData.name,
        contentType: fileData.type,
        uuid: uuidV4(),
        file: fileData,
      });
      let snShotUrl = "";
      let picFile = fileData;
      if (withVideo) {
        picFile = await getVideoSnshotFile(fileData);

        const {
          data: { url },
        } = await IMSDK.uploadFile({
          name: picFile.name,
          contentType: picFile.type,
          uuid: uuidV4(),
          file: picFile,
        });
        snShotUrl = url;
      }
      const { width, height } = await getPicInfo(picFile);

      setUploadFileList((filelist) => [
        ...filelist.filter((item) => item.status === "done"),
        {
          width,
          height,
          uid: uuidV4(),
          name: fileData.name,
          status: "done",
          url: withVideo ? snShotUrl : url,
          response: url,
        },
      ]);
    } catch (error) {
      feedbackToast({ error, msg: t("toast.uploadFailed") });
    }
  };

  const chooseRemainUser = () => {
    emitter.emit("OPEN_CHOOSE_MODAL", {
      type: "SELECT_USER",
      extraData: {
        notConversation: true,
        list: remainUserList,
      },
    });
  };

  const getAssignStr = () => {
    if (
      permission === MomentPermission.AssignCanSee ||
      permission === MomentPermission.AssignCanNotSee
    ) {
      const assignList = [...assignUserList, ...assignGroupList];
      return assignList.length > 0
        ? assignList
            .map((item) => item.nickname || item.groupName || item.showName)
            .join("、")
        : momentsPermissionMap[permission];
    }
    return momentsPermissionMap[permission];
  };

  const handlePreview = (file: UploadFile) => {
    setPreviewImage(file.url!);
  };

  useImperativeHandle(
    ref,
    () => ({
      resetState: () => {
        setText("");
        choosedCount.current = 0;
        setUploadFileList([]);
      },
    }),
    [],
  );

  const getRemainStr =
    remainUserList.length > 0
      ? remainUserList
          .map((item) => item.nickname || item.groupName || item.showName)
          .join("、")
      : "";

  const maxCount = withVideo ? 1 : 9;

  return (
    <div className={clsx(styles["prepare-wrap"], !visible && "!hidden")}>
      <Spin spinning={isLoading}>
        <div className="flex flex-1 flex-col">
          <div className="mt-2 bg-white p-2">
            <TextArea
              showCount
              bordered={false}
              placeholder={t("placeholder.publishMomentsToast")}
              className="mb-2"
              maxLength={150}
              value={text}
              onChange={(e) => setText(e.target.value)}
              autoSize={{ minRows: 4, maxRows: 4 }}
            />
            <Upload
              multiple={!withVideo}
              maxCount={maxCount}
              listType="picture-card"
              fileList={uploadFileList}
              accept={withVideo ? "video/mp4" : "image/*"}
              customRequest={customUpload}
              onChange={({ fileList }) => setUploadFileList(fileList)}
              onPreview={handlePreview}
              onRemove={() => {
                choosedCount.current -= 1;
                return true;
              }}
            >
              {uploadFileList.length < maxCount && (
                <div>
                  <PlusOutlined />
                </div>
              )}
            </Upload>
          </div>
          <div
            className="mt-2 flex cursor-pointer items-center justify-between bg-white px-4 py-4"
            onClick={() => updateShowPrepare(false)}
          >
            <div>
              <EyeFilled />
              <span className="ml-2">{t("placeholder.whoCanWatch")}</span>
            </div>
            <div className="flex max-w-[60%]">
              <div className="flex-1 truncate text-[var(--sub-text)]">
                {getAssignStr()}
              </div>
              <RightOutlined className="text-[var(--sub-text)]" />
            </div>
          </div>
          <div
            className="flex cursor-pointer items-center justify-between bg-white px-4 pt-4"
            onClick={chooseRemainUser}
          >
            <div>
              <BellFilled />
              <span className="ml-2">{t("placeholder.remindWhoToWatch")}</span>
            </div>
            <div className="flex max-w-[60%]">
              <div className="flex-1 truncate text-[var(--sub-text)]">
                {getRemainStr}
              </div>
              <RightOutlined className="text-[var(--sub-text)]" />
            </div>
          </div>
          <div className="bg-white py-2"></div>
          <div className="my-2 flex flex-1 items-end justify-center">
            <Button type="primary" ghost className="m-2 w-full" onClick={closeOverlay}>
              {t("cancel")}
            </Button>
            <Button type="primary" className="m-2 w-full" onClick={confirmPublish}>
              {t("placeholder.publish")}
            </Button>
          </div>
        </div>
      </Spin>
      <Image
        preview={{
          visible: Boolean(previewImage),
          src: previewImage,
          onVisibleChange: (value) => {
            setPreviewImage(value ? previewImage : "");
          },
        }}
      />
    </div>
  );
};

export default memo(forwardRef(PrepareContent));
