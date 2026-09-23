import {describe, it, expect} from 'vitest'
import {finalProgressNotes, parseProgressNote, sortProgressNotes, mergedDay, limitProgressNotes, progressBacklinks} from './progressNotes'
import {serializeProgressReferences, type ProgressReference} from './progressReferences'
import {serializeTeamCommentMarker} from './tasktraceTeam'
const daily = (date: string, progress: string, outstanding = '') => `<h3>每日进展 · ${date}</h3><p>${progress}</p>${outstanding ? `<p><strong>遗留问题 / 下一步</strong></p><p>${outstanding}</p>` : ''}`
describe('daily progress display', () => {
	it('never exposes an outstanding note as daily progress when its heading was rewritten', () => {
		const outstanding = {id: 7, comment: '<h3 data-tasktrace-comment-type="outstanding">已改写标题</h3><ul><li data-id="one"><aside data-tasktrace-outstanding-note="true" hidden><p>遗留备注</p></aside></li></ul>'}
		expect(sortProgressNotes([outstanding])).toEqual([])
	})

	it('sorts by entered dates even when older progress was entered later', () => {
		const notes = sortProgressNotes([{id: 1, created: '2026-09-20T10:00:00Z', comment: daily('2026-09-19', 'new')}, {id: 2, created: '2026-09-21T10:00:00Z', comment: daily('2026-09-16', 'old')}])
		expect(notes.map(note => note.id)).toEqual([1, 2])
	})
	it('splits outstanding items without dropping progress images', () => {
		const note = parseProgressNote({comment: daily('2026-09-19', '完成测试', '等待确认') + '<p><img src="/api/v1/tasks/1/attachments/2"></p>'})
		expect(note.outstanding).toBe('等待确认')
		expect(note.progress).toContain('完成测试')
		expect(note.progress).toContain('<img')
		expect(note.progress).not.toContain('等待确认')
		expect(note.progress).not.toContain('每日进展')
	})
	it('keeps normal comments and uses their creation date', () => {
		const note = parseProgressNote({comment: '<p>普通记录</p>', created: '2026-09-10T12:00:00'})
		expect(note.date).toBe('2026-09-10')
		expect(note.progress).toContain('普通记录')
		expect(note.daily).toBe(false)
	})
	it('clears earlier outstanding items when latest daily record has none', () => {
		const notes = sortProgressNotes([{comment: daily('2026-09-18', '全部完成')}, {comment: daily('2026-09-17', '处理中', '待确认')}])
		expect(notes[0].outstanding).toBe('')
	})
})

it('keeps comment revisions but only shows each member final same-day version in the overview', () => {
	const aliceFirst = {id: 1, created: '2026-09-20T08:00:00Z', comment: daily('2026-09-20', 'alice first') + serializeTeamCommentMarker({id: 'alice-1', author: 'alice'})}
	const bob = {id: 2, created: '2026-09-20T09:00:00Z', comment: daily('2026-09-20', 'bob final') + serializeTeamCommentMarker({id: 'bob-1', author: 'bob'})}
	const aliceFinal = {id: 3, created: '2026-09-20T10:00:00Z', comment: daily('2026-09-20', 'alice final') + serializeTeamCommentMarker({id: 'alice-2', author: 'alice'})}
	const history = [aliceFirst, bob, aliceFinal]

	expect(sortProgressNotes(history).map(note => note.id)).toEqual([3, 2, 1])
	expect(finalProgressNotes(history).map(note => note.id)).toEqual([3, 2])
	const aliceEditor = mergedDay(history, '2026-09-20', 'alice')
	expect(aliceEditor.text).toBe('alice final')
	expect(aliceEditor.mergedIds).toEqual([1])
})

it('loads progress into the same member editor across domain and email account forms', () => {
	const history = [
		{id: 1, created: '2026-09-20T08:00:00Z', comment: daily('2026-09-20', 'first') + serializeTeamCommentMarker({id: 'one', author: 'CHINA\\12345'})},
		{id: 2, created: '2026-09-20T09:00:00Z', comment: daily('2026-09-20', 'final') + serializeTeamCommentMarker({id: 'two', author: '12345@china.example'})},
	]
	expect(finalProgressNotes(history).map(note => note.id)).toEqual([2])
	expect(mergedDay(history, '2026-09-20', '12345').text).toBe('final')
	expect(mergedDay(history, '2026-09-20', 'CHINA\\12345').mergedIds).toEqual([1])
})

