import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import {reactive} from 'vue'

import TeamPermissionEditor from './TeamPermissionEditor.vue'

const mocks = vi.hoisted(() => ({store: {} as any, success: vi.fn(), error: vi.fn()}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => mocks.store}))
vi.mock('@/message', () => ({success: mocks.success, error: mocks.error}))
let wrapper: VueWrapper | undefined
const target = () => ({kind: 'task', node_id: 'node', task_id: 1, can_manage: true, permissions: [
	{username: 'owner', read: true, write: true, owner: true},
	{username: 'member', read: true, write: false},
]})

beforeEach(() => {
	vi.clearAllMocks()
	mocks.store = reactive({
		status: {bindings: []},
		binding: {share_id: 'share', permission_targets: [target()]},
		memberKey: (name: string) => name,
		avatarFor: () => '',
		maximumAccessFor: (name: string) => name === 'owner' ? 'write' : 'read',
		setMemberAccessWithConsent: vi.fn().mockResolvedValue({}),
		configurePermissions: vi.fn().mockResolvedValue({}),
		bindingForTask: () => mocks.store.binding,
	})
})

afterEach(() => wrapper?.unmount())

function open() {
	wrapper = mount(TeamPermissionEditor, {props: {taskId: 1, binding: mocks.store.binding}, global: {stubs: {
		XButton: {props: ['loading', 'disabled'], template: '<button :disabled="disabled || loading"><slot /></button>'},
		TeamMemberIdentity: {props: ['username'], template: '<span>{{ username }}</span>'},
		TeamSyncDetails: true,
	}}})
	return wrapper
}

async function save() {
	await wrapper!.get('input[aria-label="member 的写权限"]').setValue(true)
	await wrapper!.findAll('button').find(button => button.text() === '保存权限')!.trigger('click')
	await flushPromises()
}

describe('collaboration permission consent', () => {
	it('does not save task permissions when the user declines a required Windows permission', async () => {
		mocks.store.setMemberAccessWithConsent.mockResolvedValue(false)
		open()
		await save()
		expect(mocks.store.setMemberAccessWithConsent).toHaveBeenCalledWith('member', 'write')
		expect(mocks.store.configurePermissions).not.toHaveBeenCalled()
		expect(mocks.success).not.toHaveBeenCalled()
		expect(mocks.error).not.toHaveBeenCalled()
		expect((wrapper!.get('input[aria-label="member 的写权限"]').element as HTMLInputElement).checked).toBe(true)
	})

	it('preserves the selected task permissions while refreshing repository access', async () => {
		open()
		mocks.store.setMemberAccessWithConsent.mockImplementation(async () => {
			const nextBinding = {...mocks.store.binding, permission_targets: [target()]}
			mocks.store.binding = nextBinding
			await wrapper!.setProps({binding: nextBinding})
			return {}
		})
		await save()
		expect(mocks.store.configurePermissions).toHaveBeenCalledWith('share', 1, '', [
			{username: 'owner', read: true, write: true},
			{username: 'member', read: true, write: true},
		])
		expect(mocks.success).toHaveBeenCalledOnce()
	})

	it('keeps unsaved choices when status polling replaces the active target', async () => {
		open()
		await wrapper!.get('input[aria-label="member 的写权限"]').setValue(true)
		await wrapper!.setProps({binding: {...mocks.store.binding, permission_targets: [target()]}})
		expect((wrapper!.get('input[aria-label="member 的写权限"]').element as HTMLInputElement).checked).toBe(true)
		await wrapper!.findAll('button').find(button => button.text() === '保存权限')!.trigger('click')
		await flushPromises()
		expect(mocks.store.configurePermissions.mock.calls[0][3]).toContainEqual({username: 'member', read: true, write: true})
	})

	it('does not report success if the final task update is cancelled', async () => {
		mocks.store.configurePermissions.mockResolvedValue(false)
		open()
		await save()
		expect(mocks.success).not.toHaveBeenCalled()
		expect(mocks.error).not.toHaveBeenCalled()
	})
})
