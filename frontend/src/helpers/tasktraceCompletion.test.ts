import {describe, expect, it, vi} from 'vitest'
const sdk = vi.hoisted(() => ({taskCommentsList: vi.fn(), taskCommentsUpdate: vi.fn(), taskCommentsCreate: vi.fn(), taskCommentsDelete: vi.fn()}))
vi.mock('@/client/generated', () => sdk)
vi.mock('@/helpers/tasktraceUndo', () => ({undoGroupHeaders: () => ({})}))
import {COMPLETION_OPTIONS, effectiveCompletion, manualCompletion} from './tasktraceCompletion'
import {changeOutstanding, sharedOutstanding} from './sharedOutstanding'
import {conflictDetails} from './teamConflictDiff'

describe('manual completion and done overlay', () => {
	it('defaults to zero, clamps invalid inputs, offers 10% steps, and keeps manual values under Done', () => {
		expect(COMPLETION_OPTIONS).toEqual([0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100])
		for (const value of [undefined, null, NaN, Infinity, 'invalid', -1]) expect(manualCompletion(value)).toBe(0)
		expect(manualCompletion(200)).toBe(100)
		expect(effectiveCompletion(40, true)).toBe(100)
		expect(effectiveCompletion(40, false)).toBe(40)
	})
	it('retains manual completion across saving, refreshing, Done and unrelated note edits', async () => {
		let comment = '<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-progress="40">Content<img src="/api/v1/tasks/1/attachments/3"><aside data-tasktrace-outstanding-note hidden>Memo</aside></li><li data-id="two" data-progress="80">Other</li></ul>'
		sdk.taskCommentsList.mockImplementation(async () => ({data: {items: [{id: 4, comment}], total_pages: 1}}))
		sdk.taskCommentsUpdate.mockImplementation(async options => { comment = options.body.comment; return {data: {}} })
		for (const done of [true, false, true, false]) {
			await changeOutstanding(1, items => items.map(item => item.id === 'one' ? {...item, done, note: 'Edited memo'} : item))
			const saved = sharedOutstanding([{id: 4, comment}]).items
			expect(saved[0].completion).toBe(40)
			expect(effectiveCompletion(saved[0].completion, saved[0].done)).toBe(done ? 100 : 40)
			expect(saved[0].html).toContain('/attachments/3')
			expect(saved[0].note).toBe('Edited memo')
			expect(saved[1].completion).toBe(80)
		}
		await changeOutstanding(1, items => items.map(item => item.id === 'one' ? {...item, completion: 0} : item))
		expect(sharedOutstanding([{id: 4, comment}]).items[0].completion).toBe(0)
	})
	it('shows human-readable manual percentages when resolving task or outstanding conflicts', () => {
		expect(conflictDetails('percent_done', '0.7')[0].text).toBe('70%')
		expect(conflictDetails('outstanding:one', '<span data-tasktrace-outstanding-state hidden data-progress="40" data-done="true"></span>One').find(item => item.key === 'completion')?.text).toBe('40%')
	})
})
