import {undoGroupHeaders} from '@/helpers/tasktraceUndo'
import {taskCommentsList, taskCommentsCreate, taskCommentsUpdate, type TaskComment} from '@/client/generated'
import {sortProgressNotes} from './progressNotes'
import {deduplicateHtmlImages} from './tasktraceImages'
import {isOutstandingComment, outstandingHeading, outstandingType, outstandingTypeAttribute} from './tasktraceCommentTypes'
import {TASKTRACE_DEFAULT_PRIORITY} from './tasktracePriority'

export const sharedHeading = outstandingHeading
export type OutstandingItem = {
	id: string
	html: string
	note?: string
	done?: boolean
	priority?: number
	completedAt?: string
	reminderAt?: string
}

const outstandingNoteSelector = 'aside[data-tasktrace-outstanding-note]'

function outstandingContent(li: Element) {
	const clone = li.cloneNode(true) as Element
	const note = clone.querySelector(outstandingNoteSelector)
	const noteHtml = note?.innerHTML || ''
	note?.remove()
	return {
		html: deduplicateHtmlImages(clone.innerHTML),
		...(noteHtml ? {note: deduplicateHtmlImages(noteHtml)} : {}),
	}
}

function serializeOutstandingNote(note?: string) {
	return note?.trim() ? `<aside data-tasktrace-outstanding-note="true" hidden>${note}</aside>` : ''
}

function priority(value: string | null | undefined) {
	if (value == null || value.trim() === '') return TASKTRACE_DEFAULT_PRIORITY
	const parsed = Number(value)
	return Number.isInteger(parsed) ? Math.max(0, Math.min(9, parsed)) : TASKTRACE_DEFAULT_PRIORITY
}

function attribute(value: string) {
	return value.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function sharedOutstanding(history: TaskComment[]) {
	const record = [...history].sort((a, b) => (b.id || 0) - (a.id || 0)).find(note => isOutstandingComment(note.comment || ''))
	if (record) {
		const doc = new DOMParser().parseFromString(record.comment || '', 'text/html')
		const list = doc.querySelector('h3 + ul') || Array.from(doc.querySelectorAll('ul')).find(candidate => candidate.querySelector(':scope > li[data-id]'))
		const entries = list ? Array.from(list.children).filter(child => child.tagName === 'LI') : []
		return {
			id: record.id,
			items: entries.map((li, index) => {
				const content = outstandingContent(li)
				return {
					id: li.getAttribute('data-id') || `item-${index}`,
					...content,
					done: li.getAttribute('data-done') === 'true',
					priority: priority(li.getAttribute('data-priority')),
					completedAt: li.getAttribute('data-completed-at') || undefined,
					reminderAt: li.getAttribute('data-reminder') || undefined,
				}
			}),
		}
	}
	const latest = sortProgressNotes(history).find(note => note.daily)
	const html = latest?.outstanding || ''
	return {
		id: undefined,
		items: html
			? html.split(/<br\s*\/?>(?:\r?\n)?/i).filter(value => value.trim()).map((itemHtml, index) => ({id: `legacy-${latest?.id}-${index}`, html: itemHtml, done: false, priority: TASKTRACE_DEFAULT_PRIORITY}))
			: [],
	}
}

export function outstandingHtml(history: TaskComment[]) {
	const items = sharedOutstanding(history).items.filter(item => !item.done)
	return items.length ? `<ol>${items.map(item => `<li>${item.html}</li>`).join('')}</ol>` : ''
}

export async function readTaskHistory(taskId: number) {
	const history: TaskComment[] = []
	for (let page = 1; ; page++) {
		const result = await taskCommentsList({path: {task: taskId}, query: {page, per_page: 100, order_by: 'desc'}})
		const items = result.data.items || []
		history.push(...items)
		if (page >= (result.data.total_pages || 1) || !items.length) break
	}
	return history
}

export async function changeOutstanding(taskId: number, change: (items: OutstandingItem[]) => OutstandingItem[], headers = undoGroupHeaders()) {
	const current = sharedOutstanding(await readTaskHistory(taskId))
	const items = change(current.items)
	const comment = `<h3 ${outstandingTypeAttribute}="${outstandingType}">${sharedHeading}</h3><ul>${items.map(item => {
		const completedAt = item.completedAt ? ` data-completed-at="${attribute(item.completedAt)}"` : ''
		const reminderAt = item.reminderAt ? ` data-reminder="${attribute(item.reminderAt)}"` : ''
		return `<li data-id="${item.id.replace(/[^a-zA-Z0-9-]/g, '')}" data-done="${item.done ? 'true' : 'false'}" data-priority="${priority(String(item.priority ?? TASKTRACE_DEFAULT_PRIORITY))}"${completedAt}${reminderAt}>${item.html}${serializeOutstandingNote(item.note)}</li>`
	}).join('')}</ul>`
	if (current.id) await taskCommentsUpdate({path: {task: taskId, commentid: current.id}, body: {comment}, headers})
	else await taskCommentsCreate({path: {task: taskId}, body: {comment}, headers})
	return items
}
