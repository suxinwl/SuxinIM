import { getSSOSession } from "@/utils/storage";
import { ssoRequest, SSOError } from "./sso";

export interface DirectoryPerson {
  userID: string;
  nickname: string;
  employeeNo: string;
  department: string;
  departmentID: string;
  position: string;
  faceURL: string;
  canDirectChat?: boolean;
}
export interface DirectoryDepartment { id: string; parentID: string; name: string; groupEnabled?: boolean; leaderUserID?: string; leaderName?: string }
export interface DirectoryPage {
  items: DirectoryPerson[];
  departments: DirectoryDepartment[];
  total: number;
}
export interface DepartmentNode { key: string; title: string; children: DepartmentNode[] }

// HRM is the authority for both directory visibility and employee eligibility.
// Never infer membership or permission from an emp_ prefix or a cached SDK user.
async function directoryRequest<T>(operation: "directory" | "person", body: object): Promise<T> {
  const session = await getSSOSession();
  if (!session?.sessionToken) throw new SSOError("请使用统一账号重新登录", 401);
  const result = await ssoRequest<T>(operation, { ...body, sessionToken: session.sessionToken });
  const current = await getSSOSession();
  if (current?.sessionToken !== session.sessionToken || current?.userID !== session.userID) {
    throw new SSOError("账号会话已改变，请重试", 409);
  }
  return result;
}

export function getDirectory({ keyword = "", departmentID = "", page = 1, pageSize = 50 } = {}) {
  return directoryRequest<DirectoryPage>("directory", {
    keyword: keyword.trim(), departmentID, page: Math.max(1, Math.floor(page)),
    pageSize: Math.max(1, Math.min(100, Math.floor(pageSize))),
  });
}

export async function getDirectoryPerson(userID: string) {
  const person = await directoryRequest<DirectoryPerson>("person", { userID });
  if (person.userID !== userID) throw new SSOError("员工资料不匹配，请重试", 503);
  return person;
}

export function departmentTree(departments: DirectoryDepartment[]): DepartmentNode[] {
  const byID = new Map(departments.filter(d => d.id && d.name).map(d => [String(d.id), d]));
  const seen = new Set<string>();
  const visit = (id: string): DepartmentNode | undefined => {
    if (seen.has(id)) return;
    seen.add(id);
    const department = byID.get(id)!;
    const children = [...byID.values()].filter(d => String(d.parentID) === id)
      .map(d => visit(String(d.id))).filter((d): d is DepartmentNode => Boolean(d));
    return { key: id, title: department.name, children };
  };
  const roots = [...byID.values()].filter(d => !byID.has(String(d.parentID)) || String(d.parentID) === String(d.id))
    .map(d => visit(String(d.id))).filter((d): d is DepartmentNode => Boolean(d));
  // A malformed cycle cannot hang the UI or hide otherwise permitted nodes.
  for (const id of byID.keys()) { const node = visit(id); if (node) roots.push(node); }
  return roots;
}
