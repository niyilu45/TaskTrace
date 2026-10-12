import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import {reactive, nextTick} from 'vue'
import TeamConflictResolver from './TeamConflictResolver.vue'
import type {TaskTraceTeamConflict} from '@/client/generated'

const mocks = vi.hoisted(() => ({store: undefined as any}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => mocks.store}))
vi.mock('@/message', () => ({getErrorText: (error: Error) => error.message}))
let wrapper: VueWrapper | undefined
const first = {id: 'a', share_id: 'share', task_title: '任务甲', field: 'title', base: '原任务名', options: [{author: 'Alice', value: '新任务名'}]}
const second = {id: 'b', share_id: 'share', task_title: '任务乙', field: 'done', base: 'false', options: [{author: 'Bob', value: 'true'}]}
beforeEach(() => {
	mocks.store = reactive({status: {conflicts: [first, second]}, refresh: vi.fn().mockResolvedValue({}), resolve: vi.fn(), displayNameFor: (name: string) => name})
	mocks.store.resolve.mockImplementation(async (_share: string, resolutions: {conflict_id: string}[]) => {
		mocks.store.status.conflicts = mocks.store.status.conflicts.filter((item: TaskTraceTeamConflict) => item.id !== resolutions[0].conflict_id)
	})
	wrapper = mount(TeamConflictResolver, {global: {stubs: {Modal: {template: '<div><slot /></div>'}, ReadonlyRichText: true}}})
})
afterEach(() => wrapper?.unmount())

describe('resolve one conflict at a time', () => {
	it('requires a choice and saves only one item, leaving the next for review', async () => {
		expect(wrapper!.get('.is-primary').attributes('disabled')).toBeDefined()
		await wrapper!.findAll('input[type="radio"]')[1].setValue(true)
		await wrapper!.get('.is-primary').trigger('click'); await flushPromises()
		expect(mocks.store.resolve).toHaveBeenCalledWith('share', [{conflict_id: 'a', value: '新任务名'}])
		expect(wrapper!.get('.conflict-context h3').text()).toContain('任务乙')
		expect(wrapper!.get('.is-primary').attributes('disabled')).toBeDefined()
		expect(mocks.store.status.conflicts).toHaveLength(1)
	})
	it('skip does not write anything', async () => {
		await wrapper!.get('.conflict-footer .button:not(.is-primary)').trigger('click')
		expect(mocks.store.resolve).not.toHaveBeenCalled()
		expect(wrapper!.get('.conflict-context h3').text()).toContain('任务乙')
	})
	it('retains the chosen version after a save fails', async () => {
		mocks.store.resolve.mockRejectedValueOnce(new Error('共享目录无法连接，请重试'))
		await wrapper!.findAll('input[type="radio"]')[1].setValue(true)
		await wrapper!.get('.is-primary').trigger('click'); await flushPromises()
		expect(wrapper!.get('[role="alert"]').text()).toContain('共享目录')
		expect((wrapper!.findAll('input')[1].element as HTMLInputElement).checked).toBe(true)
		expect(mocks.store.status.conflicts).toHaveLength(2)
	})
	it('does not submit a stale conflict after background resolution', async () => {
		mocks.store.refresh.mockImplementation(async () => {mocks.store.status.conflicts = [second]})
		await wrapper!.findAll('input[type="radio"]')[1].setValue(true)
		await wrapper!.get('.is-primary').trigger('click'); await flushPromises(); await nextTick()
		expect(mocks.store.resolve).not.toHaveBeenCalled()
		expect(wrapper!.text()).toContain('已被更新或处理')
	})
})
