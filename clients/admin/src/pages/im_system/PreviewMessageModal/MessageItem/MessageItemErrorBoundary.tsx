import { Component, ReactNode } from "react";

import CatchMessageRender from "./CatchMsgRenderer";
import { MessageItem } from "@openim/wasm-client-sdk";

type MessageItemErrorBoundaryProps = {
  children: ReactNode;
  message: MessageItem;
};

type MessageItemErrorBoundaryState = {
  hasError: boolean;
  message: MessageItem;
};

class MessageItemErrorBoundary extends Component<
  MessageItemErrorBoundaryProps,
  MessageItemErrorBoundaryState
> {
  constructor(props: MessageItemErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, message: props.message };
  }

  componentDidCatch(error: Error) {
    console.log("MessageItemErrorBoundary:::");
    console.info(this.state.message);
    console.error(error);

    this.setState({ hasError: true });
  }

  render() {
    if (this.state.hasError) {
      return <CatchMessageRender />;
    }

    return this.props.children;
  }
}

export default MessageItemErrorBoundary;
