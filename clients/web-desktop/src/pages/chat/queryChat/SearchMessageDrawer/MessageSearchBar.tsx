import { SearchOutlined } from "@ant-design/icons";
import { useDebounceFn } from "ahooks";
import { Input } from "antd";
import { t } from "i18next";
import {
  forwardRef,
  ForwardRefRenderFunction,
  memo,
  useImperativeHandle,
  useState,
} from "react";

const MessageSearchBar: ForwardRefRenderFunction<
  { clear: () => void },
  { triggerSearch: (keyword: string) => void }
> = ({ triggerSearch }, ref) => {
  const [keyword, setKeyword] = useState("");
  const { run: debounceSearch } = useDebounceFn(() => triggerSearch(keyword), {
    wait: 500,
  });

  useImperativeHandle(ref, () => ({ clear: () => setKeyword("") }), []);

  return (
    <div className="px-5.5">
      <Input
        value={keyword}
        allowClear
        spellCheck={false}
        onChange={(e) => {
          setKeyword(e.target.value);
          debounceSearch();
        }}
        placeholder={t("placeholder.search")!}
        prefix={<SearchOutlined rev={undefined} />}
      />
    </div>
  );
};

export default memo(forwardRef(MessageSearchBar));
