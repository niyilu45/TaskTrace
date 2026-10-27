import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import {reactive} from 'vue'

import TasktraceTeamCenter from './TasktraceTeamCenter.vue'

const mocks = vi.hoisted(() => ({store: {} as any, success: vi.fn(), error: vi.fn()}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => mocks.store}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/composables/useVisiblePolling', () => ({useVisiblePolling: vi.fn()}))
vi.mock('@/message', () => ({success: mocks.success, error: mocks.error, getErrorText: (cause: Error) => cause.message}))

let wrapper: VueWrapper | undefined
const candidate = {username: 'new-member', account_name: 'DOMAIN\\new-member', display_name: 'New member'}

beforeEach(() => {
	vi.clearAllMocks()
	mocks.store = reactive({
		status: {username: 'me', enabled: true, bindings: [], unassigned_members: []},
		memberRoster: ['me'],
		memberKey: (name: string) => name.toLowerCase(),
		displayNameFor: (name: string) => name,
		maximumAccessFor: () => 'write',
		refresh: vi.fn().mockResolvedValue({}),
		reconcileMemberAccess: vi.fn().mockResolvedValue([]),
		grantMemberWithConsent: vi.fn(),
		setMemberAccessWithConsent: vi.fn(),
		removeMemberWithConsent: vi.fn(),
	})
})

afterEach(() => wrapper?.unmount())

function open() {
	wrapper = mount(TasktraceTeamCenter, {global: {stubs: {
		XButton: {props: ['disabled', 'loading'], template: '<button :disabled="disabled || loading"><slot /></button>'},
		TeamMemberPicker: {template: '<button class="pick-member" @click="$emit(\'select\', candidate)">Choose</button>', data: () => ({candidate})},
		TeamMemberIdentity: {props: ['username'], template: '<span>{{ username }}</span>'},
		Modal: true,
		TeamConflictResolver: true,
	}}})
	return wrapper
}

async function add() {
	await wrapper!.findAll('button').find(button => button.text() === '添加协作成员')!.trigger('click')
	await wrapper!.get('.pick-member').trigger('click')
	await flushPromises()
}

describe('team member access feedback', () => {
	it('reconciles existing collaboration members after loading the team page', async () => {
		mocks.store.reconcileMemberAccess.mockResolvedValue(['existing-member'])
		open()
		await flushPromises()
		expect(mocks.store.refresh).toHaveBeenCalledWith(true, true)
		expect(mocks.store.reconcileMemberAccess).toHaveBeenCalledOnce()
		expect(wrapper!.get('[role="status"]').text()).toContain('1 位已有协作成员')
	})

	it('blocks other permission changes only while existing member rights are being repaired', async () => {
		let finish!: (value: string[]) => void
		mocks.store.status.repository = {pending_members: [{account_name: 'member', access: 'write'}]}
		mocks.store.status.unassigned_members = ['member']
		mocks.store.memberRoster.push('member')
		mocks.store.reconcileMemberAccess.mockImplementation(() => new Promise(resolve => { finish = resolve }))
		open()
		await flushPromises()
		expect(wrapper!.get('[role="status"]').text()).toContain('正在补全')
		expect(wrapper!.findAll('button').find(button => button.text() === '添加协作成员')!.attributes('disabled')).toBeDefined()
		expect(wrapper!.get('select[id="team-access-member"]').attributes('disabled')).toBeDefined()
		expect(wrapper!.findAll('button').find(button => button.text() === '删除权限')!.attributes('disabled')).toBeDefined()
		finish(['member'])
		await flushPromises()
		expect(wrapper!.findAll('button').find(button => button.text() === '添加协作成员')!.attributes('disabled')).toBeUndefined()
		expect(wrapper!.get('[role="status"]').text()).toContain('已为 1 位')
	})

	it('shows a successful addition and the new row without leaving the page', async () => {
		mocks.store.grantMemberWithConsent.mockImplementation(async () => {
			mocks.store.memberRoster.push(candidate.account_name)
			mocks.store.status.unassigned_members.push(candidate.account_name)
			return mocks.store.status
		})
		open()
		await flushPromises()
		await add()
		expect(mocks.store.grantMemberWithConsent).toHaveBeenCalledWith(candidate)
		expect(wrapper!.get('[role="status"]').text()).toContain('已成功添加')
		expect(wrapper!.findAll('.team-unassigned-row').map(row => row.text()).join(' ')).toContain(candidate.account_name)
		expect(mocks.success).toHaveBeenCalledOnce()
		expect(wrapper!.find('.pick-member').exists()).toBe(false)
	})

	it('does not claim success or show an error when permission confirmation is cancelled', async () => {
		mocks.store.grantMemberWithConsent.mockResolvedValue(false)
		open()
		await flushPromises()
		await add()
		expect(mocks.success).not.toHaveBeenCalled()
		expect(mocks.error).not.toHaveBeenCalled()
		expect(wrapper!.find('.pick-member').exists()).toBe(true)
		expect(wrapper!.find('[role="status"]').exists()).toBe(false)
	})

	it('keeps the failure visible and permits retry', async () => {
		mocks.store.grantMemberWithConsent.mockRejectedValue(new Error('共享目录无法访问'))
		open()
		await flushPromises()
		await add()
		expect(wrapper!.get('[role="alert"]').text()).toContain('共享目录无法访问')
		expect(wrapper!.find('.pick-member').exists()).toBe(true)
		expect(mocks.success).not.toHaveBeenCalled()
	})

	it('disables repeated additions while the permission request is running', async () => {
		let finish!: (value: unknown) => void
		mocks.store.grantMemberWithConsent.mockImplementation(() => new Promise(resolve => { finish = resolve }))
		open()
		await flushPromises()
		await add()
		expect(wrapper!.get('[role="status"]').text()).toContain('正在为')
		expect(wrapper!.find('.pick-member').exists()).toBe(false)
		expect(wrapper!.findAll('button').find(button => button.text() === '取消添加')!.attributes('disabled')).toBeDefined()
		finish(false)
		await flushPromises()
		expect(wrapper!.find('.pick-member').exists()).toBe(true)
	})
})
