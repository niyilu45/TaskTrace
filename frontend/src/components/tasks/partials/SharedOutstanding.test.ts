import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import {taskAttachmentsUpload, taskCommentsCreate, taskCommentsList, taskCommentsUpdate} from '@/client/generated'
import SharedOutstanding from './SharedOutstanding.vue'

vi.mock('@/client/generated', () => ({
	taskAttachmentsUpload: vi.fn(),
	taskCommentsCreate: vi.fn(),
	taskCommentsList: vi.fn(),
	taskCommentsUpdate: vi.fn(),
}))
vi.mock('@/helpers/autoSave', () => ({autoSaveSettings: {enabled: false}, useAutoSave: () => {}}))
vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn(async ({id}: {id: number}) => `blob:attachment-${id}`)}))
vi.mock('@/helpers/tasktraceDraftCache', () => ({
	readTaskTraceDraft: vi.fn(async () => null),
	writeTaskTraceDraft: vi.fn(async () => {}),
	deleteTaskTraceDraft: vi.fn(async () => {}),
	fileAsDataUrl: vi.fn(),
	dataUrlAsFile: vi.fn(),
}))
vi.mock('@/components/input/AsyncEditor', () => ({default: {
	props: ['modelValue'],
	emits: ['update:modelValue', 'save'],
	template: '<textarea class="mock-note-editor" :value="modelValue" @input="$emit(`update:modelValue`, $event.target.value)" @keydown.ctrl.enter.prevent="$emit(`save`)" />',
}}))
vi.mock('./ReadonlyRichText.vue', () => ({default: {props: ['html'], template: '<div class="rendered-outstanding" v-html="html" />'}}))

let wrapper: VueWrapper
let history: {id: number, comment: string}[]
let attachmentId: number
let root: HTMLDivElement
const upload = vi.mocked(taskAttachmentsUpload)
const update = vi.mocked(taskCommentsUpdate)
const create = vi.mocked(taskCommentsCreate)
const list = vi.mocked(taskCommentsList)
const shared = (items: string) => `<h3>TaskTrace 遗留事项清单</h3><ul>${items}</ul>`

async function open(disabled = false) {
	wrapper = mount(SharedOutstanding, {props: {taskId: 42, disabled}, attachTo: root})
	await flushPromises()
}

async function click(label: string) {
	const button = wrapper.findAll('button').find(button => button.text() === label)
	expect(button, `Missing button: ${label}`).toBeDefined()
	await button!.trigger('click')
	await flushPromises()
}

async function paste(...names: string[]) {
	const event = new Event('paste', {bubbles: true, cancelable: true})
	Object.defineProperty(event, 'clipboardData', {value: {items: names.map(name => ({kind: 'file', type: 'image/png', getAsFile: () => new File(['png'], name, {type: 'image/png'})}))}})
	wrapper.get('textarea').element.dispatchEvent(event)
	await flushPromises()
	return event
}

beforeEach(() => {
	vi.clearAllMocks()
	history = []
	attachmentId = 0
	root = document.createElement('div')
	document.body.appendChild(root)
	vi.stubGlobal('URL', class extends URL {
		static createObjectURL(file: File) { return `blob:${file.name}` }
		static revokeObjectURL = vi.fn()
	})
	list.mockImplementation(async () => ({data: {items: structuredClone(history), total_pages: 1}}) as never)
	create.mockImplementation(async ({body}) => {
		const note = {id: 1, comment: body.comment!}
		history.push(note)
		return {data: note} as never
	})
	update.mockImplementation(async ({body, path}) => {
		const note = history.find(note => note.id === path.commentid)!
		note.comment = body.comment!
		return {data: note} as never
	})
	upload.mockImplementation(async () => ({data: {success: [{id: ++attachmentId}]}}) as never)
})

afterEach(() => {
	wrapper?.unmount()
	root.remove()
	vi.useRealTimers()
	vi.unstubAllGlobals()
})

