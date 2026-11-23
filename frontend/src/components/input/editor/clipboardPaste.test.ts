import {describe, it, expect, afterEach, beforeEach, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {ref} from 'vue'
import {Editor} from '@tiptap/core'
import {TextSelection} from '@tiptap/pm/state'

import {insertClipboardHTMLImages} from './pasteFormatting'
import {createEditorExtensions, type EditorExtensionDeps} from './editorExtensions'

vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn(async () => 'blob:fixture-image')}))
window.API_URL = 'http://localhost:3456/api/v1'

function createEditor(content: string) {
	const holder = document.createElement('div')
	document.body.appendChild(holder)

	let editor: Editor
	const deps: EditorExtensionDeps = {
		t: key => key,
		isEditing: ref(true),
		isEditEnabled: () => true,
		placeholder: '',
		contentHasChanged: ref(false),
		bubbleSave: () => {},
		getEditor: () => editor,
		uploadCallback: undefined,
		uploadAndInsertFiles: () => {},
	}

	editor = new Editor({element: holder, extensions: createEditorExtensions(deps), content, parseOptions: {preserveWhitespace: true}})

	return editor
}

function pasteHTML(editor: Editor, html: string, at = 1) {
	editor.view.dispatch(editor.state.tr.setSelection(TextSelection.create(editor.state.doc, at)))
	editor.view.pasteHTML(html)
}

function sliceContext(context: unknown[]) {
	return JSON.stringify(context).replaceAll('"', '&quot;')
}

beforeEach(() => {
	setActivePinia(createPinia())
})

afterEach(() => {
	document.body.innerHTML = ''
})

describe('pasting html the schema cannot hold', () => {
	// A ProseMirror editor whose list items hold inline content directly puts a context
	// on the clipboard that makes our listItem swallow bare text, see FRONTEND-OSS-2H9.
	it.each([
		['a list item holding text', ['bulletList', null, 'listItem', null], '<ul><li><p>pasted</p></li></ul>'],
		['an ordered list item holding text', ['orderedList', null, 'listItem', null], '<ol><li><p>pasted</p></li></ol>'],
		['a task item holding text', ['taskList', null, 'taskItem', null], '<p>pasted</p>'],
	])('wraps text pasted into %s', (_name, context, expected) => {
		const editor = createEditor('<p></p>')

		expect(() => pasteHTML(editor, `<span data-pm-slice="0 0 ${sliceContext(context)}">pasted</span>`)).not.toThrow()
		expect(() => editor.state.doc.check()).not.toThrow()
		expect(editor.getHTML()).toContain(expected)
	})

	// Copying a list item from the nested list inside it yields a listItem whose first
	// child is that nested list instead of the paragraph the schema requires.
	it.each([
		['an empty document', '<p></p>', 1],
		['a list item', '<ol><li><p>hello</p></li></ol>', 4],
	])('fills in the missing paragraph when pasting a nested list into %s', (_name, content, at) => {
		const editor = createEditor(content)
		const html = '<li data-pm-slice="1 0 []"><ol><li><p>nested</p></li></ol></li>'

		expect(() => pasteHTML(editor, html, at)).not.toThrow()
		expect(() => editor.state.doc.check()).not.toThrow()
		expect(editor.getHTML()).toContain('<li><p>nested</p></li>')
	})

	it('leaves valid pasted content alone', () => {
		const editor = createEditor('<p></p>')

		pasteHTML(editor, '<ol><li><p>one</p></li><li><p>two</p></li></ol>')

		expect(editor.getHTML()).toBe('<ol><li><p>one</p></li><li><p>two</p></li></ol><p></p>')
	})

	it('keeps a list copied out of the editor intact', () => {
		const source = createEditor('<ol><li><p>one</p></li><li><p>two</p></li></ol>')
		const copied = source.view.serializeForClipboard(source.state.doc.slice(0, source.state.doc.content.size)).dom as HTMLElement

		const editor = createEditor('<p>hello</p>')
		pasteHTML(editor, copied.innerHTML, 6)

		expect(editor.getHTML()).toBe('<p>hello</p><ol><li><p>one</p></li><li><p>two</p></li></ol><p></p>')
	})
})


