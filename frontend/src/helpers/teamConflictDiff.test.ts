import {describe, expect, it} from 'vitest'
import {changedText, conflictDetails, conflictVersions} from './teamConflictDiff'

describe('team conflict differences', () => {
	it('separates an outstanding note, completion and reminder from the title', () => {
		const details = conflictDetails('outstanding:item', '<span data-tasktrace-outstanding-state="true" data-done="true" data-reminder="2026-10-12T09:00:00Z" hidden></span><p>处理问题</p><aside data-tasktrace-outstanding-note hidden><p>原因</p><p>下一步</p><img src="/api/v1/tasks/1/attachments/2"></aside>')
		expect(details.find(item => item.key === 'content')?.text).toBe('处理问题')
		expect(details.find(item => item.key === 'note')?.text).toBe('原因\n下一步')
		expect(details.find(item => item.key === 'note')?.images).toContain('/attachments/2')
		expect(details.find(item => item.key === 'done')?.text).toBe('已完成')
		expect(details.find(item => item.key === 'reminder')?.text).not.toBe('未设置')
	})
	it('distinguishes deletion from an empty description', () => {
		expect(conflictDetails('outstanding:one', '')[0].text).toContain('不保留')
		expect(conflictDetails('description', '')[0].text).toBe('无')
	})
	it('marks changed text without damaging line breaks or emoji', () => {
		const diff = changedText('第一行\n等待😀确认', '第一行\n已经😀确认')
		expect(diff).toContainEqual({text: '已经', changed: true})
		expect(diff.map(part => part.text).join('')).toBe('第一行\n已经😀确认')
	})
	it('identifies different images even if their counts are equal', () => {
		const before = conflictDetails('description', '<img src="/api/v1/tasks/1/attachments/2">')[0]
		const after = conflictDetails('description', '<img src="/api/v1/tasks/1/attachments/3">')[0]
		expect(before.comparison).not.toBe(after.comparison)
	})
	it('keeps every exact raw resolution value, including deletion', () => {
		const versions = conflictVersions({field: 'title', base: 'Before', options: [{author: 'A', value: ''}]})
		expect(versions.map(item => item.value)).toEqual(['Before', ''])
	})
})
