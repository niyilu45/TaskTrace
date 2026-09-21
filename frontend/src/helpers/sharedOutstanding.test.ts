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

	it('parses completion metadata and numbers only unfinished items in display html', () => {
		const shared = {id: 2, comment: '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="a" data-done="false" data-priority="4">one</li><li data-id="b" data-done="true" data-priority="2" data-completed-at="2026-09-18T08:00:00.000Z">two</li></ul>'}
		const result = sharedOutstanding([shared]).items
		expect(result).toEqual([
			{id: 'a', html: 'one', done: false, priority: 4, completedAt: undefined, reminderAt: undefined},
			{id: 'b', html: 'two', done: true, priority: 2, completedAt: '2026-09-18T08:00:00.000Z', reminderAt: undefined},
		])
		expect(outstandingHtml([shared])).toBe('<ol><li>one</li></ol>')
	})

	it('imports the latest legacy list as individual unfinished items', () => {
		expect(sharedOutstanding([{id: 1, comment: '<h3>每日进展 · 2026-09-20</h3><p>x</p><p><strong>遗留问题 / 下一步</strong></p><p>one<br>two</p>'}]).items.map(item => item.html)).toEqual(['one', 'two'])
	})

	it('keeps rich notes separate from list content', () => {
		const shared = {id: 3, comment: '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="a"><p>事项正文</p><aside data-tasktrace-outstanding-note="true" hidden><p>内部备注</p><ul><li>备注列表</li></ul><p><img src="/api/v1/tasks/42/attachments/8"></p></aside></li></ul>'}
		const parsed = sharedOutstanding([shared]).items
		expect(parsed).toHaveLength(1)
		const item = parsed[0]
		expect(item.html).toBe('<p>事项正文</p>')
		expect(item.note).toContain('内部备注')
		expect(item.note).toContain('attachments/8')
		expect(outstandingHtml([shared])).toBe('<ol><li><p>事项正文</p></li></ol>')
	})
})