describe('Word pictures with surrounding text', () => {
	it('keeps image positions even if the selection moves before upload finishes', async () => {
		const editor = createEditor('<p>existing</p>')
		let finish!: (urls: string[]) => void
		const upload = vi.fn(() => new Promise<string[]>(resolve => {finish = resolve}))
		const pending = insertClipboardHTMLImages(editor, '<p style="color:red">before</p><img src="file:///word.png"><p>after</p>', [new File(['png'], 'word.png', {type: 'image/png'})], upload)
		editor.commands.focus('end')
		editor.commands.insertContent('typed later')
		await vi.waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
		finish(['https://images.example/word.png'])
		await pending
		const html = editor.getHTML()
		expect(html.indexOf('before')).toBeLessThan(html.indexOf('https://images.example/word.png'))
		expect(html.indexOf('https://images.example/word.png')).toBeLessThan(html.indexOf('after'))
		expect(html).toContain('typed later')
		expect(html).not.toMatch(/color|file:|blob:/)
		editor.destroy()
	})
	it('keeps text and removes unusable image placeholders if uploading fails', async () => {
		const editor = createEditor('<p></p>')
		await expect(insertClipboardHTMLImages(editor, '<p>before</p><img src="file:///word.png"><p>after</p>', [new File(['png'], 'word.png', {type: 'image/png'})], async () => {throw new Error('disk full')})).rejects.toThrow('disk full')
		expect(editor.getHTML()).toContain('before')
		expect(editor.getHTML()).toContain('after')
		expect(editor.getHTML()).not.toContain('blob:')
		editor.destroy()
	})
})


describe('clipboard layout whitespace', () => {
	it('does not create blank paragraphs from source HTML indentation between blocks', () => {
		const editor = createEditor('<p></p>')
		try {
			pasteHTML(editor, '\n<div>\n  <p>第一行</p>\n  <p>第二行</p>\n</div>\n')
			expect(editor.getJSON().content?.map(node => node.content?.map(child => child.text || '').join('') || '')).toEqual(['第一行', '第二行'])
		} finally {editor.destroy()}
	})
	it('does not duplicate a line break from HTML source newlines around br', () => {
		const editor = createEditor('<p></p>')
		try {
			pasteHTML(editor, '<p>第一行\r\n<br>\r\n第二行</p>')
			expect(editor.getHTML()).toBe('<p>第一行<br>第二行</p>')
		} finally {editor.destroy()}
	})
	it('preserves intentional empty paragraphs, repeated breaks and plain-text line breaks', () => {
		const editor = createEditor('<p></p>')
		try {
			pasteHTML(editor, '<p>甲<br><br>乙</p><p></p><p>丙</p>')
			expect(editor.getHTML()).toBe('<p>甲<br><br>乙</p><p></p><p>丙</p>')
		} finally {editor.destroy()}
	})
})

it('pastes plain-text newlines as single visual lines rather than separated paragraphs', () => {
	const editor = createEditor('<p></p>')
	try {
		editor.view.pasteText('甲\r\n乙\r\n\r\n丙')
		expect(editor.getHTML()).toBe('<p>甲<br>乙<br><br>丙</p>')
	} finally {editor.destroy()}
})

it('does not insert layout-only empty paragraphs when pasting indented HTML with an image', async () => {
	const editor = createEditor('<p></p>')
	try {
		await insertClipboardHTMLImages(editor, '\n<p>前文</p>\n<img src="file:///word.png">\n<p>后文</p>\n', [new File(['png'], 'word.png', {type: 'image/png'})], async () => ['/api/v1/tasks/1/attachments/1'])
		expect(editor.getHTML()).not.toContain('<p></p>')
		expect(editor.getHTML()).toContain('前文')
		expect(editor.getHTML()).toContain('后文')
		expect(editor.getHTML().match(/<img /g)).toHaveLength(1)
	} finally {editor.destroy()}
})
