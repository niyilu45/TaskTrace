
import {describe, expect, it} from 'vitest'
import {defaultRepeatRule, firstRepeatDate, parseRepeatRule, repeatRuleError, serializeRepeatRule, repeatRuleText} from './reminderRepeat'
import TaskReminderModel from '@/models/taskReminder'
describe('weekly reminders', () => {
 const rule = {weekdays: [1, 3, 5], startDate: '2026-10-12', endDate: '2026-10-23', time: '09:30'}
 it('round-trips API rules, selected weekdays and inclusive range', () => {
  const raw = serializeRepeatRule(rule)
  expect(parseRepeatRule(raw)).toEqual(rule)
  expect(firstRepeatDate(rule)?.getDate()).toBe(12)
  expect(firstRepeatDate({...rule, startDate: '2026-10-23'})?.getDate()).toBe(23)
  expect(repeatRuleText(raw)).toContain('周一、周三、周五')
  const model = new TaskReminderModel({repeat_rule: raw} as never)
  expect(model.repeatRule).toBe(raw)
  expect(defaultRepeatRule(new Date(2026, 9, 15)).weekdays).toEqual([4])
 })
 it('restores selected weekday overrides and uses the fallback for other days', () => {
  const schedule = {...rule, times: {1: '08:15', 5: '17:45'}}
  const raw = serializeRepeatRule(schedule)
  expect(parseRepeatRule(raw)).toEqual(schedule)
  expect(firstRepeatDate(schedule)).toEqual(new Date(2026, 9, 12, 8, 15))
  expect(firstRepeatDate({...schedule, startDate: '2026-10-13'})).toEqual(new Date(2026, 9, 14, 9, 30))
  expect(repeatRuleText(raw)).toContain('周一 08:15、周三 09:30、周五 17:45')
  expect(serializeRepeatRule({...rule, times: {1: rule.time}})).toBe(serializeRepeatRule(rule))
 })
 it('rejects invalid weekday overrides', () => {
  for (const times of [{1: '25:00'}, {0: '12:00'}, {7: '12:00'}, ['09:30']]) {
   expect(parseRepeatRule(JSON.stringify({...rule, times}))).toBeNull()
  }
 })
 it('rejects empty weekdays, inverted or invalid ranges, invalid time and ranges without a match', () => {
  for (const change of [{weekdays: []}, {weekdays: [7]}, {endDate: '2026-10-01'}, {startDate: '2026-02-30'}, {time: '25:00'}, {startDate: '2026-10-13', endDate: '2026-10-13'}]) {
   expect(repeatRuleError({...rule, ...change})).not.toBe('')
  }
  expect(parseRepeatRule('broken')).toBeNull()
 })
})
