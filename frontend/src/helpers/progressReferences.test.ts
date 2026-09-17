import {describe, expect, it} from 'vitest'
import {mergedDay, parseProgressNote} from './progressNotes'
import {createProgressReference, normalizeProgressReferences, serializeProgressReferences, splitProgressReferences, type ProgressReference} from './progressReferences'

const reference = (overrides: Partial<ProgressReference> = {}): ProgressReference => ({
	id: 'r0123456789abcdef0123456789abcdef', taskId: 123, date: '2026-09-17', commentIds: [1, 2],
	html: '<p>旧进展<br>第二行</p><p><img src="/api/v1/tasks/123/attachments/4"></p>', ...overrides,
})
const day = {id: 2, mergedIds: [1], html: '<p>原进展</p>'}

describe('progress reference protocol', () => {
	it('round-trips metadata and snapshots while separating own text and images', () => {
		const ref = reference()
		const own = '<p>本日更正说明</p><p><img src="/api/v1/tasks/123/attachments/8"></p>'
		const serialized = serializeProgressReferences([ref])
		expect(serialized).toContain('href="/tasks/123#comment-2"')
		expect(serialized).toContain('data-comment-ids="1,2"')
		const split = splitProgressReferences(own + serialized)
		expect(split.html).toBe(own)
		expect(split.references).toEqual([ref])
		expect(serializeProgressReferences(split.references)).toBe(serialized)
	})

	it('round-trips the native SaveProgress fixture without mixing snapshot images into own content', () => {
		// Actual C# SaveProgress output from the isolated native integration fixture.
		const fixture = {commentId: 14, taskId: 65, html: '<h3 data-tasktrace-merged="13">每日进展 · 2026-09-16</h3><p><strong>旧结论</strong>待更正</p><p><img src="/api/v1/tasks/65/attachments/9" alt="历史图片"></p><blockquote data-tasktrace-reference="1" data-reference-id="r58cfc647f97b4c45892294b1c2a47b63" data-task-id="65" data-date="2026-09-13" data-comment-ids="11"><p><strong>引用 2026-09-13 的进展</strong> <a href="/tasks/65#comment-11">查看原记录</a></p><div data-tasktrace-reference-content="1"><p>最早的原记录</p><p><img src="/api/v1/tasks/65/attachments/9"></p></div></blockquote>'}
		const own = '<p><strong>旧结论</strong>待更正</p><p><img src="/api/v1/tasks/65/attachments/9" alt="历史图片"></p>'
		const source = {id: fixture.commentId, task_id: fixture.taskId, comment: fixture.html}
		const note = parseProgressNote(source)
		expect(note.date).toBe('2026-09-16')
		expect(note.ownProgress).toBe(own)
		expect(note.references).toEqual([{
			id: 'r58cfc647f97b4c45892294b1c2a47b63', taskId: 65, date: '2026-09-13', commentIds: [11],
			html: '<p>最早的原记录</p><p><img src="/api/v1/tasks/65/attachments/9"></p>',
		}])
		expect(serializeProgressReferences(note.references)).toBe(fixture.html.slice(fixture.html.indexOf('<blockquote')))
		const split = splitProgressReferences(fixture.html)
		expect(split.html).toBe('<h3 data-tasktrace-merged="13">每日进展 · 2026-09-16</h3>' + own)
		expect(split.references).toEqual(note.references)
		const merged = mergedDay([source], note.date)
		expect(merged).toMatchObject({id: 14, mergedIds: [13], html: own, text: '旧结论待更正', references: note.references})
		expect(merged.images).toBe('<img src="/api/v1/tasks/65/attachments/9" alt="历史图片">')
	})
	it('normalizes copied content into escaped text and same-task attachment images only', () => {
		const input = reference({html: '<script>alert(1)</script><style>bad</style><h3 data-tasktrace-merged="7">标题</h3><p onclick="bad()">A &amp; &lt;b&gt;</p><a href="javascript:bad()">链接文字</a><img src="/api/v2/tasks/123/attachments/4" onerror="bad()"><img src="/api/v1/tasks/999/attachments/5"><img src="https://evil.example/api/v1/tasks/123/attachments/6"><iframe src="https://evil.example"></iframe>'})
		const [clean] = normalizeProgressReferences([input])
		expect(clean.html).toContain('标题')
		expect(clean.html).toContain('A &amp; &lt;b&gt;')
		expect(clean.html).toContain('链接文字')
		expect(clean.html).toContain('<p><img src="/api/v2/tasks/123/attachments/4"></p>')
		for (const unsafe of ['<h3', 'data-tasktrace-merged', 'script', 'onclick', 'onerror', 'javascript:', 'evil.example', '/tasks/999/', 'alert(1)']) expect(clean.html).not.toContain(unsafe)
		expect(input.html).toContain('onclick')
	})

	it.each([
		{id: 'r" onclick="bad()'}, {id: 'rABCDEF0123456789abcdef0123456789ab'},
		{taskId: 0}, {taskId: -1}, {taskId: 1.5}, {taskId: Number.MAX_SAFE_INTEGER + 1},
		{date: '2026-02-30'}, {date: '2026-9-17'}, {date: '2026-09-17"'},
		{commentIds: []}, {commentIds: [0]}, {commentIds: [-1]}, {commentIds: [1.5]}, {commentIds: [Number.MAX_SAFE_INTEGER + 1]},
	] as Partial<ProgressReference>[])('rejects invalid reference metadata %j', override => {
		expect(normalizeProgressReferences([reference(override)])).toEqual([])
		expect(serializeProgressReferences([reference(override)])).toBe('')
	})

	it('validates unknown draft structures and deduplicates by snapshot ID only', () => {
		expect(normalizeProgressReferences(null)).toEqual([])
		expect(normalizeProgressReferences({})).toEqual([])
		expect(normalizeProgressReferences([null, 2, {}, {...reference(), html: 42}, {...reference(), taskId: '123'}, {...reference(), commentIds: ['1']}])).toEqual([])
		const first = reference({commentIds: [2, 1, 2]})
		const second = reference({id: 'rfedcba9876543210fedcba9876543210', html: '<p>同日另一份快照</p>'})
		const refs = normalizeProgressReferences([first, first, second])
		expect(refs.map(ref => ref.id)).toEqual([first.id, second.id])
		expect(refs[0].commentIds).toEqual([1, 2])
		expect(refs[0].commentIds).not.toBe(first.commentIds)
	})

	it.each([
		['data-tasktrace-reference="1"', 'data-tasktrace-reference="2"'],
		['data-task-id="123"', 'data-task-id="123e0"'],
		['data-comment-ids="1,2"', 'data-comment-ids="1,0"'],
		['data-date="2026-09-17"', 'data-date="2026-02-30"'],
		['data-tasktrace-reference-content="1"', 'data-tasktrace-reference-content="2"'],
	])('preserves unrecognized wrappers as ordinary content: %s', (before, after) => {
		const html = serializeProgressReferences([reference()]).replace(before, after)
		const split = splitProgressReferences(html)
		expect(split.references).toEqual([])
		expect(split.html).toBe(html)
	})

	it('does not discard extra user content outside a malformed snapshot body', () => {
		const html = serializeProgressReferences([reference()]).replace('</blockquote>', '<p>后来写入的正文</p></blockquote>')
		expect(splitProgressReferences(html)).toEqual({html, references: []})
	})
})

