import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {mount} from '@vue/test-utils'

import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import TaskCollaborationMembers from './TaskCollaborationMembers.vue'

vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: false}))

beforeEach(() => setActivePinia(createPinia()))

describe('TaskCollaborationMembers', () => {
	it('shows the total below the task name and expands the member list', async () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			username: 'current',
			bindings: [{root_task_id: 42, task_ids: [], owner: 'owner', members: ['teammate']}],
			profiles: [],
			conflicts: [],
			notifications: [],
		}
		const wrapper = mount(TaskCollaborationMembers, {
			props: {taskId: 42},
			global: {stubs: {Icon: true}},
		})

		expect(wrapper.get('button').text()).toContain('3 人协作')
		expect(wrapper.find('ul').exists()).toBe(false)
		await wrapper.get('button').trigger('click')
		expect(wrapper.findAll('li > span:last-child').map(item => item.text())).toEqual(['owner', 'current', 'teammate'])
	})
})