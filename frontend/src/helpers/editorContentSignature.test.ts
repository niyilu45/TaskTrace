import {describe, expect, it} from 'vitest'
import {editorContentSignature} from './editorContentSignature'

describe('editorContentSignature', () => {
	it('ignores browser-only markup and image preview changes', () => {
		const saved = `<P style="margin: 0" class="ProseMirror-selectednode">正文</P><IMG src="data:image/png;base64,old" data-tasktrace-src="/api/v1/tasks/42/attachments/8">`
		const opened = `<p>正文</p><img data-tasktrace-src="/api/v2/tasks/42/attachments/8" src="blob:new-preview">`
		expect(editorContentSignature(opened)).toBe(editorContentSignature(saved))
	})

	it('keeps meaningful formatting and line breaks distinct', () => {
		expect(editorContentSignature('<p><strong>正文</strong></p>')).not.toBe(editorContentSignature('<p>正文</p>'))
		expect(editorContentSignature('<p>第一行<br>第二行</p>')).not.toBe(editorContentSignature('<p>第一行 第二行</p>'))
	})
})