describe('creating a frozen reference', () => {
	it('allows only strictly earlier valid dates and positive source IDs', () => {
		for (const [source, target] of [['2026-09-17', '2026-09-17'], ['2026-09-18', '2026-09-17'], ['2026-02-30', '2026-03-01'], ['2026-01-01', '日期未知']]) expect(createProgressReference(123, source, target, day)).toBeNull()
		expect(createProgressReference(0, '2026-09-16', '2026-09-17', day)).toBeNull()
		expect(createProgressReference(123, '2026-09-16', '2026-09-17', {...day, id: 0})).toBeNull()
		expect(createProgressReference(123, '2026-09-16', '2026-09-17', {...day, mergedIds: [1, -2]})).toBeNull()
		expect(createProgressReference(123, '2026-09-16', '2026-09-17', {html: day.html, mergedIds: []})).toBeNull()
	})

	it.each([['2024-02-29', '2024-03-01'], ['2025-12-31', '2026-01-01']])('accepts earlier natural dates across boundaries %s → %s', (source, target) => {
		const snapshot = createProgressReference(123, source, target, day)!
		expect(snapshot).not.toBeNull()
		expect(snapshot.id).toMatch(/^r[0-9a-f]{12}4[0-9a-f]{3}[89ab][0-9a-f]{15}$/)
		expect(snapshot.commentIds).toEqual([1, 2])
	})

	it('copies own content and images while excluding earlier references recursively', () => {
		const old = serializeProgressReferences([reference({html: '<p>不再重复的引用</p><p><img src="/api/v1/tasks/123/attachments/4"></p>'})])
		const source = {id: 9, mergedIds: [7, 8], html: '<p>本条内容 &amp; 更正</p><img src="/api/v1/tasks/123/attachments/6">' + old}
		const snapshot = createProgressReference(123, '2026-09-18', '2026-09-19', source)!
		expect(snapshot.html).toBe('<p>本条内容 &amp; 更正</p><p><img src="/api/v1/tasks/123/attachments/6"></p>')
		expect(snapshot.commentIds).toEqual([7, 8, 9])
		source.html = '<p>源记录后来改动或删除</p>'; source.mergedIds.push(99)
		expect(snapshot.html).toContain('本条内容')
		expect(snapshot.commentIds).not.toContain(99)
		expect(createProgressReference(123, '2026-09-18', '2026-09-19', {...day, html: old})).toBeNull()
	})
})
