import {describe, it, expect, vi} from 'vitest'
vi.mock('@/client/generated', () => ({}))
import {sharedOutstanding, outstandingHtml} from './sharedOutstanding'
describe('shared outstanding', () => {
 it('uses the shared list regardless of progress dates and retains empty lists', () => {
  const old = {id: 1, comment: '<h3>每日进展 · 2026-09-20</h3><p>x</p><p><strong>遗留问题 / 下一步</strong></p><p>legacy</p>'}
  const shared = {id: 2, comment: '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="a">one</li><li data-id="b">two</li></ul>'}
  expect(sharedOutstanding([old, shared]).items.map(item => item.id)).toEqual(['a', 'b'])
  expect(outstandingHtml([old, shared])).not.toContain('legacy')
  expect(sharedOutstanding([old, {...shared, comment: '<h3>TaskTrace 遗留事项清单</h3><ul></ul>'}]).items).toEqual([])
 })
 it('imports the latest legacy list as individual items', () => {
  expect(sharedOutstanding([{id: 1, comment: '<h3>每日进展 · 2026-09-20</h3><p>x</p><p><strong>遗留问题 / 下一步</strong></p><p>one<br>two</p>'}]).items.map(item => item.html)).toEqual(['one', 'two'])
 })
})
