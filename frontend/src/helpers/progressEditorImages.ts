import {taskAttachmentsUpload} from '@/client/generated'
import type {IAttachment} from '@/modelTypes/IAttachment'
import {dataUrlAsFile, fileAsDataUrl} from '@/helpers/tasktraceDraftCache'
import {deduplicateHtmlImages} from '@/helpers/tasktraceImages'

export const PROGRESS_IMAGE_PREFIX = 'tasktrace-progress-'

function extensionFor(file: File) {
	const fromName = file.name.match(/(\.[a-z0-9]{1,8})$/i)?.[1]
	if (fromName) return fromName.toLowerCase()
	const subtype = file.type.split('/')[1]?.replace(/[^a-z0-9]/gi, '') || 'png'
	return `.${subtype === 'jpeg' ? 'jpg' : subtype}`
}

export function isProgressImageAttachment(attachment: Pick<IAttachment, 'file'>) {
	return attachment.file.name.toLowerCase().startsWith(PROGRESS_IMAGE_PREFIX)
}

export async function stageProgressImages(files: File[] | FileList) {
	return Promise.all(Array.from(files).map(fileAsDataUrl))
}

export function countProgressImages(html: string) {
	return new DOMParser().parseFromString(html || '', 'text/html').querySelectorAll('img').length
}

function normalizedAttachmentSource(source: string) {
	if (/^\/api\/v[12]\/tasks\/\d+\/attachments\/\d+$/.test(source)) return source.replace(/^\/api\/v2/, '/api/v1')
	try {
		const parsed = new URL(source, window.location.origin)
		const api = new URL(window.API_URL, window.location.origin)
		if (parsed.origin !== api.origin) return ''
		const match = parsed.pathname.match(/^\/api\/v[12]\/tasks\/\d+\/attachments\/\d+$/)
		return match ? match[0].replace(/^\/api\/v2/, '/api/v1') : ''
	} catch {
		return ''
	}
}

export async function persistProgressImages(html: string, taskId: number) {
	const doc = new DOMParser().parseFromString(html || '', 'text/html')
	for (const image of Array.from(doc.querySelectorAll('img'))) {
		const source = image.getAttribute('data-src') || image.getAttribute('src') || ''
		if (source.startsWith('data:image/')) {
			const staged = await dataUrlAsFile(source, 'progress-image', source.slice(5, source.indexOf(';')))
			const named = new File([staged], `${PROGRESS_IMAGE_PREFIX}${globalThis.crypto?.randomUUID?.() || Date.now().toString(36)}${extensionFor(staged)}`, {type: staged.type})
			const result = await taskAttachmentsUpload({path: {task: taskId}, body: {files: [named]}})
			const id = result.data.success?.[0]?.id
			if (!id || result.data.errors?.length) throw new Error('Progress image upload failed')
			image.setAttribute('src', `/api/v1/tasks/${taskId}/attachments/${id}`)
		} else if (normalizedAttachmentSource(source)) {
			image.setAttribute('src', normalizedAttachmentSource(source))
		} else {
			image.remove()
			continue
		}
		image.removeAttribute('data-src')
		image.removeAttribute('id')
	}
	return deduplicateHtmlImages(doc.body.innerHTML)
}
