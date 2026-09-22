import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import ProjectProgressOverview from './ProjectProgressOverview.vue'
import {clearProjectProgressCache} from '@/helpers/projectProgressCache'

const mocks = vi.hoisted(() => ({
	list: vi.fn(),
	refresh: vi.fn(),
	mountRow: vi.fn(),
}))

vi.mock('@/client/generated', () => ({
	projectTasksList: mocks.list,
	taskCommentsList: vi.fn(),
}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('vue-router', () => ({
	useRoute: () => ({name: 'project.view', fullPath: '/projects/1/1'}),
	useRouter: () => ({push: vi.fn()}),
}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({info: {username: 'me'}})}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => ({
	status: {username: 'me'},
	memberRoster: [],
	refresh: mocks.refresh,
	memberKey: (value: string) => value.toLocaleLowerCase(),
	bindingForTask: () => undefined,
	avatarFor: () => '',
	displayNameFor: (value: string) => value,
	identityTitleFor: (value: string) => value,
})}))
vi.mock('./ProjectProgressRow.vue', async () => {
	const {defineComponent} = await import('vue')
	return {default: defineComponent({
		props: {task: {type: Object, required: true}},
		mounted() { mocks.mountRow(this.task.id) },
		template: '<div class="progress-row-stub">{{ task.title }}</div>',
	})}
})
vi.mock('./ProjectProgressTable.vue', () => ({default: {template: '<div><slot /></div>'}}))
vi.mock('@/components/tasks/partials/TaskCollaborationMembers.vue', () => ({default: {template: '<span />'}}))

let wrapper: VueWrapper

beforeEach(() => {
	clearProjectProgressCache()
	vi.clearAllMocks()
})

afterEach(() => wrapper?.unmount())

describe('project display loading', () => {
	it('loads tasks without waiting for a slow teamData refresh', async () => {
		mocks.refresh.mockReturnValue(new Promise(() => {}))
		mocks.list.mockResolvedValue({data: {items: [{id: 11, title: '立即显示的任务', done: false, status: 'todo', comment_count: 0, related_tasks: {}}], total_pages: 1}})
		wrapper = mount(ProjectProgressOverview, {props: {projectId: 1}, global: {stubs: {Icon: true, XButton: {template: '<button><slot /></button>'}}}})

		await vi.waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1))
		await flushPromises()
		expect(wrapper.text()).toContain('立即显示的任务')
	})
})


describe('project display refresh work', () => {
	it('does not remount rows from earlier pages while subsequent pages arrive', async () => {
		let resolveSecond!: (value: unknown) => void
		mocks.list.mockResolvedValueOnce({data: {items: [{id: 11, title: 'First', comment_count: 0}], total_pages: 2}})
			.mockReturnValueOnce(new Promise(resolve => { resolveSecond = resolve }))
		wrapper = mount(ProjectProgressOverview, {props: {projectId: 1}, global: {stubs: {Icon: true, XButton: {template: '<button><slot /></button>'}}}})
		await flushPromises()
		expect(mocks.mountRow).toHaveBeenCalledTimes(1)
		resolveSecond({data: {items: [{id: 12, title: 'Second', comment_count: 0}], total_pages: 2}})
		await flushPromises()
		expect(mocks.mountRow).toHaveBeenCalledTimes(2)
	})

	it('preserves expanded details and row instances after an unchanged refresh', async () => {
		mocks.list.mockImplementation(async () => ({data: {items: [{id: 11, title: 'First', comment_count: 0}], total_pages: 1}}))
		wrapper = mount(ProjectProgressOverview, {props: {projectId: 1}, global: {stubs: {Icon: true, XButton: {template: '<button><slot /></button>'}}}})
		await flushPromises()
		const details = wrapper.get('details').element as HTMLDetailsElement
		details.open = true
		await wrapper.findAll('button').find(button => button.text() === '刷新进展')!.trigger('click')
		await flushPromises()
		expect(mocks.mountRow).toHaveBeenCalledTimes(1)
		expect(wrapper.get('details').element).toBe(details)
		expect(details.open).toBe(true)
	})
})
