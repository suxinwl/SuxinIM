import test from "node:test";
import assert from "node:assert/strict";
import { exceptionErrorText, exceptionCountText } from "../assets/wecom.js";

test("known approval errors explain validation broadly and preserve unknown errors", () => {
  assert.equal(exceptionErrorText("normalized approval failed HRM validation or persistence"), "审批起止、申请时长或业务字段校验未通过；原文已保留，请核对");
  assert.match(exceptionErrorText("approval template is not enabled or mapped"), /未配置或未启用/);
  assert.equal(exceptionErrorText("future unknown error: 999"), "future unknown error: 999");
  assert.match(exceptionErrorText("unsupported checkin_type; raw record retained", { checkin_type: "" }), /未返回打卡类型/);
});
test("exception count distinguishes the truncated list from the full total", () => {
  assert.equal(exceptionCountText({ approvals: Array(100).fill({}), totals: { approvals: 655 } }, "approvals"), "仅显示前 100 条 / 共 655 条；其余记录仍保留在系统中");
  assert.equal(exceptionCountText({ checkins: [{}, {}, {}], totals: { checkins: 3 } }, "checkins"), "显示 3 条 / 共 3 条");
  assert.equal(exceptionCountText({ contacts: [] }, "contacts"), "当前显示 0 条");
});
