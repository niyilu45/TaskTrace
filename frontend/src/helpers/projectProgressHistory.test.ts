import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createProjectProgressHistory} from './projectProgressHistory'

const mocks = vi.hoisted(() => ({list: vi.fn()}))
vi.mock('@/client/generated', () => ({taskCommentsList: mocks.list}))
beforeEach(() => { mocks.list.mockReset() })

describe('display progress history requests', () => {
	it('shares all pages between a row, outstanding summary and activity filter', async () => {
		mocks.list.mockImplementation(async ({query}: {query: {page: number}}) => ({data: {items: [{id: query.page}], total_pages: 2}}))
		const history = createProjectProgressHistory()
		const [row, summary, filter] = await Promise.all([history.read(11), history.read(11), history.read(11)])
		expect(row.map(comment => comment.id)).toEqual([1, 2])
		expect(summary).toBe(row)
		expect(filter).toBe(row)
		expect(await history.read(11)).toBe(row)
		expect(mocks.list).toHaveBeenCalledTimes(2)
	})

	it('re-reads edited comments on refresh even when their count has not changed', async () => {
		mocks.list.mockResolvedValueOnce({data: {items: [{id: 1, comment: 'Before'}], total_pages: 1}})
			.mockResolvedValueOnce({data: {items: [{id: 1, comment: 'Edited by another member'}], total_pages: 1}})
		const history = createProjectProgressHistory()
		expect((await history.read(11))[0].comment).toBe('Before')
		history.clear()
		expect((await history.read(11))[0].comment).toBe('Edited by another member')
		expect(mocks.list).toHaveBeenCalledTimes(2)
	})

	it('allows retries after a failed read', async () => {
		mocks.list.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({data: {items: [], total_pages: 1}})
		const history = createProjectProgressHistory()
		await expect(history.read(11)).rejects.toThrow('offline')
		await expect(history.read(11)).resolves.toEqual([])
		expect(mocks.list).toHaveBeenCalledTimes(2)
	})

	it('does not let an older failure discard the refreshed request', async () => {
		let rejectOld!: (error: Error) => void
		mocks.list.mockReturnValueOnce(new Promise((_, reject) => { rejectOld = reject }))
			.mockResolvedValueOnce({data: {items: [{id: 2}], total_pages: 1}})
		const history = createProjectProgressHistory()
		const old = history.read(11)
		history.clear()
		const refreshed = await history.read(11)
		rejectOld(new Error('old request failed'))
		await expect(old).rejects.toThrow('old request failed')
		expect(await history.read(11)).toBe(refreshed)
		expect(mocks.list).toHaveBeenCalledTimes(2)
	})
	it('stops stale multi-page reads before queued requests start and after active pages return', async () => {
		const release: ((result: unknown) => void)[] = []
		mocks.list.mockImplementation(() => new Promise(resolve => { release.push(resolve) }))
		const history = createProjectProgressHistory()
		const requests = Array.from({length: 7}, (_, index) => history.read(index + 1))
		const settled = Promise.allSettled(requests)
		expect(mocks.list).toHaveBeenCalledTimes(6)
		history.clear()
		for (const resolve of release) resolve({data: {items: [{id: 1}], total_pages: 5}})
		const outcomes = await settled
		expect(outcomes.every(result => result.status === 'rejected' && result.reason.name === 'AbortError')).toBe(true)
		expect(mocks.list).toHaveBeenCalledTimes(6)
		mocks.list.mockResolvedValueOnce({data: {items: [{id: 2}], total_pages: 1}})
		expect(await history.read(1)).toEqual([{id: 2}])
		expect(mocks.list).toHaveBeenCalledTimes(7)
	})

})
