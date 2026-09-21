import {beforeEach, describe, expect, it, vi} from 'vitest'
import {mount} from '@vue/test-utils'
import {createPinia, setActivePinia} from 'pinia'

import TeamMemberPicker from './TeamMemberPicker.vue'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

describe('TeamMemberPicker', () => {
	beforeEach(() => {
		setActivePinia(createPinia())
	})

	it('lists previously added collaboration members as soon as the field receives focus', async () => {
		const teamStore = useTasktraceTeamStore()
		teamStore.status = {
			enabled: true,
			username: 'current',
			repository: {candidates: ['LAN-PC\\alice']},
			unassigned_members: ['LAN-PC\\bob'],
			bindings: [{owner: 'current', members: ['carol']}],
			conflicts: [],
			notifications: [],
		}
		vi.spyOn(teamStore, 'refresh').mockResolvedValue(teamStore.status)

		const wrapper = mount(TeamMemberPicker, {
			props: {
				inputId: 'member-search',
				excluded: ['current'],
			},
		})
		await wrapper.get('input').trigger('focus')

		const options = wrapper.findAll('.team-member-option strong').map(option => option.text())
		expect(options).toEqual(expect.arrayContaining(['LAN-PC\\alice', 'LAN-PC\\bob', 'carol']))
		expect(teamStore.refresh).toHaveBeenCalledTimes(1)
	})
})