it('hides revisions absorbed by stable team ids even when local database ids differ', () => {
	const first = {id: 41, created: '2026-09-20T08:00:00Z', comment: daily('2026-09-20', 'first') + serializeTeamCommentMarker({id: 'shared-first', author: 'alice'})}
	const final = {id: 99, created: '2026-09-20T09:00:00Z', comment: '<h3 data-tasktrace-team-merged="shared-first">每日进展 · 2026-09-20</h3><p>final</p>' + serializeTeamCommentMarker({id: 'shared-final', author: 'alice'})}

	expect(sortProgressNotes([first, final]).map(note => note.id)).toEqual([99])
	expect(mergedDay([first, final], '2026-09-20', 'alice').mergedTeamIds).toEqual(['shared-first'])
})

it('does not hide another member when imported numeric merged ids collide', () => {
	const alice = {id: 41, created: '2026-09-20T08:00:00Z', comment: daily('2026-09-20', 'alice remains') + serializeTeamCommentMarker({id: 'alice-final', author: 'alice'})}
	const bob = {id: 99, created: '2026-09-20T09:00:00Z', comment: '<h3 data-tasktrace-merged="41">每日进展 · 2026-09-20</h3><p>bob final</p>' + serializeTeamCommentMarker({id: 'bob-final', author: 'bob'})}

	expect(finalProgressNotes([alice, bob]).map(note => [note.author, note.id])).toEqual([['bob', 99], ['alice', 41]])
})

it('does not hide another date when imported numeric merged ids collide', () => {
	const earlier = {id: 41, created: '2026-09-19T08:00:00Z', comment: daily('2026-09-19', 'earlier remains') + serializeTeamCommentMarker({id: 'alice-earlier', author: 'alice'})}
	const later = {id: 99, created: '2026-09-20T09:00:00Z', comment: '<h3 data-tasktrace-merged="41">每日进展 · 2026-09-20</h3><p>later final</p>' + serializeTeamCommentMarker({id: 'alice-later', author: 'alice'})}

	expect(sortProgressNotes([earlier, later]).map(note => note.id)).toEqual([99, 41])
})

it('merges same-day content and images while hiding absorbed source records', () => {
	const original = [{id: 1, comment: daily('2026-09-20', 'first')}, {id: 2, comment: daily('2026-09-20', 'second') + '<img src="/api/v1/tasks/1/attachments/9">'}]
	const merged = mergedDay(original, '2026-09-20')
	expect(merged.text).toContain('first')
	expect(merged.text).toContain('second')
	expect(merged.images).toContain('/attachments/9')
	expect(merged.mergedIds).toEqual([1])
	const saved = {...original[1], comment: '<h3 data-tasktrace-merged="1">每日进展 · 2026-09-20</h3><p>edited</p>'}
	expect(sortProgressNotes([original[0], saved]).map(note => note.id)).toEqual([2])
	expect(mergedDay([original[0], saved], '2026-09-20').mergedIds).toEqual([1])
})

describe('progress date range', () => {
	const notesWithDates = (dates: string[]) => dates.map((date, index) => ({...parseProgressNote({id: index + 1, comment: daily('2026-01-01', `progress ${index}`)}), date}))

	it.each([0, -1, 1.5, NaN, Infinity, -Infinity, Number.MAX_SAFE_INTEGER + 1])('keeps all notes for invalid or unlimited days %s', days => {
		const notes = notesWithDates(['2026-09-20', '2020-01-01'])
		expect(limitProgressNotes(notes, days)).toBe(notes)
	})

	it('anchors to the latest parsed date instead of today, creation time or the number of records', () => {
		const notes = sortProgressNotes([
			{id: 1, created: '2020-06-11T00:00:00Z', comment: daily('2020-06-10', 'latest')},
			{id: 2, created: '2099-01-01T00:00:00Z', comment: daily('2020-06-08', 'boundary')},
			{id: 3, created: '2099-02-01T00:00:00Z', comment: daily('2020-06-07', 'outside')},
		])
		expect(limitProgressNotes(notes, 3).map(note => note.id)).toEqual([1, 2])
	})

	it('retains every record on the latest day even when only one day is selected', () => {
		const notes = notesWithDates(['2026-09-20', '2026-09-20', '2026-09-19'])
		expect(limitProgressNotes(notes, 1).map(note => note.id)).toEqual([1, 2])
	})

	it.each([
		[['2026-03-01', '2026-02-28', '2026-02-27'], 2, 2],
		[['2027-01-01', '2026-12-31', '2026-12-30'], 2, 2],
		[['2024-03-01', '2024-02-29', '2024-02-28', '2024-02-27'], 3, 3],
		[['2025-03-01', '2025-02-28', '2025-02-27'], 2, 2],
		[['2026-03-09', '2026-03-08', '2026-03-07'], 2, 2],
		[['2026-11-02', '2026-11-01', '2026-10-31'], 2, 2],
	] as [string[], number, number][])('uses inclusive natural days across calendar boundaries: %j', (dates, days, count) => {
		const notes = notesWithDates(dates)
		expect(limitProgressNotes(notes, days).map(note => note.date)).toEqual(dates.slice(0, count))
	})

	it('retains unknown and invalid dates without using them as anchors', () => {
		const dates = ['日期未知', '2026-02-30', '2026-13-01', '', '2026-09-09', '2026-09-07']
		const notes = notesWithDates(dates)
		expect(limitProgressNotes(notes, 1).map(note => note.date)).toEqual(dates.slice(0, 5))
	})

	it('preserves original ordering and objects without mutating unsorted input', () => {
		const notes = notesWithDates(['2026-09-18', '2026-09-16', '日期未知', '2026-09-19'])
		notes.forEach(Object.freeze)
		Object.freeze(notes)
		const filtered = limitProgressNotes(notes, 2)
		expect(filtered).toEqual([notes[0], notes[2], notes[3]])
		expect(filtered[0]).toBe(notes[0])
		expect(notes.map(note => note.date)).toEqual(['2026-09-18', '2026-09-16', '日期未知', '2026-09-19'])
	})

	it('keeps empty or entirely undated history unchanged', () => {
		const empty = notesWithDates([])
		const unknown = notesWithDates(['日期未知', '2026-02-30'])
		expect(limitProgressNotes(empty, 7)).toBe(empty)
		expect(limitProgressNotes(unknown, 7)).toBe(unknown)
	})
})

