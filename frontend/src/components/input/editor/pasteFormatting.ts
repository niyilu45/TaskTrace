import {Extension, type Editor} from '@tiptap/core'
import type {UploadCallback} from './types'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
import {dataUrlAsFile, fileAsDataUrl} from '@/helpers/tasktraceDraftCache'

// Keep document structure and indentation, never the source application's theme.
export function cleanPastedFormatting(html: string): string {
	const doc = new DOMParser().parseFromString(html, 'text/html')
	for (const element of Array.from(doc.body.querySelectorAll<HTMLElement>('*')).reverse()) {
		if (/^(SCRIPT|STYLE|IFRAME|OBJECT|EMBED|SVG|MATH|LINK|META|FORM|INPUT|BUTTON)$/.test(element.tagName)) {element.remove(); continue}
		const indent = element.style.marginLeft || element.style.paddingLeft || element.getAttribute('data-tasktrace-indent') || ''
		const first = element.style.textIndent || ''
		const attrs = new Map(Array.from(element.attributes).map(attr => [attr.name, attr.value]))
		for (const attr of Array.from(element.attributes)) element.removeAttribute(attr.name)
		for (const name of ['src', 'data-src', 'alt', 'href', 'colspan', 'rowspan', 'start', 'data-pm-slice', 'data-type', 'data-checked', 'download']) {
			if (!attrs.has(name)) continue
			const value = attrs.get(name)!
			if (['href', 'src', 'data-src'].includes(name) && /^(?:javascript|vbscript|file):/i.test(value.replace(/\s/g, ''))) continue
			element.setAttribute(name, value)
		}
		if (/^(STRONG|B|EM|I|U|S|STRIKE|FONT)$/.test(element.tagName)) {
			element.replaceWith(...element.childNodes)
			continue
		}
		if (/^(P|DIV|LI|H[1-6])$/.test(element.tagName)) {
			if (/^\d+(?:\.\d+)?(?:px|pt|em|rem)$/.test(indent)) element.style.marginLeft = indent
			if (/^-?\d+(?:\.\d+)?(?:px|pt|em|rem)$/.test(first)) element.style.textIndent = first
		}
	}
	return doc.body.innerHTML
}

export const PasteIndentation = Extension.create({
	name: 'pasteIndentation',
	addGlobalAttributes() {
		return [{types: ['paragraph', 'heading', 'listItem'], attributes: {
			pasteIndent: {
				default: null,
				parseHTML: element => element.style.marginLeft || null,
				renderHTML: attrs => attrs.pasteIndent ? {style: `margin-left: ${attrs.pasteIndent}`} : {},
			},
			firstLineIndent: {
				default: null,
				parseHTML: element => element.style.textIndent || null,
				renderHTML: attrs => attrs.firstLineIndent ? {style: `text-indent: ${attrs.firstLineIndent}`} : {},
			},
		}}]
	},
})

export function isClipboardFileUrl(url: string) {
	return /^data:application\/octet-stream;base64,[a-z0-9+/=\s]+$/i.test(url)
}

async function clipboardFile(source: string, name: string, image: boolean): Promise<File> {
	const attachment = source.match(/^(?:https?:\/\/[^/]+)?\/api\/v[12]\/tasks\/(\d+)\/attachments\/(\d+)$/)
	if (attachment) {
		const origin = new URL(source, window.location.origin).origin
		if (origin !== new URL(window.API_URL, window.location.origin).origin) throw new Error('无法复制其它服务器的附件，请先下载附件。')
		source = await fetchAttachmentBlobUrl({taskId: Number(attachment[1]), id: Number(attachment[2])})
	}
	const file = await dataUrlAsFile(source, name, image ? 'image/png' : 'application/octet-stream')
	return image ? file : new File([file], name, {type: 'application/octet-stream'})
}

// Copy bytes, not per-window blob URLs or attachment links belonging to the source task.
export async function portableClipboardHTML(html: string) {
	const doc = new DOMParser().parseFromString(cleanPastedFormatting(html), 'text/html')
	for (const element of doc.querySelectorAll<HTMLImageElement | HTMLAnchorElement>('img, a[href]')) {
		const image = element.tagName === 'IMG'
		const attribute = image ? 'src' : 'href'
		const source = element.getAttribute('data-src') || element.getAttribute(attribute) || ''
		if (!image && !/\/api\/v[12]\/tasks\/\d+\/attachments\/\d+$/.test(source) && !isClipboardFileUrl(source)) continue
		if (source.startsWith('data:')) continue
		const name = element.getAttribute('download') || element.textContent || (image ? 'clipboard.png' : '附件')
		const file = await clipboardFile(source, name, image)
		element.setAttribute(attribute, await fileAsDataUrl(file))
		element.removeAttribute('data-src')
		if (!image) element.setAttribute('download', name)
	}
	return doc.body.innerHTML
}

