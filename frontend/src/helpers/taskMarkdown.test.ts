import {beforeEach, describe, expect, it, vi} from 'vitest'
import {saveMarkdownDocument, saveTaskMarkdown} from './taskMarkdown'
const api = vi.hoisted(() => ({save: vi.fn(), task: vi.fn()}))
vi.mock('@/client/generated', () => ({tasktraceExportMarkdown: api.save, tasksTasktraceMarkdown: api.task}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))

describe('local Markdown export notifications', () => {
 beforeEach(() => vi.clearAllMocks())
 it('delivers the saved-file notification once with the displayed snapshot', async () => {
  const listener = vi.fn(), notice = {id: 91, name: 'tasktrace.export.completed', notification: {filename: 'report.md'}}
  window.addEventListener('tasktrace-export-notification', listener)
  try {
   api.save.mockResolvedValue({data: {path: 'D:/reports/report.md', notification: notice}})
   expect(await saveMarkdownDocument('# Displayed snapshot', 'report.md')).toBe('')
   expect(api.save).toHaveBeenCalledWith({throwOnError: true, body: {filename: 'report.md', markdown: '# Displayed snapshot'}})
   expect(listener).toHaveBeenCalledTimes(1)
   expect(listener.mock.calls[0][0].detail).toEqual(notice)
  } finally {window.removeEventListener('tasktrace-export-notification', listener)}
 })
 it('notifies write failure with the cause, never a folder destination', async () => {
  const listener = vi.fn(), notice = {id: 92, name: 'tasktrace.export.failed', notification: {filename: 'report.md', error: 'Access denied'}}
  window.addEventListener('tasktrace-export-notification', listener)
  try {
   api.save.mockResolvedValue({data: {error: 'Access denied', notification: notice}})
   await expect(saveMarkdownDocument('# Review', 'report.md')).rejects.toThrow('Access denied')
   expect(listener).toHaveBeenCalledTimes(1)
   expect(listener.mock.calls[0][0].detail.notification).not.toHaveProperty('path')
  } finally {window.removeEventListener('tasktrace-export-notification', listener)}
 })
 it('keeps cancel silent', async () => {
  const listener = vi.fn()
  window.addEventListener('tasktrace-export-notification', listener)
  try {
   api.save.mockResolvedValue({data: {cancelled: true}})
   await expect(saveMarkdownDocument('# Review', 'report.md')).rejects.toMatchObject({name: 'AbortError'})
   expect(listener).not.toHaveBeenCalled()
  } finally {window.removeEventListener('tasktrace-export-notification', listener)}
 })
 it('uses the same native save flow for task exports and preserves warnings', async () => {
  api.task.mockResolvedValue({data: {markdown: '# Task', filename: 'Task.md', warnings: ['Some images unavailable']}})
  api.save.mockResolvedValue({data: {path: 'D:/Task.md', warning: 'Notification unavailable'}})
  expect(await saveTaskMarkdown(12, 'Task')).toEqual(['Some images unavailable', 'Notification unavailable'])
  expect(api.save).toHaveBeenCalledWith({throwOnError: true, body: {filename: 'Task.md', markdown: '# Task'}})
 })
})
