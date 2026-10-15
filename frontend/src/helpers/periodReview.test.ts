import {describe, it, expect} from 'vitest'
import {reviewDefaults, reviewRange} from './periodReview'
describe('period review dates', () => {
 it('defaults to seven inclusive calendar days across a month boundary', () => {
  const now = new Date(2026, 9, 3, 18), defaults = reviewDefaults(now)
  expect(defaults).toEqual({mode: 'days', days: 7, from: '2026-09-27', to: '2026-10-03'})
  const range = reviewRange(defaults, now)
  expect(range.from).toMatch(/^2026-09-27T00:00:00[+-]/)
  expect(range.until).toMatch(/^2026-10-04T00:00:00[+-]/)
 })
 it('includes both ends and supports a single day', () => {
  const value = {...reviewDefaults(), mode: 'dates' as const, from: '2026-10-10', to: '2026-10-10'}
  expect(reviewRange(value).until).toMatch(/^2026-10-11T/)
 })
 it('rejects invalid dates, reversed bounds and invalid counts', () => {
  for (const days of [0,-1,1.5,NaN,36501]) expect(() => reviewRange({...reviewDefaults(),days})).toThrow()
  for (const dates of [{from:'2026-02-30',to:'2026-03-03'},{from:'2026-10-10',to:'2026-10-09'}]) expect(() => reviewRange({...reviewDefaults(),mode:'dates',...dates})).toThrow()
 })
})