// Reserve every media position before asynchronous reads/uploads, even if the user keeps typing.
export async function insertClipboardHTMLImages(editor: Editor, html: string, files: File[], upload: UploadCallback) {
	const doc = new DOMParser().parseFromString(cleanPastedFormatting(html), 'text/html')
	const pending: {element: Element, image: boolean, file: Promise<File>, placeholder: string}[] = []
	const remaining = [...files]
	const add = (element: Element, image: boolean, file: Promise<File>) => {
		const placeholder = URL.createObjectURL(new Blob())
		element.setAttribute(image ? 'src' : 'href', placeholder)
		element.removeAttribute('data-src')
		pending.push({element, image, file, placeholder})
	}
	for (const element of doc.querySelectorAll<HTMLImageElement | HTMLAnchorElement>('img, a[href]')) {
		const image = element.tagName === 'IMG'
		const source = element.getAttribute('data-src') || element.getAttribute(image ? 'src' : 'href') || ''
		if (image && (!source || /^file:/i.test(source))) {
			const index = remaining.findIndex(file => file.type.startsWith('image/'))
			if (index >= 0) {add(element, true, Promise.resolve(remaining.splice(index, 1)[0])); continue}
		}
		if (image && source || !image && isClipboardFileUrl(source)) {
			const name = element.getAttribute('download') || element.textContent || (image ? 'clipboard.png' : '附件')
			add(element, image, clipboardFile(source, name, image))
		}
	}
	// Browsers often provide both CF_HTML and the same bitmap; do not insert it twice.
	const additional = pending.some(item => item.image) ? remaining.filter(file => !file.type.startsWith('image/')) : remaining
	for (const file of additional) {
		const image = file.type.startsWith('image/')
		const element = doc.createElement(image ? 'img' : 'a')
		if (!image) {element.textContent = file.name; element.setAttribute('download', file.name)}
		doc.body.append(element)
		add(element, image, Promise.resolve(image ? file : new File([file], file.name, {type: 'application/octet-stream'})))
	}
	editor.commands.insertContent(doc.body.innerHTML)
	const replace = (urls: string[]) => {
		if (editor.isDestroyed) return
		const transaction = editor.state.tr
		const changes: {pos: number, size: number, index: number, attrs: Record<string, unknown>, link: boolean}[] = []
		editor.state.doc.descendants((node, pos) => {
			if (node.type.name === 'image') {
				const index = pending.findIndex(item => item.image && item.placeholder === node.attrs.src)
				if (index >= 0) changes.push({pos, size: node.nodeSize, index, attrs: node.attrs, link: false})
			} else if (node.isText) {
				for (const mark of node.marks.filter(mark => mark.type.name === 'link')) {
					const index = pending.findIndex(item => !item.image && item.placeholder === mark.attrs.href)
					if (index >= 0) changes.push({pos, size: node.nodeSize, index, attrs: mark.attrs, link: true})
				}
			}
		})
		for (const item of changes.reverse()) {
			const url = urls[item.index]
			if (item.link) {
				const mark = editor.state.schema.marks.link
				transaction.removeMark(item.pos, item.pos + item.size, mark)
				if (url) transaction.addMark(item.pos, item.pos + item.size, mark.create({...item.attrs, href: url}))
			} else if (url) transaction.setNodeMarkup(item.pos, undefined, {...item.attrs, src: url, 'data-src': null})
			else transaction.delete(item.pos, item.pos + item.size)
		}
		editor.view.dispatch(transaction)
	}
	try {
		const prepared = await Promise.all(pending.map(item => item.file))
		const urls = await upload(prepared)
		if (urls.length !== prepared.length) throw new Error('部分图片或文件未能粘贴，请重试。')
		replace(urls)
		return urls
	} catch (error) {
		// Keep failed media as a draft when the bytes were read successfully.
		const fallback = await Promise.all(pending.map(item => item.file.then(fileAsDataUrl).catch(() => '')))
		replace(fallback)
		throw error
	} finally {pending.forEach(item => URL.revokeObjectURL(item.placeholder))}
}
