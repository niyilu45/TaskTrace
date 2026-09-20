import {describe, expect, it} from 'vitest'

import {stripEditorUploadPlaceholders} from './editorUploadPlaceholder'

describe('editor upload placeholder', () => {
	it('removes legacy upload placeholders without changing surrounding text and images', () => {
		const html = '<p>已有文字</p><p><strong>UPLOAD_PLACEHOLDER</strong></p><img src="data:image/png;base64,AQID"><p>后续文字</p>'
		expect(stripEditorUploadPlaceholders(html)).toBe('<p>已有文字</p><img src="data:image/png;base64,AQID"><p>后续文字</p>')
	})

	it('does not remove the marker when it is part of normal text', () => {
		expect(stripEditorUploadPlaceholders('<p>保留 UPLOAD_PLACEHOLDER 说明</p>')).toBe('<p>保留 UPLOAD_PLACEHOLDER 说明</p>')
	})
})
