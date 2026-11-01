import dayjs from "dayjs";

export type NewMeetingInfo = {
  meetingName: string;
  startTime: dayjs.Dayjs;
  duration: number;
  roomID: string;
  token: string;
  liveURL: string;
  hostUserID: string;
  onlyHostInviteUser?: boolean;
};

export type MeetingRecordItem = {
  createTime: number;
  endTime: number;
  hostUserID: string;
  joinDisableVideo: boolean;
  roomID: string;
  meetingName: string;
  onlyHostInviteUser: boolean;
  participantCanEnableVideo: boolean;
  startTime: number;
  hostFaceURL?: string;
  hostNickname?: string;
};

export type MeetingAuthData = {
  liveURL: string;
  token: string;
  roomID: string;
};

export type LaunchMeetingFormFields = {
  meetingName: string;
  startTime: dayjs.Dayjs;
  duration: number;
  meetingNo: string;
};

export enum LaunchStep {
  Prepare,
  Order,
  Display,
  HistoryDisplay,
  Update,
  Join,
}

export const displayTypes = [LaunchStep.Display, LaunchStep.HistoryDisplay];
export const notBackTypes = [LaunchStep.Prepare, LaunchStep.Update];
