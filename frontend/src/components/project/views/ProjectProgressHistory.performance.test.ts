import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import ProjectProgressOverview from './ProjectProgressOverview.vue'
import {clearProjectProgressCache} from '@/helpers/projectProgressCache'

const mocks = vi.hoisted(() => ({list: vi.fn(), comments: vi.fn(), changed: undefined as undefined | ((message: unknown) => void)}))
vi.mock('@/client/generated', () => ({projectTasksList: mocks.list, taskCommentsList: mocks.comments}))
vi.mock('@/composables/useWebSocket', async () => {
	const {ref} = await import('vue')
	return {useWebSocket: () => ({authenticated: ref(true), subscribe: (_: string, callback: (message: unknown) => void) => {mocks.changed = callback; return () => {}}})}
})
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
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })

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


describe('large project live refresh', () => {
	it('reads and parses only the changed task while retaining its parent summary', async () => {
		vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
		const parse = vi.spyOn(DOMParser.prototype, 'parseFromString')
		const tasks = Array.from({length: 21}, (_, index) => ({id: index + 1, title: `Task ${index + 1}`, comment_count: 11,
			...(index ? {related_tasks: {parenttask: [{id: 1}]}} : {}),
		}))
		mocks.list.mockResolvedValue({data: {items: tasks, total_pages: 1}})
		let priority = 7
		mocks.comments.mockImplementation(async ({path}: {path: {task: number}}) => ({data: {items: [
			{id: 11, comment: `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="i-${path.task}" data-priority="${path.task === 2 ? priority : 7}">Remaining ${path.task}</li></ul>`},
			...Array.from({length: 10}, (_, index) => ({id: index + 1, comment: `<h3>每日进展 · 2026-10-${String(index + 1).padStart(2, '0')}</h3><p>Progress ${index}</p>`})),
		], total_pages: 1}}))
		const started = performance.now()
		wrapper = mount(ProjectProgressOverview, {props: {projectId: 1}, global: {stubs: {Icon: true, XButton: {template: '<button><slot /></button>'}}}})
		await flushPromises()
		console.log('PERF initial', {milliseconds: Math.round(performance.now() - started), requests: mocks.comments.mock.calls.length, parses: parse.mock.calls.length})
		expect(mocks.comments).toHaveBeenCalledTimes(21)
		expect(parse.mock.calls.length).toBeLessThan(950)
		mocks.comments.mockClear(); parse.mockClear()
		priority = 2
		mocks.changed?.({data: {task_id: 2, project_id: 1}})
		await vi.waitFor(() => expect(wrapper.get('[data-task-id="2"] .outstanding-cell').text()).toContain('[P2]'))
		await flushPromises()
		console.log('PERF live refresh', {requests: mocks.comments.mock.calls.length, parses: parse.mock.calls.length})
		expect(wrapper.findAll('.outstanding-source').find(entry => entry.text().includes('Remaining 2'))?.text()).toContain('[P2]')
		expect(mocks.comments).toHaveBeenCalledTimes(1)
		parse.mockRestore()
	})
})
