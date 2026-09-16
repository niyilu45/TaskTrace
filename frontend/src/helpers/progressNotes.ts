import type {TaskComment} from '@/client/generated'

export function parseProgressNote(note: TaskComment) {
	// Parse in an inert document; all returned HTML is sanitized by ReadonlyRichText before rendering.
	const doc = new DOMParser().parseFromString(note.comment || '', 'text/html')
	const heading = doc.body.querySelector('h3')
	const match = heading?.textContent?.trim().match(/^每日进展\s*[·:：]\s*(\d{4}-\d{2}-\d{2})$/)
	const created = new Date(note.created || '')
	const fallback = Number.isNaN(+created) ? '日期未知' : `${created.getFullYear()}-${String(created.getMonth() + 1).padStart(2, '0')}-${String(created.getDate()).padStart(2, '0')}`
	const candidate = match?.[1]
	const parsedDate = candidate ? new Date(candidate + 'T00:00:00Z') : null
	const validDate = parsedDate && !Number.isNaN(+parsedDate) && parsedDate.toISOString().slice(0, 10) === candidate
	const date = validDate ? candidate! : fallback
	let outstanding = ''
	if (match) {
		heading?.remove()
		const label = Array.from(doc.body.querySelectorAll('p')).find(p => p.textContent?.trim() === '遗留问题 / 下一步' && p.querySelector('strong'))
		if (label) {
			const content = label.nextElementSibling
			if (content?.tagName === 'P') { outstanding = content.innerHTML; content.remove() }
			label.remove()
		}
	}
	return {id: note.id, date, daily: !!match, progress: doc.body.innerHTML, outstanding, created: Number.isNaN(+created) ? 0 : +created}
}

export function sortProgressNotes(notes: TaskComment[]) {
	return notes.map(parseProgressNote).sort((a, b) => (b.date === '日期未知' ? '' : b.date).localeCompare(a.date === '日期未知' ? '' : a.date) || b.created - a.created || (b.id || 0) - (a.id || 0))
}
