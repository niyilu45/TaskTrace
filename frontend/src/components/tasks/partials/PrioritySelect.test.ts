import {beforeEach, describe, expect, it, vi} from 'vitest'
import {mount} from '@vue/test-utils'
import PrioritySelect from './PrioritySelect.vue'
import PriorityLabel from './PriorityLabel.vue'

const local = vi.hoisted(() => ({value: true}))
vi.mock('@/helpers/tasktraceLocal', () => ({get isLocalBuild() { return local.value }}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({settings: {frontendSettings: {minimumPriority: 2}}})}))
const global = {mocks: {$t: (key: string) => key}, stubs: {Icon: true}}
beforeEach(() => { local.value = true })

describe('TaskTrace local priorities', () => {
	it('shows only 0 through 9, renders an unset task as 9 and stores chosen numbers in reverse order', async () => {
		const wrapper = mount(PrioritySelect, {props: {modelValue: 0}, global})
		expect(wrapper.findAll('option').map(option => option.attributes('value'))).toEqual(Array.from({length: 10}, (_, index) => String(index)))
		expect(wrapper.get('select').element.value).toBe('9')
		expect(wrapper.emitted('update:modelValue')).toBeUndefined()
		await wrapper.get('select').setValue('0')
		expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([10])
		await wrapper.get('select').setValue('9')
		expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([1])
		wrapper.unmount()
	})

	it('shows numeric labels for all local priorities and gives legacy unset a default of 9', () => {
		for (let raw = 0; raw <= 10; raw++) {
			const wrapper = mount(PriorityLabel, {props: {priority: raw, showAll: true}, global})
			expect(wrapper.text()).toBe(`优先级 ${raw === 0 ? 9 : 10 - raw}`)
			wrapper.unmount()
		}
	})

	it('retains default label visibility and completed-task behavior', () => {
		const hiddenDefault = mount(PriorityLabel, {props: {priority: 0}, global})
		expect(hiddenDefault.find('.priority-label').exists()).toBe(false)
		hiddenDefault.unmount()
		const completed = mount(PriorityLabel, {props: {priority: 10, showAll: true, done: true}, global})
		expect(completed.find('.priority-label').exists()).toBe(false)
		completed.unmount()
	})

	it('preserves upstream options, emitted values and labels outside the local build', async () => {
		local.value = false
		const select = mount(PrioritySelect, {props: {modelValue: 0}, global})
		expect(select.findAll('option').map(option => option.attributes('value'))).toEqual(['0', '1', '2', '3', '4', '5'])
		expect(select.text()).toContain('task.priority.unset')
		await select.get('select').setValue('3')
		expect(select.emitted('update:modelValue')?.at(-1)).toEqual([3])
		const label = mount(PriorityLabel, {props: {priority: 3}, global})
		expect(label.text()).toBe('task.priority.high')
		label.unmount()
		select.unmount()
	})
})
