import {describe, it, expect, vi, afterEach} from 'vitest'
import {nextTick} from 'vue'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import Attachments from './Attachments.vue'
import Modal from '@/components/misc/Modal.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import XButton from '@/components/input/Button.vue'
import type {IAttachment} from '@/modelTypes/IAttachment'
import type {ITask} from '@/modelTypes/ITask'

const actions = vi.hoisted(() => ({upload: vi.fn(), remove: vi.fn()}))

vi.mock('@/helpers/attachments', () => ({
	uploadFiles: actions.upload,
	generateAttachmentUrl: vi.fn(),
}))

vi.mock('@/services/attachment', () => ({
	default: class {
		loading = false
		uploadProgress = 0
		delete = actions.remove
	},
}))

vi.mock('@/stores/tasks', () => ({useTaskStore: () => ({isLoading: false})}))

vi.mock('vue-i18n', async importOriginal => ({
	...(await importOriginal<typeof import('vue-i18n')>()),
	useI18n: () => ({t: (key: string) => key}),
}))

vi.mock('@/message', () => ({error: vi.fn(), success: vi.fn()}))

const attachment = {
	id: 1,
	taskId: 1,
	file: {name: 'invoice.pdf', size: 1234, mime: 'application/pdf'},
	created: new Date(),
	createdBy: {id: 1, username: 'demo'},
} as unknown as IAttachment

const task = {id: 1, attachments: [attachment]} as unknown as ITask

const mounted: VueWrapper[] = []
const renderErrors: unknown[] = []

function mountAttachments(attachments = task.attachments) {
	const wrapper = mount(Attachments, {
		attachTo: document.body,
		props: {task: {...task, attachments}},
		global: {
			components: {XButton, BaseButton, Modal},
			stubs: {
				Icon: true,
				User: true,
				FilePreview: true,
				AudioPreview: true,
				ProgressBar: true,
				ImageLightbox: true,
				RouterLink: true,
			},
			directives: {tooltip: {}, cy: {}},
			mocks: {$t: (key: string) => key},
			config: {
				errorHandler: (err: unknown) => renderErrors.push(err),
			},
		},
	})
	mounted.push(wrapper)
	return wrapper
}

afterEach(() => {
	mounted.splice(0).forEach(wrapper => wrapper.unmount())
	renderErrors.length = 0
	vi.clearAllMocks()
	document.body.innerHTML = ''
})

describe('Attachments delete modal', () => {
	it('does not render the attachment name after the modal was closed', async () => {
		const wrapper = mountAttachments()

		await wrapper.find('.attachment-actions [aria-label="task.attachment.deleteTooltip"]').trigger('click')
		await nextTick()

		expect(document.body.innerHTML).toContain('task.attachment.deleteText1')

		// The modal keeps its slot mounted for the duration of the close
		// transition, so it re-renders once the attachment is already gone.
		wrapper.findComponent(Modal).vm.$emit('close')
		await nextTick()

		expect(renderErrors).toEqual([])
		expect(document.querySelector('dialog.modal-dialog')).not.toBeNull()
		expect(document.body.innerHTML).not.toContain('task.attachment.deleteText1')
	})
})

// Filtering the visible list must not change the complete attachment metadata
// emitted to the parent; inline editors still depend on those hidden entries.
describe('Attachments inline media metadata', () => {
	const progress = {...attachment, id: 2, file: {...attachment.file, name: 'tasktrace-progress-image.png'}}
	const legacy = {...attachment, id: 3, file: {...attachment.file, name: 'outstanding-0123456789abcdef0123456789abcdef.png'}}
	const manual = {...attachment, id: 4, file: {...attachment.file, name: 'outstanding-report.png'}}

	it('hides generated inline images and retains them when uploading a visible attachment', async () => {
		const uploaded = {...attachment, id: 5, file: {...attachment.file, name: 'notes.txt'}}
		actions.upload.mockResolvedValue([uploaded])
		const wrapper = mountAttachments([attachment, progress, legacy, manual])
		expect(wrapper.findAll('.filename').map(node => node.text())).toEqual(['invoice.pdf', 'outstanding-report.png'])
		const input = wrapper.get('input[type="file"]')
		Object.defineProperty(input.element, 'files', {value: [new File(['notes'], 'notes.txt')]})
		await input.trigger('change')
		await flushPromises()
		expect(wrapper.emitted('update:attachments')?.[0][0]).toEqual([attachment, progress, legacy, manual, uploaded])
		expect(renderErrors).toEqual([])
	})

	it('retains hidden inline images when deleting a visible attachment', async () => {
		actions.remove.mockResolvedValue({message: 'deleted'})
		const wrapper = mountAttachments([attachment, progress, legacy])
		await wrapper.get('.attachment-actions [aria-label="task.attachment.deleteTooltip"]').trigger('click')
		wrapper.findComponent(Modal).vm.$emit('submit')
		await flushPromises()
		expect(actions.remove).toHaveBeenCalledWith(attachment)
		expect(wrapper.emitted('update:attachments')?.[0][0]).toEqual([progress, legacy])
		expect(renderErrors).toEqual([])
	})
})
