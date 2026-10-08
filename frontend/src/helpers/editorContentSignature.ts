import {attachmentImageKey, deduplicateHtmlImages} from './tasktraceImages'

const transientAttributes = new Set([
	'class',
	'contenteditable',
	'data-node-view-content',
	'data-node-view-wrapper',
	'data-placeholder',
	'draggable',
	'spellcheck',
	'style',
])

function stableImageSource(image: HTMLImageElement) {
	const source = image.getAttribute('data-tasktrace-src') || image.getAttribute('data-src') || image.getAttribute('src') || ''
	return attachmentImageKey(source)
}

export function editorContentSignature(value: string) {
	const doc = new DOMParser().parseFromString(deduplicateHtmlImages(value || ''), 'text/html')
	for (const image of doc.body.querySelectorAll('img')) {
		const source = stableImageSource(image)
		for (const attribute of [...image.attributes]) image.removeAttribute(attribute.name)
		if (source) image.setAttribute('src', source)
	}
	for (const element of doc.body.querySelectorAll('*')) {
		for (const attribute of [...element.attributes]) {
			if (transientAttributes.has(attribute.name) || attribute.name.startsWith('data-pm-')) element.removeAttribute(attribute.name)
		}
		const attributes = [...element.attributes].sort((left, right) => left.name.localeCompare(right.name) || left.value.localeCompare(right.value))
		for (const attribute of attributes) element.removeAttribute(attribute.name)
		for (const attribute of attributes) element.setAttribute(attribute.name, attribute.value)
	}
	for (const node of [...doc.body.childNodes]) {
		if (node.nodeType === Node.TEXT_NODE && !node.textContent?.trim()) node.remove()
	}
	return doc.body.innerHTML.replace(/\r\n?/g, '\n')
}
