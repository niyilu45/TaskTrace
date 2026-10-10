import type {TaskComment} from '@/client/generated'
import {splitProgressReferences, normalizeProgressReferences} from './progressReferences'
import {readTeamCommentMarker} from './tasktraceTeam'
import {teamMemberKey} from './tasktraceTeamMembers'
import {deduplicateHtmlImages} from './tasktraceImages'
import {isOutstandingComment, isOutstandingActivity} from './tasktraceCommentTypes'

// A history is consumed by progress, backlinks, filters and summaries. Reuse
// parsing while the exact record fields are unchanged; WeakMap entries disappear
// with their history, and an in-place edit must never reuse an older result.
const parsedNotes = new WeakMap<TaskComment, {fields: unknown[], value: ReturnType<typeof parseRecord>}>()
function readProgressNote(note: TaskComment) {
	const fields = [note.comment, note.id, note.created, note.author?.username, note.author?.name]
	const cached = parsedNotes.get(note)
	if (cached && fields.every((value, index) => value === cached.fields[index])) return cached.value
	const value = parseRecord(note)
	parsedNotes.set(note, {fields, value})
	return value
}
function copyParsedNote(parsed: ReturnType<typeof parseRecord>['parsed']) {
	return {...parsed, references: parsed.references.map(reference => ({...reference, commentIds: [...reference.commentIds]}))}
}
export function parseProgressNote(note: TaskComment) {
	return copyParsedNote(readProgressNote(note).parsed)
}
function parseRecord(note: TaskComment) {
	// Parse in an inert document; all returned HTML is sanitized by ReadonlyRichText before rendering.
	const doc = new DOMParser().parseFromString(note.comment || '', 'text/html')
	const outstandingRecord = isOutstandingComment(note.comment || '', doc) || isOutstandingActivity(note.comment || '', doc)
	const heading = doc.body.querySelector('h3')
	const merged = (name: string) => (heading?.getAttribute(name) || '').split(',').map(value => value.trim()).filter(Boolean)
	const mergedIds = merged('data-tasktrace-merged')
	const mergedTeamIds = merged('data-tasktrace-team-merged')
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
	const progress = doc.body.innerHTML
	const {html: ownProgress, references} = splitProgressReferences(progress)
	const fallbackAuthor = note.author?.username || note.author?.name || ''
	const teamMarker = readTeamCommentMarker(note.comment || '')
	return {outstandingRecord, mergedIds, mergedTeamIds, parsed: {id: note.id, date, daily: !!match, progress, ownProgress, references, outstanding, author: teamMarker?.author || fallbackAuthor, teamId: teamMarker?.id || '', created: Number.isNaN(+created) ? 0 : +created}}
}

function progressAuthorKey(author: string) {
	return teamMemberKey(author) || author.trim().toLocaleLowerCase()
}

function mergedAttribute(comment: string, name: string) {
	const heading = new DOMParser().parseFromString(comment || '', 'text/html').querySelector('h3')
	return (heading?.getAttribute(name) || '').split(',').map(value => value.trim()).filter(Boolean)
}

export function sortProgressNotes(notes: TaskComment[]) {
	const entries = notes.map(raw => ({raw, ...readProgressNote(raw)}))
	const byId = new Map(entries.filter(entry => !!entry.raw.id).map(entry => [entry.raw.id!, entry]))
	const byTeamId = new Map(entries.filter(entry => !!entry.parsed.teamId).map(entry => [entry.parsed.teamId, entry]))
	const absorbed = new Set<number>()
	const absorbedTeam = new Set<string>()
	for (const source of entries) {
		if (!source.parsed.daily) continue
		const lineage = `${source.parsed.date}\u0000${progressAuthorKey(source.parsed.author)}`
		for (const value of source.mergedIds) {
			const id = Number(value)
			const target = byId.get(id)
			if (id > 0 && id !== source.raw.id && target?.parsed.daily && `${target.parsed.date}\u0000${progressAuthorKey(target.parsed.author)}` === lineage) absorbed.add(id)
		}
		for (const id of source.mergedTeamIds) {
			const target = byTeamId.get(id)
			if (id !== source.parsed.teamId && target?.parsed.daily && `${target.parsed.date}\u0000${progressAuthorKey(target.parsed.author)}` === lineage) absorbedTeam.add(id)
		}
	}
	return entries.filter(entry => !absorbed.has(entry.raw.id || 0) && (!entry.parsed.teamId || !absorbedTeam.has(entry.parsed.teamId)))
		.filter(entry => !entry.outstandingRecord)
		.map(entry => copyParsedNote(entry.parsed))
		.sort((a, b) => (b.date === '日期未知' ? '' : b.date).localeCompare(a.date === '日期未知' ? '' : a.date) || b.created - a.created || (b.id || 0) - (a.id || 0))
}

