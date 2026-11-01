import React, { ReactNode, useEffect, useRef } from "react";
import twemoji from "twemoji";

interface TwemojiOptions {
  className?: string;
  size?: number;
}

interface TwemojiProps {
  children: ReactNode;
  options?: TwemojiOptions;
  tag?: string;
  [key: string]: unknown;
}

const baseOptions = {
  className: "emojione",
  base: "https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.2/assets/",
};

const Twemoji: React.FC<TwemojiProps> = (props) => {
  const { children, tag, options = {}, ...rest } = props;
  const rootRef = useRef<HTMLDivElement>(null);
  const childrenRefs = useRef<{ [key: string]: React.RefObject<HTMLElement> }>({});

  const noWrapper = !tag;

  const parseTwemoji = () => {
    if (noWrapper) {
      // eslint-disable-next-line guard-for-in
      for (const i in childrenRefs.current) {
        const node = childrenRefs.current[i].current!;
        twemoji.parse(node, { ...baseOptions, ...options });
      }
    } else {
      const node = rootRef.current!;
      twemoji.parse(node, { ...baseOptions, ...options });
    }
  };

  useEffect(() => {
    parseTwemoji();
  }, []);

  if (noWrapper) {
    return (
      <>
        {React.Children.map(children, (child, index) => {
          if (typeof child === "string") {
            console.warn(
              `Twemoji can't parse string child when noWrapper is set. Skipping child "${child}"`,
            );
            return child;
          }
          childrenRefs.current[index] =
            childrenRefs.current[index] || React.createRef<HTMLElement>();
          return React.cloneElement(child as JSX.Element, {
            ref: childrenRefs.current[index],
          });
        })}
      </>
    );
  }

  return React.createElement(tag ?? "div", { ref: rootRef, ...rest }, children);
};

export default Twemoji;

export const parseTwemoji = (unicode: string) => {
  const div = document.createElement("div");
  div.textContent = unicode;
  document.body.appendChild(div);
  twemoji.parse(document.body, baseOptions);
  setTimeout(() => document.body.removeChild(div));
  return div.innerHTML;
};
