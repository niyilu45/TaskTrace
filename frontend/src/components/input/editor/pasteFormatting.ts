import {Extension, type Editor} from '@tiptap/core'
import type {UploadCallback} from './types'

// Keep document structure and indentation, never the source application's theme.
export function cleanPastedFormatting(html: string): string {
	const doc = new DOMParser().parseFromString(html, 'text/html')
	for (const element of Array.from(doc.body.querySelectorAll<HTMLElement>('*')).reverse()) {
		if (/^(SCRIPT|STYLE|IFRAME|OBJECT|EMBED|SVG|MATH|LINK|META|FORM|INPUT|BUTTON)$/.test(element.tagName)) {element.remove(); continue}
		const indent = element.style.marginLeft || element.style.paddingLeft || element.getAttribute('data-tasktrace-indent') || ''
		const first = element.style.textIndent || ''
		const attrs = new Map(Array.from(element.attributes).map(attr => [attr.name, attr.value]))
		for (const attr of Array.from(element.attributes)) element.removeAttribute(attr.name)
		for (const name of ['src', 'data-src', 'alt', 'href', 'colspan', 'rowspan', 'start', 'data-pm-slice', 'data-type', 'data-checked']) {
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

// Reserve image positions synchronously so typing during upload cannot move the pasted pictures.
export async function insertClipboardHTMLImages(editor: Editor, html: string, files: File[], upload: UploadCallback) {
	const doc = new DOMParser().parseFromString(cleanPastedFormatting(html), 'text/html')
	const slots = Array.from(doc.querySelectorAll<HTMLImageElement>('img')).filter(img => !img.getAttribute('src'))
	const placeholders = files.map(file => URL.createObjectURL(file))
	for (const [index, src] of placeholders.entries()) {
		const image = slots[index] || doc.createElement('img')
		image.src = src
		if (!slots[index]) doc.body.append(image)
	}
	editor.commands.insertContent(doc.body.innerHTML)
	const replace = (urls: string[]) => {
		if (editor.isDestroyed) return
		const transaction = editor.state.tr
		const positions: {pos: number, size: number, attrs: Record<string, unknown>, index: number}[] = []
		editor.state.doc.descendants((node, pos) => {
			const index = placeholders.indexOf(node.attrs.src)
			if (node.type.name === 'image' && index >= 0) positions.push({pos, size: node.nodeSize, attrs: node.attrs, index})
		})
		for (const item of positions.reverse()) {
			if (urls[item.index]) transaction.setNodeMarkup(item.pos, undefined, {...item.attrs, src: urls[item.index], 'data-src': null})
			else transaction.delete(item.pos, item.pos + item.size)
		}
		editor.view.dispatch(transaction)
	}
	try {replace(await upload(files))}
	catch (error) {replace([]); throw error}
	finally {placeholders.forEach(url => URL.revokeObjectURL(url))}
}
