import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {mount} from '@vue/test-utils'
import {ref, nextTick, defineComponent} from 'vue'
import {isUndoTextTarget, isUndoMutation, undoGroupHeaders, isUndoSafeRoute, useTasktraceUndoGuard, undoBlockReason, undoPendingWrites, undoInProgress, beginUndoWrite, finishUndoWrite, setUndoRefreshHandler} from './tasktraceUndo'
const local = vi.hoisted(() => ({value: true}))
vi.mock('@/helpers/tasktraceLocal', () => ({get isLocalBuild() {return local.value}}))

beforeEach(() => { local.value = true; undoPendingWrites.value = 0; undoInProgress.value = false })
afterEach(() => { setUndoRefreshHandler(); vi.useRealTimers() })

describe('TaskTrace undo guards', () => {
	it('leaves undo inside native fields and deeply nested rich text to the editor', () => {
		for (const tag of ['input', 'textarea', 'select']) expect(isUndoTextTarget(document.createElement(tag))).toBe(true)
		const editor = document.createElement('div')
		editor.contentEditable = 'true'
		editor.innerHTML = '<p><strong><span>Draft</span></strong></p>'
		expect(isUndoTextTarget(editor.querySelector('span'))).toBe(true)
		expect(isUndoTextTarget(document.createElement('button'))).toBe(false)
	})

	it('blocks undo synchronously for dirty editors and clears the guard on unmount', async () => {
		const dirty = ref(false)
		const wrapper = mount(defineComponent({setup() {useTasktraceUndoGuard(dirty, 'Save first'); return () => null}}))
		expect(undoBlockReason.value).toBe('')
		dirty.value = true
		expect(undoBlockReason.value).toBe('Save first')
		dirty.value = false
		await nextTick()
		expect(undoBlockReason.value).toBe('')
		dirty.value = true
		wrapper.unmount()
		expect(undoBlockReason.value).toBe('')
	})

	it('warns before refreshing a page with an unsaved editor', () => {
		const dirty = ref(true)
		const wrapper = mount(defineComponent({setup() {useTasktraceUndoGuard(dirty, 'Save first'); return () => null}}))
		const event = new Event('beforeunload', {cancelable: true})
		window.dispatchEvent(event)
		expect(event.defaultPrevented).toBe(true)
		wrapper.unmount()
	})

	it('tracks pending writes exactly once and refreshes server status after settling', async () => {
		vi.useFakeTimers()
		const refresh = vi.fn(async () => {})
		setUndoRefreshHandler(refresh)
		const key = {}
		beginUndoWrite(key); beginUndoWrite(key)
		expect(undoPendingWrites.value).toBe(1)
		expect(undoBlockReason.value).toContain('正在保存')
		finishUndoWrite(key); finishUndoWrite(key)
		expect(undoPendingWrites.value).toBe(0)
		await vi.advanceTimersByTimeAsync(100)
		expect(refresh).toHaveBeenCalledTimes(1)
	})

	it('does not require secure-context UUID APIs in upstream HTTP builds', () => {
		local.value = false
		const uuid = vi.spyOn(crypto, 'randomUUID').mockImplementation(() => { throw new Error('unavailable') })
		expect(undoGroupHeaders()).toEqual({})
		expect(uuid).not.toHaveBeenCalled()
		uuid.mockRestore()
	})

	it('limits remounting to guarded task/list routes', () => {
		expect(isUndoSafeRoute('task.detail')).toBe(true)
		expect(isUndoSafeRoute('project.view')).toBe(true)
		expect(isUndoSafeRoute('user.settings.general')).toBe(false)
		expect(isUndoSafeRoute('project.settings.edit')).toBe(false)
	})

	it('only opts in to local task mutations and never standalone image uploads or reads', () => {
		for (const path of ['/api/v2/tasks/5', '/api/v1/tasks/5/comments', '/api/v2/projects/2/tasks', '/tasks/5/outstanding/move']) expect(isUndoMutation('POST', path)).toBe(true)
		for (const path of ['/api/v2/tasktrace/undo', '/api/v2/tasks/5/attachments', '/api/v2/tasks/5/read', '/api/v2/user/settings/general']) expect(isUndoMutation('POST', path)).toBe(false)
		expect(isUndoMutation('GET', '/api/v2/tasks/5')).toBe(false)
		local.value = false
		expect(isUndoMutation('PATCH', '/api/v2/tasks/5')).toBe(false)
	})
})
