import {describe, expect, it, vi} from 'vitest'
import {flushPromises, mount} from '@vue/test-utils'
import ReadonlyRichText from './ReadonlyRichText.vue'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'

vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn()}))

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