describe('outstanding item images', () => {
	it('announces busy synchronously throughout image upload and releases it after saving', async () => {
		let finishUpload!: (result: never) => void
		upload.mockImplementationOnce(() => new Promise<never>(resolve => { finishUpload = resolve }))
		await open()
		await paste('pending.png')
		await click('添加遗留事项')
		expect(wrapper.emitted('busy')).toEqual([[true]])
		expect(create).not.toHaveBeenCalled()
		finishUpload({data: {success: [{id: 5}]}} as never)
		await flushPromises()
		expect(wrapper.emitted('busy')).toEqual([[true], [false]])
		expect(create).toHaveBeenCalledTimes(1)
	})

	it('clears busy when the component is unmounted during an upload', async () => {
		let finishUpload!: (result: never) => void
		upload.mockImplementationOnce(() => new Promise<never>(resolve => { finishUpload = resolve }))
		await open()
		await paste('pending.png')
		await click('添加遗留事项')
		expect(wrapper.emitted('busy')).toEqual([[true]])
		wrapper.unmount()
		expect(wrapper.emitted('busy')?.slice(-1)[0]).toEqual([false])
		finishUpload({data: {success: [{id: 5}]}} as never)
		await flushPromises()
	})

	it('saves an image-only item and consumes paste before the daily progress form', async () => {
		const parentPaste = vi.fn()
		root.addEventListener('paste', parentPaste)
		await open()
		const event = await paste('screenshot.png')
		expect(event.defaultPrevented).toBe(true)
		expect(parentPaste).not.toHaveBeenCalled()
		expect(wrapper.findAll('.outstanding-images img')).toHaveLength(1)
		await click('添加遗留事项')
		expect(upload).toHaveBeenCalledTimes(1)
		expect(history[0].comment).toContain('src="/api/v1/tasks/42/attachments/1"')
		await vi.waitFor(() => expect(wrapper.findAll('ol.outstanding-list > li')).toHaveLength(1))
		expect(wrapper.findAll('.outstanding-images img')).toHaveLength(0)
		expect(wrapper.emitted('saved')).toHaveLength(1)
	})

	it('retains a failed draft and retries only images which were not uploaded', async () => {
		await open()
		await wrapper.get('textarea').setValue('Keep this note')
		await paste('one.png', 'two.png')
		upload.mockResolvedValueOnce({data: {success: [{id: 11}]}} as never).mockRejectedValueOnce(new Error('Network unavailable')).mockResolvedValueOnce({data: {success: [{id: 12}]}} as never)
		await click('添加遗留事项')
		expect(wrapper.get('textarea').element.value).toBe('Keep this note')
		expect(wrapper.findAll('.outstanding-images img')).toHaveLength(2)
		expect(wrapper.get('[role="alert"]').text()).toContain('输入和图片已保留')
		expect(create).not.toHaveBeenCalled()
		await click('添加遗留事项')
		expect(upload).toHaveBeenCalledTimes(3)
		expect(history[0].comment).toContain('Keep this note')
		expect(history[0].comment).toContain('attachments/11')
		expect(history[0].comment).toContain('attachments/12')
		expect(wrapper.get('textarea').element.value).toBe('')
	})

	it('edits text and images without losing other items or duplicating an uncertain save', async () => {
		history = [{id: 1, comment: shared('<li data-id="first"><p>Original</p><p><img src="/api/v1/tasks/42/attachments/8" alt="旧图"></p></li>')}]
		await open()
		await click('编辑')
		expect(wrapper.get('textarea').element.value).toBe('Original')
		expect(wrapper.get('.outstanding-images img').attributes('src')).toBe('blob:attachment-8')
		await wrapper.get('textarea').setValue('Image caption')
		await paste('detail.png')
		history[0].comment = shared('<li data-id="first"><p>Original</p><p><img src="/api/v1/tasks/42/attachments/8" alt="旧图"></p></li><li data-id="second">Concurrent item</li>')
		update.mockImplementationOnce(async ({body}) => {
			history[0].comment = body.comment!
			throw new Error('Response lost after server save')
		})
		await click('保存修改')
		expect(wrapper.findAll('.outstanding-images img')).toHaveLength(2)
		await click('保存修改')
		expect(upload).toHaveBeenCalledTimes(1)
		expect(history[0].comment).toContain('Concurrent item')
		expect(history[0].comment.match(/Image caption/g)).toHaveLength(1)
		expect(history[0].comment.match(/<img /g)).toHaveLength(2)
	})

	it('preserves separate drafts when switching between new and existing items', async () => {
		history = [{id: 1, comment: shared('<li data-id="first">Existing</li>')}]
		await open()
		await wrapper.get('textarea').setValue('New outstanding draft')
		await paste('new.png')
		await click('编辑')
		expect(wrapper.get('textarea').element.value).toBe('Existing')
		await paste('existing.png')
		await click('返回新增事项')
		expect(wrapper.get('textarea').element.value).toBe('New outstanding draft')
		expect(wrapper.get('.outstanding-images img').attributes('src')).toBe('blob:new.png')
		await click('编辑')
		expect(wrapper.get('.outstanding-images img').attributes('src')).toBe('blob:existing.png')
	})

	it('edits a private rich note without showing it in the outstanding list', async () => {
		history = [{id: 1, comment: shared('<li data-id="first"><p>Visible item</p><aside data-tasktrace-outstanding-note="true" hidden><p>Existing note</p></aside></li>')}]
		await open()
		expect(wrapper.get('.rendered-outstanding').text()).toBe('Visible item')
		expect(wrapper.text()).not.toContain('Existing note')
		await click('编辑')
		const note = wrapper.get('.mock-note-editor')
		expect((note.element as HTMLTextAreaElement).value).toContain('Existing note')
		await note.setValue('<p>Updated note</p><p><img src="/api/v1/tasks/42/attachments/9"></p>')
		await click('保存修改')
		expect(history[0].comment).toContain('data-tasktrace-outstanding-note="true"')
		expect(history[0].comment).toContain('Updated note')
		expect(history[0].comment).toContain('attachments/9')
		expect(wrapper.get('.rendered-outstanding').text()).toBe('Visible item')
		expect(wrapper.text()).not.toContain('Updated note')
	})

	it('does not re-create an item that was moved or removed while its images were being added', async () => {
		history = [{id: 1, comment: shared('<li data-id="first">Existing</li>')}]
		await open()
		await click('编辑')
		await paste('unsaved.png')
		history[0].comment = shared('')
		await click('保存修改')
		expect(update).not.toHaveBeenCalled()
		expect(wrapper.get('[role="alert"]').text()).toContain('已被移除或移动')
		expect(wrapper.findAll('.outstanding-images img')).toHaveLength(1)
		await click('重新读取')
		expect(wrapper.text()).not.toContain('第 0 条')
		await click('另存为新遗留事项')
		await click('添加遗留事项')
		expect(upload).toHaveBeenCalledTimes(1)
		expect(history[0].comment).toContain('attachments/1')
	})

	it('does not leak a disabled image paste to the parent form', async () => {
		const parentPaste = vi.fn()
		root.addEventListener('paste', parentPaste)
		await open(true)
		await paste('disabled.png')
		expect(parentPaste).not.toHaveBeenCalled()
		expect(wrapper.findAll('.outstanding-images img')).toHaveLength(0)
		expect(upload).not.toHaveBeenCalled()
	})

	it('hides completed items until requested and keeps their original numbers', async () => {
		history = [{id: 1, comment: shared('<li data-id="pending" data-done="false">Pending</li><li data-id="done" data-done="true" data-completed-at="2026-09-18T08:00:00.000Z">Completed</li>')}]
		await open()
		expect(wrapper.text()).toContain('Pending')
		expect(wrapper.text()).not.toContain('Completed')
		expect(wrapper.get('ol.outstanding-list > li').attributes('value')).toBe('1')
		await click('显示已完成的遗留事项（1）')
		expect(wrapper.text()).toContain('Completed')
		const completed = wrapper.get('ol.completed-list > li')
		expect(completed.attributes('value')).toBe('2')
		expect((completed.get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true)
	})

	it('moves a checked item into completed items after five seconds and allows restoring it', async () => {
		vi.useFakeTimers()
		history = [{id: 1, comment: shared('<li data-id="pending" data-done="false">Pending</li>')}]
		await open()
		await wrapper.get('input[aria-label="完成第 1 条遗留事项"]').setValue(true)
		await flushPromises()
		expect(wrapper.text()).toContain('Pending')
		expect(history[0].comment).toContain('data-done="true"')
		vi.advanceTimersByTime(4999)
		await wrapper.vm.$nextTick()
		expect(wrapper.text()).toContain('Pending')
		vi.advanceTimersByTime(1)
		await wrapper.vm.$nextTick()
		expect(wrapper.text()).not.toContain('Pending')
		await click('显示已完成的遗留事项（1）')
		await wrapper.get('input[aria-label="恢复第 1 条遗留事项"]').setValue(false)
		await flushPromises()
		expect(wrapper.text()).toContain('Pending')
		expect(history[0].comment).toContain('data-done="false"')
		expect(history[0].comment).not.toContain('data-completed-at')
	})
})
