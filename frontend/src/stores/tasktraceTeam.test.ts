import {beforeEach, describe, expect, it} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'

import {useTasktraceTeamStore} from './tasktraceTeam'

beforeEach(() => setActivePinia(createPinia()))

describe('TaskTrace team bindings', () => {
	it('finds a legacy root binding even when task_ids is empty', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [], owner: 'owner', members: ['teammate']}],
			conflicts: [],
			notifications: [],
		}

		expect(store.bindingForTask(42)?.share_id).toBe('shared')
	})

	it('finds descendants from task_ids', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42, 43], owner: 'owner', members: ['owner', 'teammate']}],
			conflicts: [],
			notifications: [],
		}

		expect(store.bindingForTask(43)?.share_id).toBe('shared')
	})
})