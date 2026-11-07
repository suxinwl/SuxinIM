import { ApartmentOutlined, ReloadOutlined } from "@ant-design/icons";
import { Alert, Button, Empty, Input, Pagination, Spin, Table, Tree } from "antd";
import { useEffect, useRef, useState } from "react";
import { departmentTree, DirectoryDepartment, DirectoryPerson, getDirectory } from "@/api/directory";
import OIMAvatar from "@/components/OIMAvatar";
import emitter from "@/utils/events";
import styles from "./organization.module.scss";

export function Organization() {
  const [departments, setDepartments] = useState<DirectoryDepartment[]>([]);
  const [items, setItems] = useState<DirectoryPerson[]>([]);
  const [departmentID, setDepartmentID] = useState("");
  const [draft, setDraft] = useState("");
  const [keyword, setKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const attempt = ++generation.current;
    setLoading(true); setError(""); setItems([]);
    getDirectory({ departmentID, keyword, page }).then(result => {
      if (attempt !== generation.current) return;
      setDepartments(result.departments ?? []); setItems(result.items ?? []); setTotal(result.total ?? 0);
    }).catch(() => { if (attempt === generation.current) setError("组织通讯录暂不可用，请重试"); })
      .finally(() => { if (attempt === generation.current) setLoading(false); });
    return () => { generation.current++; };
  }, [departmentID, keyword, page, revision]);
  const openPerson = (person: DirectoryPerson) => emitter.emit("OPEN_USER_CARD", { userID: person.userID });
  const title = keyword ? `搜索“${keyword}”` : departments.find(d => String(d.id) === departmentID)?.name ?? "全部同事";
  const leader = departments.find(d => String(d.id) === departmentID)?.leaderName;
  return <div className={styles.organization}>
    <aside className={styles.departments}>
      <h2><ApartmentOutlined /> 组织架构</h2>
      <Tree blockNode defaultExpandAll key={departments.map(d => d.id).join(",")} selectedKeys={[departmentID]}
        treeData={[{ key: "", title: "全部同事", children: departmentTree(departments) }]}
        onSelect={keys => { if (keys[0] === undefined) return; setDepartmentID(String(keys[0])); setDraft(""); setKeyword(""); setPage(1); }} />
    </aside>
    <section className={styles.people}>
      <div className={styles.toolbar}>
        <h2>{title}</h2>
        <Input.Search aria-label="按手机号、姓名或工号查找同事" placeholder="搜索员工手机号、姓名或工号" value={draft} allowClear
          onChange={event => { setDraft(event.target.value); if (!event.target.value) { setKeyword(""); setPage(1); } }}
          onSearch={value => { setKeyword(value.trim()); setDepartmentID(""); setPage(1); }} />
        <Button icon={<ReloadOutlined />} aria-label="刷新组织架构" onClick={() => setRevision(v => v + 1)} />
      </div>
      <p className={styles.hint}>点击同事查看资料，可直接发消息或添加好友。</p>
      {leader && <p className={styles.hint}>部门负责人：{leader}</p>}
      {error ? <Alert type="error" message={error} action={<Button onClick={() => setRevision(v => v + 1)}>重试</Button>} /> :
        <Spin spinning={loading}><Table<DirectoryPerson> rowKey="userID" dataSource={items} pagination={false}
          locale={{ emptyText: <Empty description={keyword ? "没有找到匹配手机号、姓名或工号的同事" : "该部门暂无可显示的同事"} /> }}
          onRow={person => ({ onClick: () => openPerson(person), className: styles.person })}
          columns={[
            { title: "姓名", key: "nickname", render: (_, person) => <button type="button" className={styles.name} onClick={event => { event.stopPropagation(); openPerson(person); }}><OIMAvatar size={36} src={person.faceURL} text={person.nickname} /><span>{person.nickname}</span></button> },
            { title: "工号", dataIndex: "employeeNo", render: value => value || "—" },
            { title: "部门", dataIndex: "department", render: value => value || "—" },
            { title: "岗位", dataIndex: "position", render: value => value || "—" },
          ]} />
          <Pagination className={styles.pagination} current={page} pageSize={50} total={total} hideOnSinglePage showSizeChanger={false} onChange={setPage} />
        </Spin>}
    </section>
  </div>;
}
