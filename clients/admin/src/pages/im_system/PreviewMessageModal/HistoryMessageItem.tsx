import { forwardRef } from 'react';
import { tipTypes } from './utils';
import MessageRender from './MessageItem';
import { MessageItem } from '@openim/wasm-client-sdk';
import { tipMessaggeFormat } from '@/utils/common';

const HistoryMessageItem = (
  {
    message,
    isSelf,
  }: { message: MessageItem; isSelf: boolean; },
  ref: any,
) => {
  if (tipTypes.includes(message.contentType)) {
    return (
      <div ref={ref} className="w-full text-center text-sm text-[#999] py-2 px-10">
        {tipMessaggeFormat(message)}
      </div>
    );
  }

  return (
    <MessageRender isSender={isSelf} message={message} />
  );
};

export default forwardRef(HistoryMessageItem);
