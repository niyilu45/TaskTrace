import {afterEach, describe, expect, it, vi} from 'vitest'
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils'
import DailyProgress from './DailyProgress.vue'
import {taskCommentsCreate} from '@/client/generated'
import {sharedOutstanding} from '@/helpers/sharedOutstanding'
import {undoBlockReason, undoInProgress} from '@/helpers/tasktraceUndo'
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/helpers/autoSave', () => ({useAutoSave: () => {}}))
vi.mock('@/client/generated', () => ({taskCommentsCreate: vi.fn(), taskCommentsUpdate: vi.fn(), taskAttachmentsUpload: vi.fn()}))
vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn()}))
vi.mock('@/helpers/sharedOutstanding', () => ({readTaskHistory: vi.fn(async () => []), sharedOutstanding: vi.fn(), changeOutstanding: vi.fn()}))
vi.mock('./AutoSaveSettings.vue', () => ({default: {template: '<span />'}}))
vi.mock('./SharedOutstanding.vue', () => ({default: {template: '<span />'}}))
vi.mock('./ReadonlyRichText.vue', () => ({default: {template: '<span />'}}))
let wrapper: VueWrapper
afterEach(() => { wrapper?.unmount(); localStorage.clear(); undoInProgress.value = false; vi.clearAllMocks() })
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
	it('removes a cleared persistent draft so an Undo remount cannot revive and autosave it', async () => {
		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		const date = wrapper.get<HTMLInputElement>('input[type=date]').element.value
		const key = `tasktrace-day-draft-81-${date}`
		await wrapper.get<HTMLTextAreaElement>('textarea').setValue('discard this draft')
		expect(localStorage.getItem(key)).toContain('discard this draft')
		expect(undoBlockReason.value).toContain('每日进展')
		await wrapper.get<HTMLTextAreaElement>('textarea').setValue('')
		expect(localStorage.getItem(key)).toBeNull()
		expect(undoBlockReason.value).toBe('')
		undoInProgress.value = true
		wrapper.unmount()
		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		undoInProgress.value = false
		expect(wrapper.get<HTMLTextAreaElement>('textarea').element.value).toBe('')
		expect(undoBlockReason.value).toBe('')
	})
})
