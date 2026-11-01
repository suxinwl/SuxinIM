type Sender = {
  sendMessage: (params: any, operationID?: string) => Promise<any>;
  sendMessageNotOss: (params: any, operationID?: string) => Promise<any>;
};

export function sendSdkMessage(sdk: Sender, params: any, operationID?: string) {
  // Pinned SDK MessageType.QuoteMessage = 114. Its normal Go send path derefs
  // a missing referenced message in the TARGET conversation (api.go:509).
  // The official NotOss path still performs normal checks/persistence/network
  // delivery, and serializes this already-uploaded quote without that lookup.
  if (params?.message?.contentType === 114) {
    const quote = params.message.quoteElem;
    if (typeof quote?.text !== "string" || !quote.quoteMessage?.clientMsgID || !quote.quoteMessage?.sendID) {
      return Promise.reject(new Error("Quoted message is incomplete"));
    }
    return sdk.sendMessageNotOss(params, operationID);
  }
  return sdk.sendMessage(params, operationID);
}
