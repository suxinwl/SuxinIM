export interface ExType {
  checked?: boolean;
  isAppend?: boolean;
  gapTime?: boolean;
  jump?: boolean;
  errCode?: number;
}

export type ExMessageItem = MessageItem & ExType;
