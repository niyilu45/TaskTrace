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

	it('uses the current member task permission for editing', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			username: 'reader',
			bindings: [{
				share_id: 'shared',
				root_task_id: 42,
				task_ids: [42],
				owner: 'owner',
				members: ['owner', 'reader'],
				permission_targets: [{
					node_id: 'root',
					task_id: 42,
					kind: 'task',
					permissions: [
						{username: 'owner', read: true, write: true, owner: true},
						{username: 'reader', read: true, write: false},
					],
				}],
			}],
			conflicts: [],
			notifications: [],
		}

		expect(store.canWriteTask(42)).toBe(false)
	})

	it('keeps the collaboration owner writable for a legacy manifest', () => {
		const store = useTasktraceTeamStore()
		store.status = {
			enabled: true,
			username: 'owner',
			bindings: [{share_id: 'shared', root_task_id: 42, task_ids: [42], owner: 'owner', members: ['owner', 'reader']}],
			conflicts: [],
			notifications: [],
		}

		expect(store.canWriteTask(42)).toBe(true)
	})
})
