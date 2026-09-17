import {afterEach, describe, expect, it, vi} from 'vitest'
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils'
import DailyProgress from './DailyProgress.vue'
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
afterEach(() => { wrapper?.unmount(); localStorage.clear(); undoInProgress.value = false })
describe('daily progress Undo drafts', () => {
	it('removes a cleared persistent draft so an Undo remount cannot revive and autosave it', async () => {
		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		const date = wrapper.get('input[type=date]').element.value
		const key = `tasktrace-day-draft-81-${date}`
		await wrapper.get('textarea').setValue('discard this draft')
		expect(localStorage.getItem(key)).toContain('discard this draft')
		expect(undoBlockReason.value).toContain('每日进展')
		await wrapper.get('textarea').setValue('')
		expect(localStorage.getItem(key)).toBeNull()
		expect(undoBlockReason.value).toBe('')
		undoInProgress.value = true
		wrapper.unmount()
		wrapper = mount(DailyProgress, {props: {taskId: 81}})
		await flushPromises()
		undoInProgress.value = false
		expect(wrapper.get('textarea').element.value).toBe('')
		expect(undoBlockReason.value).toBe('')
	})
})
