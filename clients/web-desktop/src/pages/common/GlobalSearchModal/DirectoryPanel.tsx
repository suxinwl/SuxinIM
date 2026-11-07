import { Alert, Button, Empty, List, Pagination, Spin } from "antd";
import { DirectoryPerson } from "@/api/directory";
import OIMAvatar from "@/components/OIMAvatar";

export default function DirectoryPanel({ people, loading, error, total, page, searched, summary = false, onPage, onOpen, onMore }: {
  people: DirectoryPerson[]; loading: boolean; error: string; total: number; page: number; searched: boolean;
  summary?: boolean; onPage: (page: number) => void; onOpen: (person: DirectoryPerson) => void; onMore: () => void;
}) {
  if (!searched) return summary ? null : <Empty description="输入手机号、姓名或工号查找企业同事" />;
  return <section className={summary ? "max-h-64 shrink-0 overflow-y-auto border-b p-3" : "h-full overflow-y-auto p-3"}>
    <div className="mb-1 flex items-center justify-between"><strong>企业同事</strong>{summary && total > 3 && <Button type="link" size="small" onClick={onMore}>查看全部 {total} 位</Button>}</div>
    {error ? <Alert type="error" message={error} action={<Button size="small" onClick={() => onPage(page)}>重试</Button>} /> : <Spin spinning={loading}>
      <List dataSource={summary ? people.slice(0, 3) : people} locale={{ emptyText: loading ? "查询中…" : "没有找到匹配手机号、姓名或工号的同事" }}
        renderItem={person => <List.Item key={person.userID}>
          <button type="button" className="flex w-full items-center gap-3 text-left" onClick={() => onOpen(person)}>
            <OIMAvatar src={person.faceURL} text={person.nickname} size={36} />
            <span><span className="block font-medium">{person.nickname}</span><span className="text-xs text-gray-500">工号：{person.employeeNo || "—"} · {person.department || "未分部门"}</span></span>
          </button>
        </List.Item>} />
      {!summary && <Pagination size="small" current={page} total={total} pageSize={20} hideOnSinglePage showSizeChanger={false} onChange={onPage} />}
    </Spin>}
  </section>;
}
