import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import ProjectProgressOverview from './ProjectProgressOverview.vue'
import {clearProjectProgressCache} from '@/helpers/projectProgressCache'

const mocks = vi.hoisted(() => ({list: vi.fn(), comments: vi.fn()}))
vi.mock('@/client/generated', () => ({projectTasksList: mocks.list, taskCommentsList: mocks.comments}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: false}))
vi.mock('vue-router', () => ({
	useRoute: () => ({name: 'project.view', fullPath: '/projects/1/1'}),
	useRouter: () => ({push: vi.fn()}),
}))
vi.mock('@vueuse/core', async importOriginal => {
	const {onMounted} = await import('vue')
	return {...await importOriginal<typeof import('@vueuse/core')>(),
		useIntersectionObserver: (_: unknown, callback: (entries: {isIntersecting: boolean}[]) => void) => {
			onMounted(() => callback([{isIntersecting: true}]))
			return {stop: vi.fn()}
		},
	}
})
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({info: {username: 'me'}})}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => ({
	status: {username: 'me'}, memberRoster: [],
	memberKey: (value: string) => value.toLocaleLowerCase(),
	bindingForTask: () => undefined,
	avatarFor: () => '',
	displayNameFor: (value: string) => value,
	identityTitleFor: (value: string) => value,
})}))
vi.mock('./ProjectProgressTable.vue', () => ({default: {template: '<table><tbody><slot /></tbody></table>'}}))
vi.mock('@/components/tasks/partials/TaskCollaborationMembers.vue', () => ({default: {template: '<span />'}}))
vi.mock('@/components/tasks/partials/ReadonlyRichText.vue', () => ({default: {props: ['html'], template: '<span>{{ html }}</span>'}}))
vi.mock('@/components/tasks/partials/ProgressBacklinks.vue', () => ({default: {template: '<span />'}}))
let wrapper: VueWrapper
beforeEach(() => { clearProjectProgressCache(); localStorage.clear(); vi.clearAllMocks() })
afterEach(() => wrapper?.unmount())

describe('display history consumers', () => {
	it('reads each task once per refresh and updates edited comments without remounting rows', async () => {
		let childTitle = 'Child'
		mocks.list.mockImplementation(async () => ({data: {items: [
			{id: 11, title: 'Parent', comment_count: 1},
			{id: 12, title: childTitle, comment_count: 1, related_tasks: {parenttask: [{id: 11}]}},
		], total_pages: 1}}))
		let text = 'Before'
		const today = new Date()
		const date = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
		mocks.comments.mockImplementation(async ({path}: {path: {task: number}}) => ({data: {items: [
			{id: path.task, comment: `<h3>每日进展 · ${date}</h3><p>${text}</p><p><strong>遗留问题 / 下一步</strong></p><p>Remaining</p>`},
		], total_pages: 1}}))
		wrapper = mount(ProjectProgressOverview, {props: {projectId: 1}, global: {stubs: {Icon: true, XButton: {template: '<button><slot /></button>'}}}})
		await flushPromises()
		expect(wrapper.text()).toContain('Before')
		expect(mocks.comments).toHaveBeenCalledTimes(2)
		await wrapper.get('[aria-label="按最近有进展的天数筛选事项"]').setValue('7')
		await flushPromises()
		expect(mocks.comments).toHaveBeenCalledTimes(2)
		const row = wrapper.get('[data-task-id="12"]').element
		text = 'Edited same record'
		childTitle = 'Renamed child'
		await wrapper.findAll('button').find(button => button.text() === '刷新进展')!.trigger('click')
		await flushPromises()
		expect(wrapper.text()).toContain('Edited same record')
		expect(wrapper.get('.outstanding-source').text()).toContain('Renamed child')
		expect(wrapper.text()).not.toContain('Before')
		expect(mocks.comments).toHaveBeenCalledTimes(4)
		expect(wrapper.get('[data-task-id="12"]').element).toBe(row)
	})
})
