import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils'
import DailyProgress from './DailyProgress.vue'
import {taskCommentsCreate} from '@/client/generated'
import {readTaskHistory, sharedOutstanding} from '@/helpers/sharedOutstanding'
import {readTeamCommentMarker, serializeTeamCommentMarker} from '@/helpers/tasktraceTeam'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {undoBlockReason, undoInProgress} from '@/helpers/tasktraceUndo'
const draftMocks = vi.hoisted(() => ({write: vi.fn(async () => {}), read: vi.fn(async () => null), remove: vi.fn(async () => {}), check: undefined as undefined | (() => Promise<void>)}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/helpers/autoSave', () => ({autoSaveSettings: {enabled: false}, useAutoSave: (check: () => Promise<void>) => { draftMocks.check = check }}))
vi.mock('@/client/generated', () => ({taskCommentsCreate: vi.fn(), taskCommentsUpdate: vi.fn(), taskAttachmentsUpload: vi.fn()}))
vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn()}))
vi.mock('@/helpers/tasktraceDraftCache', () => ({
	readTaskTraceDraft: draftMocks.read,
	writeTaskTraceDraft: draftMocks.write,
	deleteTaskTraceDraft: draftMocks.remove,
	fileAsDataUrl: vi.fn(),
	dataUrlAsFile: vi.fn(),
}))
vi.mock('@/helpers/sharedOutstanding', () => ({readTaskHistory: vi.fn(async () => []), sharedOutstanding: vi.fn(), changeOutstanding: vi.fn()}))
vi.mock('./AutoSaveSettings.vue', () => ({default: {template: '<span />'}}))
vi.mock('./SharedOutstanding.vue', () => ({default: {template: '<span />'}}))
vi.mock('./ReadonlyRichText.vue', () => ({default: {template: '<span />'}}))
let wrapper: VueWrapper
beforeEach(() => {
	setActivePinia(createPinia())
	const teamStore = useTasktraceTeamStore()
	teamStore.loaded = true
	teamStore.status = {enabled: true, username: 'current', bindings: [], conflicts: [], notifications: []}
})
afterEach(() => { wrapper?.unmount(); localStorage.clear(); undoInProgress.value = false; draftMocks.check = undefined; vi.clearAllMocks() })
describe('daily progress Undo drafts', () => {
	it('refuses a date switch while saving and allows it only after the save finishes', async () => {
		let finishSave!: () => void
		const saveGate = new Promise<void>(resolve => { finishSave = resolve })
		vi.mocked(sharedOutstanding).mockReturnValueOnce({id: 1, items: []})
		vi.mocked(taskCommentsCreate).mockImplementationOnce(async () => {
			await saveGate
			return {data: {id: 99}, request: new Request('http://localhost'), response: new Response()}
		})
		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		const exposed = wrapper.vm as unknown as {switchDate: (date: string) => Promise<boolean>}
		expect(await exposed.switchDate('2026-09-17')).toBe(true)
		await wrapper.get<HTMLTextAreaElement>('textarea').setValue('当前日期待保存的进展')
		await wrapper.get('form').trigger('submit')
		await flushPromises()
		expect(taskCommentsCreate).toHaveBeenCalledTimes(1)
		expect(wrapper.get<HTMLInputElement>('input[type=date]').attributes('disabled')).toBeDefined()
		expect(await exposed.switchDate('2026-09-16')).toBe(false)
		expect(wrapper.get<HTMLInputElement>('input[type=date]').element.value).toBe('2026-09-17')
		expect(wrapper.get<HTMLTextAreaElement>('textarea').element.value).toBe('当前日期待保存的进展')
		expect(wrapper.get('[role=status]').text()).toContain('完成后再切换日期')
		finishSave()
		await flushPromises()
		expect(wrapper.get<HTMLInputElement>('input[type=date]').attributes('disabled')).toBeUndefined()
		expect(await exposed.switchDate('2026-09-16')).toBe(true)
		await flushPromises()
		expect(wrapper.get<HTMLInputElement>('input[type=date]').element.value).toBe('2026-09-16')
		expect(wrapper.get<HTMLTextAreaElement>('textarea').element.value).toBe('')
		expect(taskCommentsCreate).toHaveBeenCalledTimes(1)
	})
	it('lets a collaborator edit another member final daily progress while preserving revisions', async () => {
		const history = [
			{id: 10, created: '2026-09-20T08:00:00Z', comment: '<h3>每日进展 · 2026-09-20</h3><p>alice old</p>' + serializeTeamCommentMarker({id: 'alice-old', author: 'alice'})},
			{id: 11, created: '2026-09-20T09:00:00Z', comment: '<h3>每日进展 · 2026-09-20</h3><p>alice final</p>' + serializeTeamCommentMarker({id: 'alice-final', author: 'alice'})},
		]
		vi.mocked(readTaskHistory).mockResolvedValue(history)
		vi.mocked(sharedOutstanding).mockReturnValue({id: 1, items: []})
		vi.mocked(taskCommentsCreate).mockResolvedValue({data: {id: 99}} as never)
		const teamStore = useTasktraceTeamStore()
		teamStore.status = {
			enabled: true,
			username: 'current',
			bindings: [{root_task_id: 81, task_ids: [], owner: 'current', members: ['alice']}],
			conflicts: [],
			notifications: [],
		}

		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		const exposed = wrapper.vm as unknown as {switchDate: (date: string) => Promise<boolean>}
		expect(await exposed.switchDate('2026-09-20')).toBe(true)
		await wrapper.get<HTMLSelectElement>('#progress-author-81').setValue('alice')
		await flushPromises()
		expect(wrapper.get<HTMLTextAreaElement>('textarea').element.value).toBe('alice final')

		await wrapper.get<HTMLTextAreaElement>('textarea').setValue('alice corrected')
		await wrapper.get('form').trigger('submit')
		await flushPromises()

		const request = vi.mocked(taskCommentsCreate).mock.calls.at(-1)?.[0]
		const body = request?.body?.comment || ''
		expect(body).toContain('alice corrected')
		expect(body).toContain('data-tasktrace-team-merged="alice-old,alice-final"')
		expect(readTeamCommentMarker(body)).toMatchObject({author: 'alice'})
	})

	it('caches changed progress without submitting it and clears the cache after explicit save', async () => {
		vi.mocked(sharedOutstanding).mockReturnValue({id: 1, items: []})
		vi.mocked(taskCommentsCreate).mockResolvedValue({data: {id: 99}} as never)
		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		const date = wrapper.get<HTMLInputElement>('input[type=date]').element.value
		await wrapper.get<HTMLTextAreaElement>('textarea').setValue('discard this draft')
		expect(undoBlockReason.value).toContain('每日进展')
		await draftMocks.check?.()
		await flushPromises()
		expect(draftMocks.write).toHaveBeenCalledWith('progress', 81, date, expect.objectContaining({progress: 'discard this draft'}))
		await draftMocks.check?.()
		expect(draftMocks.write).toHaveBeenCalledTimes(1)
		expect(taskCommentsCreate).not.toHaveBeenCalled()
		expect(wrapper.get('[role=status]').text()).toContain('尚未保存')
		await wrapper.get('form').trigger('submit')
		await flushPromises()
		expect(taskCommentsCreate).toHaveBeenCalledTimes(1)
		expect(draftMocks.remove).toHaveBeenCalledWith('progress', 81, date)
	})
})