it('shows one image when collaboration produced duplicate image tags', () => {
	const image = '<p><img src="/api/v1/tasks/4/attachments/7" alt="协作图片"></p>'
	const merged = mergedDay([
		{id: 1, comment: daily('2026-09-20', 'first') + image},
		{id: 2, comment: daily('2026-09-20', 'second') + image.replace('/v1/', '/v2/')},
	], '2026-09-20')
	expect((merged.images.match(/<img/g) || []).length).toBe(1)
})
describe('progress reference isolation', () => {
	const ref: ProgressReference = {id: 'r0123456789abcdef0123456789abcdef', taskId: 123, date: '2026-09-17', commentIds: [55], html: '<p>引用旧内容</p><p><img src="/api/v1/tasks/123/attachments/4"></p>'}
	it('keeps full display HTML but excludes quotes from editable text and own images', () => {
		const history = [{id: 90, comment: daily('2026-09-18', '今日更正', '仍然待办') + '<p><img src="/api/v1/tasks/123/attachments/6"></p>' + serializeProgressReferences([ref])}]
		const note = parseProgressNote(history[0])
		expect(note.progress).toContain('引用旧内容')
		expect(note.ownProgress).not.toContain('引用旧内容')
		expect(note.outstanding).toBe('仍然待办')
		const merged = mergedDay(history, '2026-09-18')
		expect(merged.text).toBe('今日更正')
		expect(merged.html).not.toContain('data-tasktrace-reference')
		expect(merged.images).toContain('/attachments/6')
		expect(merged.images).not.toContain('/attachments/4')
		expect(merged.references).toEqual([ref])
	})
	it('deduplicates identical reference IDs during day merge without hiding source records', () => {
		const second = {...ref, id: 'rfedcba9876543210fedcba9876543210', html: '<p>不同快照</p>'}
		const history = [
			{id: 55, comment: daily('2026-09-17', '历史原记录')},
			{id: 90, comment: daily('2026-09-18', '更正一') + serializeProgressReferences([ref])},
			{id: 91, comment: daily('2026-09-18', '更正二') + serializeProgressReferences([ref, second])},
		]
		const merged = mergedDay(history, '2026-09-18')
		expect(merged.id).toBe(91)
		expect(merged.mergedIds).toEqual([90])
		expect(merged.references.map(value => value.id)).toEqual([ref.id, second.id])
		expect(sortProgressNotes(history).map(note => note.id)).toEqual([91, 90, 55])
		const saved = {...history[2], comment: '<h3 data-tasktrace-merged="90">每日进展 · 2026-09-18</h3>' + merged.html + serializeProgressReferences(merged.references)}
		expect(sortProgressNotes([history[0], history[1], saved]).map(note => note.id)).toEqual([91, 55])
		expect(mergedDay([history[0], history[1], saved], '2026-09-18').references).toEqual(merged.references)
	})
	it('links source records back to the progress entries that cite them', () => {
		const history = [
			{id: 55, comment: daily('2026-09-17', '历史原记录')},
			{id: 90, comment: daily('2026-09-18', '更正内容') + serializeProgressReferences([ref])},
		]
		expect(progressBacklinks(history)[55]).toEqual([{id: 90, date: '2026-09-18', html: '<p>更正内容</p>'}])
		expect(progressBacklinks(history)[90]).toBeUndefined()
	})
})
