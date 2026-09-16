import {describe, it, expect} from 'vitest'
import {parseProgressNote, sortProgressNotes} from './progressNotes'
const daily = (date: string, progress: string, outstanding = '') => `<h3>每日进展 · ${date}</h3><p>${progress}</p>${outstanding ? `<p><strong>遗留问题 / 下一步</strong></p><p>${outstanding}</p>` : ''}`
describe('daily progress display', () => {
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
