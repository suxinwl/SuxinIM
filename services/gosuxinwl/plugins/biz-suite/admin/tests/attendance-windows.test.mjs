import test from "node:test";
import assert from "node:assert/strict";
import { windowClock, windowSecond, inspectWindows } from "../assets/attendance-windows.js";

test("second-accurate windows include displayed final second without sharing the midday boundary", () => {
  assert.equal(windowSecond(0,"12:39:59",true),45600);
  assert.equal(windowSecond(0,"12:40:00"),45600);
  const result=inspectWindows([
    {name:"上午",punch_mode:"out",local_windows:{out_start_second:43200,out_end_second:45600}},
    {name:"下午",punch_mode:"in",local_windows:{in_start_second:45600,in_end_second:48601}},
  ]);
  assert.deepEqual(result.issues,[]);
  assert.equal(result.rows[0].end,result.rows[1].start);
  assert.deepEqual(windowClock(45600,true),{day:0,time:"12:39:59"});
});
test("boundaries are configurable, preserve midnight and negative/next-day offsets", () => {
  assert.equal(windowSecond(0,"12:20:00"),44400);
  assert.equal(windowSecond(0,"00:00:00"),0);
  assert.equal(windowSecond(-1,"23:59:59"),-1);
  assert.equal(windowSecond(1,"08:00:00",true),115201);
  for(const value of [-86400,-1,0,45600,86400,115201]) assert.equal(windowSecond(windowClock(value).day,windowClock(value).time),value);
  assert.equal(windowSecond(0,""),null);
  assert.equal(windowSecond(0,"25:00:00"),null);
  assert.equal(windowClock(null).time,"");
});
test("missing required evidence remains unconfigured while exempt directions need no window", () => {
  assert.match(inspectWindows([{name:"晚班",punch_mode:"both",local_windows:{in_start_second:null,in_end_second:0}}]).issues.join("；"),/待配置/);
  assert.deepEqual(inspectWindows([{name:"免卡",punch_mode:"none"},{name:"休息",kind:"break"},{name:"仅上班",punch_mode:"in",local_windows:{in_start_second:0,in_end_second:1}}]).issues,[]);
});
test("overlaps and reversed bounds are visible instead of silently assigning a clock",()=>{
  assert.match(inspectWindows([{name:"上午",punch_mode:"out",local_windows:{out_start_second:43200,out_end_second:45601}},{name:"下午",punch_mode:"in",local_windows:{in_start_second:45600,in_end_second:48601}}]).issues.join("；"),/窗口重叠/);
  assert.match(inspectWindows([{name:"夜班",punch_mode:"out",local_windows:{out_start_second:86400,out_end_second:72000}}]).issues.join("；"),/不能早于/);
});
