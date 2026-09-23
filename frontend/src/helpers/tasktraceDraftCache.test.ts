import {beforeEach, describe, expect, it, vi} from 'vitest'

const http = vi.hoisted(() => ({get: vi.fn(), put: vi.fn(), delete: vi.fn()}))
vi.mock('@/helpers/fetcher', () => ({
	apiV2Url: (value: string) => value,
	AuthenticatedHTTPFactory: () => http,
}))

import {deleteTaskTraceDraft, readTaskTraceDraft, rememberTaskTraceDraft, writeTaskTraceDraft} from './tasktraceDraftCache'

beforeEach(() => {
	localStorage.clear()
	http.get.mockReset()
	http.put.mockReset()
	http.delete.mockReset()
})

describe('TaskTrace emergency drafts', () => {
	it('restores the latest browser draft when the server was stopped', async () => {
		rememberTaskTraceDraft('progress', 42, '2026-09-24', {progress: 'last input'})
		http.get.mockRejectedValue(new Error('server stopped'))
		await expect(readTaskTraceDraft<{progress: string}>('progress', 42, '2026-09-24')).resolves.toEqual({progress: 'last input'})
	})

	it('remembers a draft before starting the asynchronous cache request', async () => {
		http.put.mockRejectedValue(new Error('process killed'))
		await expect(writeTaskTraceDraft('progress', 42, 'today', {progress: 'recover me'})).rejects.toThrow('process killed')
		http.get.mockRejectedValue(new Error('still offline'))
		await expect(readTaskTraceDraft<{progress: string}>('progress', 42, 'today')).resolves.toEqual({progress: 'recover me'})
	})

	it('clears the emergency copy after an explicit save', async () => {
		rememberTaskTraceDraft('outstanding', 42, 'item', {text: 'saved'})
		http.delete.mockResolvedValue({})
		await deleteTaskTraceDraft('outstanding', 42, 'item')
		http.get.mockResolvedValue({data: {exists: false}})
		await expect(readTaskTraceDraft('outstanding', 42, 'item')).resolves.toBeNull()
	})
})
