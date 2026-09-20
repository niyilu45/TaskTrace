import {beforeEach, describe, expect, it, vi} from 'vitest'
import {taskAttachmentsUpload} from '@/client/generated'
import {countProgressImages, isProgressImageAttachment, persistProgressImages, PROGRESS_IMAGE_PREFIX} from './progressEditorImages'

vi.mock('@/client/generated', () => ({taskAttachmentsUpload: vi.fn()}))
vi.mock('@/helpers/tasktraceDraftCache', () => ({
	fileAsDataUrl: vi.fn(),
	dataUrlAsFile: vi.fn(async () => new File([new Uint8Array([1, 2, 3])], 'clipboard.png', {type: 'image/png'})),
}))

beforeEach(() => vi.clearAllMocks())

describe('progress editor images', () => {
	it('counts inline images and keeps existing progress images in their text position', async () => {
		const html = '<p>before</p><img data-src="/api/v1/tasks/8/attachments/4" src="#"><p>after</p>'
		expect(countProgressImages(html)).toBe(1)
		expect(await persistProgressImages(html, 8)).toBe('<p>before</p><img src="/api/v1/tasks/8/attachments/4"><p>after</p>')
		expect(taskAttachmentsUpload).not.toHaveBeenCalled()
	})

	it('uploads staged data images with the progress-only filename prefix', async () => {
		vi.mocked(taskAttachmentsUpload).mockResolvedValue({data: {success: [{id: 44}], errors: []}} as never)
		const html = await persistProgressImages('<p>text</p><img src="data:image/png;base64,AQID"><p>end</p>', 8)
		expect(html).toBe('<p>text</p><img src="/api/v1/tasks/8/attachments/44"><p>end</p>')
		const file = vi.mocked(taskAttachmentsUpload).mock.calls[0][0].body.files[0]
		expect(file.name.startsWith(PROGRESS_IMAGE_PREFIX)).toBe(true)
		expect(isProgressImageAttachment({file: {name: file.name}} as never)).toBe(true)
		expect(isProgressImageAttachment({file: {name: 'report.png'}} as never)).toBe(false)
	})
})
