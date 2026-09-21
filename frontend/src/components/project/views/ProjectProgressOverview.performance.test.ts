import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import ProjectProgressOverview from './ProjectProgressOverview.vue'
import {clearProjectProgressCache} from '@/helpers/projectProgressCache'

const mocks = vi.hoisted(() => ({
	list: vi.fn(),
	refresh: vi.fn(),
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
vi.mock('./ProjectProgressRow.vue', () => ({default: {props: ['task'], template: '<div class="progress-row-stub">{{ task.title }}</div>'}}))
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
