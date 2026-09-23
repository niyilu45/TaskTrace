export const outstandingHeading = 'TaskTrace 遗留事项清单'
export const outstandingTypeAttribute = 'data-tasktrace-comment-type'
export const outstandingType = 'outstanding'

export function isOutstandingComment(comment: string) {
	const doc = new DOMParser().parseFromString(comment || '', 'text/html')
	if (doc.querySelector(`[${outstandingTypeAttribute}="${outstandingType}"]`)) return true
	if (doc.querySelector('h3')?.textContent?.trim() === outstandingHeading) return true
	return !!doc.querySelector('aside[data-tasktrace-outstanding-note]')
}
