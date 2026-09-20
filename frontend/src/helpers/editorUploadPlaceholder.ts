const LEGACY_UPLOAD_PLACEHOLDER = 'UPLOAD_PLACEHOLDER'

export function stripEditorUploadPlaceholders(html: string) {
	const doc = new DOMParser().parseFromString(html || '', 'text/html')
	for (const paragraph of Array.from(doc.body.querySelectorAll('p'))) {
		if (!paragraph.querySelector('img') && paragraph.textContent?.trim() === LEGACY_UPLOAD_PLACEHOLDER) paragraph.remove()
	}
	return doc.body.innerHTML
}
