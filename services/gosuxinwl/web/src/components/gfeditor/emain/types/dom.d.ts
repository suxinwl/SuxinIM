import type { NodeInterface, RangePath, RangeInterface, TinyCanvasInterface } from './index';
declare global {
  interface Node {
    is_mark?: boolean;
    __canvas?: TinyCanvasInterface;
    __range?: RangeInterface;
    __target?: NodeInterface | { start: RangePath; end: RangePath };
    __node?: Node;
    __card_root?: NodeInterface;
  }
  interface KeyboardEvent { isDelete?: boolean; }
  interface Event { rangeParent?: Node; rangeOffset?: number; }
  interface MouseEvent { toElement?: EventTarget | null; }
  interface HTMLMediaElement { controlsList: DOMTokenList; }
}
export {};
