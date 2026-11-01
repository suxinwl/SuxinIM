import { Button, DatePicker, Form, Input, Select } from "antd";
import { RangePickerProps } from "antd/es/date-picker";
import dayjs from "dayjs";
import { t } from "i18next";
import { memo } from "react";

import { IMSDK } from "@/layout/MainContentWrap";
import { useUserStore } from "@/store";

import { UpdateMeetingParams } from "../MeetingModal/data";
import {
  LaunchMeetingFormFields,
  LaunchStep,
  MeetingAuthData,
  NewMeetingInfo,
} from "./data";

type OrderOrJoinMeetingProps = {
  isLaunch?: boolean;
  meetingInfo?: NewMeetingInfo;
  joinLoading?: boolean;
  joinMeeting?: (roomID: string) => void;
  updateStep?: (step: LaunchStep, extraData?: unknown) => void;
};
export const OrderOrJoinMeeting = memo(
  ({
    isLaunch,
    meetingInfo,
    joinLoading,
    joinMeeting,
    updateStep,
  }: OrderOrJoinMeetingProps) => {
    const [form] = Form.useForm<LaunchMeetingFormFields>();
    const selfInfo = useUserStore((state) => state.selfInfo);

    const onFinish = (values: LaunchMeetingFormFields) => {
      if (meetingInfo) {
        const options = {} as UpdateMeetingParams;
        options.startTime = values.startTime.unix();
        options.endTime = values.startTime.unix() + values.duration;
        options.meetingName = values.meetingName;
        IMSDK.signalingUpdateMeetingInfo({
          ...options,
          roomID: meetingInfo.roomID,
        })
          .then(() => updateStep?.(LaunchStep.Display, { ...meetingInfo, ...values }))
          .catch((err) => {
            console.error(err);
          });
        return;
      }

      if (isLaunch) {
        IMSDK.signalingCreateMeeting<MeetingAuthData>({
          meetingName: values.meetingName,
          meetingHostUserID: selfInfo.userID,
          startTime: values.startTime.unix(),
          meetingDuration: values.duration,
          inviteeUserIDList: [],
        })
          .then(({ data }) => {
            IMSDK.signalingUpdateMeetingInfo({
              roomID: data.roomID,
              participantCanUnmuteSelf: true,
            });
            updateStep?.(LaunchStep.Display, {
              ...data,
              ...values,
              hostUserID: selfInfo.userID,
            });
          })
          .catch((err) => {
            console.error(err);
          });
      } else {
        joinMeeting?.(values.meetingNo);
      }
    };

    const disabledDate: RangePickerProps["disabledDate"] = (current) => {
      // Can not select days before today
      return current && current < dayjs().subtract(1, "d").endOf("day");
    };
    const disabledTime: RangePickerProps["disabledTime"] = (current) => {
      // If the date is today, disables the selection of values before the current time
      if (current && current.isSame(dayjs(), "day")) {
        return {
          disabledHours: () => [...Array(dayjs(meetingInfo?.startTime).hour()).keys()],
          disabledMinutes: () => [
            ...Array(dayjs(meetingInfo?.startTime).minute()).keys(),
          ],
          disabledSeconds: () => [
            ...Array(dayjs(meetingInfo?.startTime).second()).keys(),
          ],
        };
      }

      // There are no restrictions on the time of other dates
      return {};
    };

    return (
      <div className="p-4">
        <Form
          className="meeting-form w-full"
          form={form}
          initialValues={meetingInfo}
          layout="vertical"
          size="small"
          autoComplete="off"
          onFinish={onFinish}
        >
          {isLaunch ? (
            <>
              <Form.Item
                name="meetingName"
                label={t("placeholder.meetingName")}
                rules={[{ required: true, message: t("toast.inputMeetingName") }]}
              >
                <Input
                  maxLength={20}
                  placeholder={t("toast.inputMeetingName")}
                  spellCheck={false}
                />
              </Form.Item>
              <Form.Item
                name="startTime"
                label={t("time.startTime")}
                rules={[
                  {
                    required: true,
                    message: t("toast.selectMeetingSatrtTime"),
                  },
                  {
                    message: t("toast.selectMeetingTimeFailed"),
                    // eslint-disable-next-line
                    validator: async (_, value: dayjs.Dayjs) => {
                      if (
                        value
                          .clone()
                          .add(2, "m")
                          .isBefore(dayjs(meetingInfo?.startTime))
                      ) {
                        throw new Error(t("toast.selectMeetingTimeFailed"));
                      }
                    },
                  },
                ]}
              >
                <DatePicker
                  disabledDate={disabledDate}
                  disabledTime={disabledTime as any}
                  showTime={{ format: "HH:mm" }}
                  placeholder={t("toast.selectMeetingSatrtTime")}
                />
              </Form.Item>
              <Form.Item
                name="duration"
                label={t("placeholder.meetingTime")}
                rules={[{ required: true, message: t("toast.selectMeetingTime") }]}
              >
                <Select placeholder={t("toast.selectMeetingTime")}>
                  <Select.Option value={1800}>
                    {t("date.hour", { num: 0.5 })}
                  </Select.Option>
                  <Select.Option value={3600}>
                    {t("date.hour", { num: 0.1 })}
                  </Select.Option>
                  <Select.Option value={5400}>
                    {t("date.hour", { num: 1.5 })}
                  </Select.Option>
                  <Select.Option value={7200}>
                    {t("date.hour", { num: 2 })}
                  </Select.Option>
                </Select>
              </Form.Item>
            </>
          ) : (
            <Form.Item
              name="meetingNo"
              label={t("placeholder.meetingID")}
              rules={[{ required: true, message: t("toast.inputmeetingID") }]}
            >
              <Input placeholder={t("toast.inputmeetingID")} />
            </Form.Item>
          )}
        </Form>

        <div className="mb-14 mt-20 w-full text-center">
          <Button
            className="ignore-drag w-[80%] rounded"
            type="primary"
            onClick={form.submit}
            loading={joinLoading}
          >
            {meetingInfo
              ? t("confirm")
              : isLaunch
              ? t("placeholder.orderMeeting")
              : t("placeholder.joinMeeting")}
          </Button>
        </div>
      </div>
    );
  },
);
