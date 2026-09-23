import {describe, expect, it} from 'vitest'
import {deduplicateHtmlImages} from './tasktraceImages'

describe('tasktrace image deduplication', () => {
	it('uses retained attachment urls when editor images share the same placeholder src', () => {
		const html = '<img src="#" data-src="/api/v1/tasks/8/attachments/1"><img src="#" data-src="/api/v1/tasks/8/attachments/2">'
		expect(new DOMParser().parseFromString(deduplicateHtmlImages(html), 'text/html').querySelectorAll('img')).toHaveLength(2)
	})

	it('still removes repeated references to the same attachment', () => {
		const html = '<img src="#" data-src="/api/v1/tasks/8/attachments/1"><img src="/api/v2/tasks/8/attachments/1">'
		expect(new DOMParser().parseFromString(deduplicateHtmlImages(html), 'text/html').querySelectorAll('img')).toHaveLength(1)
	})
})
