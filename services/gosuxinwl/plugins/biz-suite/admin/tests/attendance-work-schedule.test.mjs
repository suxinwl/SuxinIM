import test from "node:test";
import assert from "node:assert/strict";
import { fixedScheduleMode, initialWorkSchedule, workScheduleCycle } from "../assets/attendance-work-schedule.js";

test("production employees have five required weekdays and optional weekend overtime templates", () => {
  const schedule = initialWorkSchedule({ category:"员工", shifts:[{id:"normal",overtime_variant:{shift:{id:"late"}}},{id:"late"}] });
  assert.deepEqual(schedule.weekday_shift_ids,["normal"]);
  assert.deepEqual(schedule.restday_shift_ids,["normal"]);
  const cycle = workScheduleCycle(schedule);
  assert.equal(cycle.weekdays.length,7);
  assert.deepEqual(cycle.weekdays.map(day=>day.scheduled_work),[true,true,true,true,true,false,false]);
  assert.deepEqual(cycle.weekdays.map(day=>day.category),["workday","workday","workday","workday","workday","restday","restday"]);
  assert.deepEqual(cycle.weekdays[6].candidate_shift_ids,["normal"]);
});

test("production staff have six workdays and never guess a missing Saturday shift",()=>{
  const schedule=initialWorkSchedule({category:"生产职员",shifts:[{id:"normal"}]});
  assert.deepEqual(schedule.saturday_shift_ids,[]);
  const cycle=workScheduleCycle(schedule);
  assert.equal(cycle.weekdays[5].category,"workday");
  assert.equal(cycle.weekdays[5].scheduled_work,true);
  assert.equal(cycle.weekdays[5].shift_id,"");
  assert.equal(cycle.weekdays[6].scheduled_work,false);
  const explicit=initialWorkSchedule({category:"生产职员",shifts:[{id:"normal"},{id:"saturday",name:"周六正班"}]});
  assert.deepEqual(explicit.weekday_shift_ids,["normal"]);
  assert.deepEqual(explicit.saturday_shift_ids,["saturday"]);
});

test("explicit policies preserve nulls, unavailable IDs and distinct Saturday/rest-day choices for validation",()=>{
  const config={category:"生产职员",work_schedule:{mode:"after_sales",weekday_shift_ids:["base","duty"],saturday_shift_ids:["sat-base","sat-duty"],restday_shift_ids:["sun"],duty_shift_ids:["duty","sat-duty"],duty_group:"售后留守组"}};
  assert.deepEqual(initialWorkSchedule(config),config.work_schedule);
  const cycle=workScheduleCycle(config.work_schedule);
  assert.deepEqual(cycle.weekdays[5].candidate_shift_ids,["sat-base","sat-duty"]);
  assert.deepEqual(cycle.weekdays[6].candidate_shift_ids,["sun"]);
  assert.deepEqual(initialWorkSchedule({category:"员工",work_schedule:{mode:"production_employee",weekday_shift_ids:null,saturday_shift_ids:null,restday_shift_ids:null}}).weekday_shift_ids,[]);
});

test("ordinary staff keep explicit advanced cycles without classification-based rewriting",()=>{
  assert.equal(fixedScheduleMode("职员"),"");
  assert.equal(initialWorkSchedule({category:"职员",cycle:{mode:"cycle",days:Array(14).fill("day")}}),null);
  assert.equal(fixedScheduleMode("员工","after_sales"),"production_employee");
});
