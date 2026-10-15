
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
 it('rejects empty weekdays, inverted or invalid ranges, invalid time and ranges without a match', () => {
  for (const change of [{weekdays: []}, {weekdays: [7]}, {endDate: '2026-10-01'}, {startDate: '2026-02-30'}, {time: '25:00'}, {startDate: '2026-10-13', endDate: '2026-10-13'}]) {
   expect(repeatRuleError({...rule, ...change})).not.toBe('')
  }
  expect(parseRepeatRule('broken')).toBeNull()
 })
})
