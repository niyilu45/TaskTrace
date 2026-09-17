import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils'
import Comments from './Comments.vue'
import {undoBlockReason, undoInProgress} from '@/helpers/tasktraceUndo'
const update = vi.hoisted(() => vi.fn(async (data: object) => data))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('vue-i18n', () => ({useI18n: () => ({t: (key: string) => key})}))
vi.mock('@/components/input/AsyncEditor', () => ({default: {props: ['modelValue'], emits: ['update:modelValue'], template: '<textarea :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'}}))
vi.mock('./DailyProgress.vue', () => ({default: {template: '<span />'}}))
vi.mock('@/components/misc/CustomTransition.vue', () => ({default: {template: '<span><slot /></span>'}}))
vi.mock('@/components/input/Reactions.vue', () => ({default: {template: '<span />'}}))
vi.mock('@/components/misc/UserAvatar.vue', () => ({default: {template: '<span />'}}))
vi.mock('@/components/misc/PaginationEmit.vue', () => ({default: {template: '<span />'}}))
vi.mock('@/services/taskComment', () => ({default: class {loading = false; totalPages = 1; resultCount = 1; update = update}}))
vi.mock('@/models/taskComment', () => ({default: class {constructor(data: object = {}) {Object.assign(this, data)}}}))
vi.mock('@/stores/config', () => ({useConfigStore: () => ({taskCommentsEnabled: true, maxItemsPerPage: 100, frontendUrl: 'http://localhost/'})}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({info: {id: 1}, settings: {frontendSettings: {commentSortOrder: 'asc'}}})}))
vi.mock('@/helpers/attachments', () => ({uploadFile: vi.fn(), uploadFilesForEditor: vi.fn()}))
vi.mock('@/message', () => ({success: vi.fn()}))
vi.mock('@/helpers/time/formatDate', () => ({formatDateLong: () => '', formatDisplayDate: () => ''}))
vi.mock('@/models/user', () => ({getDisplayName: () => 'tester'}))
vi.mock('@/composables/useCopyToClipboard', () => ({useCopyToClipboard: () => vi.fn()}))
let wrapper: VueWrapper
beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); undoInProgress.value = false })
afterEach(() => { wrapper?.unmount(); vi.useRealTimers(); undoInProgress.value = false })
function open() {
	wrapper = mount(Comments, {props: {taskId: 1, projectId: 1, initialComments: [{id: 2, comment: 'old', author: {id: 1}, created: new Date(), updated: new Date()}] as never}, global: {mocks: {$t: (key: string) => key}, directives: {tooltip: () => {}}, stubs: {Icon: true, Modal: true, XButton: true}}})
}
describe('comment Undo protection', () => {
	it('guards new and delayed existing comment drafts, and flushes them on normal navigation', async () => {
		open()
		await wrapper.findAll('textarea')[1].setValue('new draft')
		expect(undoBlockReason.value).toContain('评论草稿')
		await wrapper.findAll('textarea')[1].setValue('')
		expect(undoBlockReason.value).toBe('')
		await wrapper.findAll('textarea')[0].setValue('edited')
		expect(undoBlockReason.value).toContain('评论草稿')
		wrapper.unmount()
		await flushPromises()
		expect(update).toHaveBeenCalledOnce()
		expect(update).toHaveBeenCalledWith(expect.objectContaining({id: 2, taskId: 1, comment: 'edited'}))
	})
	it('cancels the delayed write when an Undo remount disposes the editor', async () => {
		open()
		await wrapper.findAll('textarea')[0].setValue('stale')
		undoInProgress.value = true
		wrapper.unmount()
		undoInProgress.value = false
		await vi.advanceTimersByTimeAsync(6000)
		expect(update).not.toHaveBeenCalled()
		expect(undoBlockReason.value).toBe('')
	})
})
