export function attachmentImageKey(source: string) {
	const match = source.match(/\/api\/v[12]\/tasks\/(\d+)\/attachments\/(\d+)/i)
	return match ? `${match[1]}:${match[2]}` : source
}

export function deduplicateHtmlImages(html: string) {
	const doc = new DOMParser().parseFromString(html || '', 'text/html')
	const seen = new Set<string>()
	for (const image of doc.body.querySelectorAll('img')) {
		const source = image.getAttribute('data-tasktrace-src') || image.getAttribute('data-src') || image.getAttribute('src') || ''
		const key = attachmentImageKey(source)
		if (!source || seen.has(key)) image.remove()
		else seen.add(key)
	}
	return doc.body.innerHTML
}
