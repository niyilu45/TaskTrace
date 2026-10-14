import {beforeEach, describe, expect, it, vi} from 'vitest'
import {defineComponent, h} from 'vue'
import {flushPromises, mount} from '@vue/test-utils'
import ReadonlyRichText from './ReadonlyRichText.vue'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'

vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn()}))
beforeEach(() => {
	vi.clearAllMocks()
	window.API_URL = window.location.origin + '/api/v2'
})

describe('ReadonlyRichText progress references', () => {
	it('renders saved reference snapshots behind an expand and collapse control', async () => {
		const html = '<p>本日更正</p><blockquote data-tasktrace-reference="1" data-reference-id="r0123456789abcdef0123456789abcdef" data-task-id="12" data-date="2026-09-17" data-comment-ids="55"><p><strong>引用 2026-09-17 的进展</strong> <a href="/tasks/12#comment-55">查看原记录</a></p><div data-tasktrace-reference-content="1"><p>历史内容</p></div></blockquote>'
		const wrapper = mount(ReadonlyRichText, {props: {html}})
		await flushPromises()
		const details = wrapper.get('details.progress-reference-details')
		expect(details.attributes('open')).toBeUndefined()
		expect(details.get('summary').text()).toContain('展开引用的历史进展（2026-09-17）')
		expect(details.text()).toContain('历史内容')
		expect(details.get('a').attributes('target')).toBe('_blank')
	})

	it('renders the same protected attachment only once', async () => {
		vi.mocked(fetchAttachmentBlobUrl).mockResolvedValue('blob:attachment-8')
		const html = '<p><img src="/api/v1/tasks/42/attachments/8"></p><p><img src="/api/v2/tasks/42/attachments/8"></p>'
		const wrapper = mount(ReadonlyRichText, {props: {html}})
		await flushPromises()
		expect(fetchAttachmentBlobUrl).toHaveBeenCalledTimes(1)
		expect(wrapper.findAll('img')).toHaveLength(1)
		expect(wrapper.get('img').attributes('src')).toBe('blob:attachment-8')
	})
})

describe('image previews inside editable task rows', () => {
	function renderRow(html: string) {
		const edit = vi.fn(), editOnDoubleClick = vi.fn()
		const row = mount(defineComponent({
			setup: () => () => h('div', {onClick: edit, onDblclick: editOnDoubleClick}, h(ReadonlyRichText, {html})),
		}), {global: {stubs: {ImageLightbox: {props: ['blobUrl'], template: '<div class="preview">{{ blobUrl }}</div>'}}}})
		return {row, edit, editOnDoubleClick}
	}

	it('opens the image without triggering either parent edit action', async () => {
		vi.mocked(fetchAttachmentBlobUrl).mockResolvedValue('blob:outstanding-image')
		const {row, edit, editOnDoubleClick} = renderRow('<p>Outstanding text</p><img src="/api/v2/tasks/42/attachments/8">')
		await flushPromises()
		await row.get('img').trigger('click')
		await row.get('img').trigger('click', {detail: 2})
		await row.get('img').trigger('dblclick')
		expect(row.get('.preview').text()).toBe('blob:outstanding-image')
		expect(edit).not.toHaveBeenCalled()
		expect(editOnDoubleClick).not.toHaveBeenCalled()
		row.unmount()
	})

	it('also blocks edit and navigation when the picture is inside a link', async () => {
		vi.mocked(fetchAttachmentBlobUrl).mockResolvedValue('blob:outstanding-image')
		const {row, edit} = renderRow('<a href="/tasks/42"><img src="/api/v2/tasks/42/attachments/8"></a>')
		await flushPromises()
		const event = new MouseEvent('click', {bubbles: true, cancelable: true})
		row.get('img').element.dispatchEvent(event)
		await flushPromises()
		expect(event.defaultPrevented).toBe(true)
		expect(edit).not.toHaveBeenCalled()
		expect(row.get('.preview').text()).toBe('blob:outstanding-image')
		row.unmount()
	})

	it('does not turn a failed image into an edit action', async () => {
		vi.mocked(fetchAttachmentBlobUrl).mockRejectedValue(new Error('offline'))
		const {row, edit, editOnDoubleClick} = renderRow('<p><img src="/api/v2/tasks/42/attachments/8"></p>')
		await flushPromises()
		await row.get('img').trigger('click')
		await row.get('img').trigger('dblclick')
		expect(edit).not.toHaveBeenCalled()
		expect(editOnDoubleClick).not.toHaveBeenCalled()
		expect(row.find('.preview').exists()).toBe(false)
		row.unmount()
	})

	it('keeps the normal edit action when clicking the item text', async () => {
		const {row, edit} = renderRow('<p>Outstanding text</p>')
		await flushPromises()
		await row.get('.readonly-rich-text').trigger('click')
		expect(edit).toHaveBeenCalledOnce()
		row.unmount()
	})
})