export function finalProgressNotes(notes: TaskComment[]) {
	const seen = new Set<string>()
	return sortProgressNotes(notes).filter(note => {
		if (!note.daily) return true
		const key = `${note.date}\u0000${progressAuthorKey(note.author)}`
		if (seen.has(key)) return false
		seen.add(key)
		return true
	})
}

export function limitProgressNotes(notes: ReturnType<typeof sortProgressNotes>, days: number): ReturnType<typeof sortProgressNotes> {
	if (!Number.isSafeInteger(days) || days <= 0) return notes
	const dates = notes.map(note => {
		if (!/^\d{4}-\d{2}-\d{2}$/.test(note.date)) return null
		const parsed = new Date(`${note.date}T00:00:00Z`)
		if (Number.isNaN(+parsed) || parsed.toISOString().slice(0, 10) !== note.date) return null
		// UTC day numbers avoid local daylight-saving changes at month/day boundaries.
		return +parsed / 86400000
	})
	let latest: number | null = null
	for (const date of dates) if (date !== null && (latest === null || date > latest)) latest = date
	if (latest === null) return notes
	const earliest = latest - days + 1
	return notes.filter((_, index) => {
		const date = dates[index]
		return date === null || date >= earliest
	})
}

export type ProgressBacklink = {id: number, date: string, html: string}

export function progressBacklinks(history: TaskComment[]): Record<number, ProgressBacklink[]> {
	const result: Record<number, ProgressBacklink[]> = {}
	for (const note of sortProgressNotes(history)) {
		if (!note.daily || !note.id || !note.references.length) continue
		for (const reference of note.references) {
			for (const sourceId of reference.commentIds) {
				const items = result[sourceId] ||= []
				if (!items.some(item => item.id === note.id)) items.push({id: note.id, date: note.date, html: note.ownProgress})
			}
		}
	}
	for (const items of Object.values(result)) items.sort((a, b) => b.date.localeCompare(a.date) || b.id - a.id)
	return result
}
export function mergedDay(history: TaskComment[], date: string, author?: string) {
	const authorKey = author ? progressAuthorKey(author) : ''
	const notes = sortProgressNotes(history).filter(note => note.daily && note.date === date && (!authorKey || progressAuthorKey(note.author) === authorKey)).sort((a, b) => a.created - b.created || (a.id || 0) - (b.id || 0))
	const primaryNote = notes.reduce<(typeof notes)[number] | undefined>((latest, note) => !latest || note.created > latest.created || (note.created === latest.created && (note.id || 0) > (latest.id || 0)) ? note : latest, undefined)
	const primary = primaryNote?.id
	const primaryTeamId = primaryNote?.teamId || ''
	const ids = new Set(notes.map(note => note.id).filter((id): id is number => !!id && id !== primary))
	const teamIds = new Set(notes.map(note => note.teamId).filter(id => !!id && id !== primaryTeamId))
	for (const note of history.filter(note => notes.some(active => active.id === note.id))) {
		for (const id of mergedAttribute(note.comment || '', 'data-tasktrace-merged')) if (Number(id) > 0 && Number(id) !== primary) ids.add(Number(id))
		for (const id of mergedAttribute(note.comment || '', 'data-tasktrace-team-merged')) if (id !== primaryTeamId) teamIds.add(id)
	}
	const contentNotes = author && primaryNote ? [primaryNote] : notes
	const html = deduplicateHtmlImages(contentNotes.map(note => note.ownProgress).join(''))
	const references = normalizeProgressReferences(contentNotes.flatMap(note => note.references))
	const doc = new DOMParser().parseFromString(html, 'text/html')
	const images = Array.from(doc.querySelectorAll('img')).map(img => img.outerHTML).join('')
	doc.querySelectorAll('img').forEach(img => img.remove())
	doc.querySelectorAll('br').forEach(br => br.replaceWith('\n'))
	doc.querySelectorAll('p,div,li').forEach(el => el.append('\n'))
	return {id: primary, teamId: primaryTeamId, mergedIds: [...ids], mergedTeamIds: [...teamIds], html, images, text: (doc.body.textContent || '').trim(), references}
}
