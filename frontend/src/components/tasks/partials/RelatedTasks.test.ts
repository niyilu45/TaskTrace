import {beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount} from '@vue/test-utils'
import RelatedTasks from './RelatedTasks.vue'
import {tasksCreate, tasksRelationsCreate} from '@/client/generated'

vi.mock('@/client/generated', () => ({tasksCreate: vi.fn(), tasksRelationsCreate: vi.fn(), patchTasksRead: vi.fn()}))
vi.mock('@/services/task', () => ({default: class {loading = false}}))
const relationService = vi.hoisted(() => ({remove: vi.fn()}))
vi.mock('@/services/taskRelation', () => ({default: class {loading = false; delete = relationService.remove}}))
const taskStore = vi.hoisted(() => ({createNewTask: vi.fn(), delete: vi.fn()}))
vi.mock('@/stores/tasks', () => ({useTaskStore: () => taskStore}))
vi.mock('@/stores/projects', () => ({useProjectStore: () => ({projects: {}})}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({settings: {frontendSettings: {defaultTaskRelationType: 'subtask'}}})}))
vi.mock('vue-router', () => ({useRoute: () => ({name: 'task.detail', fullPath: '/tasks/42'})}))
vi.mock('vue-i18n', async original => ({...(await original<typeof import('vue-i18n')>()), useI18n: () => ({t: (key: string) => key})}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/helpers/taskHierarchyDepth', () => ({taskHierarchySpan: async () => 1, assertCanAddSubtask: async () => {}, MAX_TASK_DEPTH: 5, TASK_DEPTH_MESSAGE: '最多五级'}))
vi.mock('@/helpers/tasktraceUndo', () => ({useTasktraceUndoGuard: vi.fn(), undoGroupHeaders: () => ({})}))
vi.mock('@/message', () => ({error: vi.fn(), success: vi.fn(), getErrorText: (cause: Error) => cause.message}))

const global = {stubs: {XButton: true, Icon: true, Multiselect: {name: 'Multiselect', emits: ['create'], template: '<button class="quick-create" @click="$emit(\'create\', \'Quick child\')">Create</button>'}, FancyCheckbox: true, QuickAddMagic: true, Modal: {name: 'Modal', props: ['enabled'], emits: ['submit'], template: '<div v-if="enabled"><slot name="header" /><slot name="text" /><button class="confirm-delete" @click="$emit(\'submit\')">Confirm</button></div>'}, RouterLink: {template: '<a><slot /></a>'}}, mocks: {$t: (key: string) => key}, directives: {focus: {}, tooltip: {}}}

beforeEach(() => vi.clearAllMocks())

describe('atomic subtask creation', () => {
	it('deletes a subtask entity instead of unlinking it into a standalone task', async () => {
		taskStore.delete.mockResolvedValueOnce({})
		const child = {id: 99, title: 'Delete this child', projectId: 7, done: false}
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true, initialRelatedTasks: {subtask: [child as never]}}, global})
		await flushPromises()
		await wrapper.get('[aria-label="删除子任务：Delete this child"]').trigger('click')
		expect(wrapper.text()).toContain('子任务会被删除，不会变成独立事项')
		await wrapper.get('.confirm-delete').trigger('click')
		await flushPromises()
		expect(taskStore.delete).toHaveBeenCalledWith(expect.objectContaining({id: 99}))
		expect(relationService.remove).not.toHaveBeenCalled()
		expect(wrapper.text()).not.toContain('Delete this child')
		wrapper.unmount()
	})

	it('keeps a denied deletion in the list and displays the permission error', async () => {
		taskStore.delete.mockRejectedValueOnce(new Error('没有删除该协作任务的写权限'))
		const child = {id: 99, title: 'Keep this child', projectId: 7, done: false}
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true, initialRelatedTasks: {subtask: [child as never]}}, global})
		await flushPromises()
		await wrapper.get('[aria-label="删除子任务：Keep this child"]').trigger('click')
		await wrapper.get('.confirm-delete').trigger('click')
		await flushPromises()
		expect(wrapper.text()).toContain('Keep this child')
		expect(wrapper.text()).toContain('没有删除该协作任务的写权限')
		expect(relationService.remove).not.toHaveBeenCalled()
		wrapper.unmount()
	})

	it('labels unlinking a parent explicitly and does not delete the parent entity', async () => {
		relationService.remove.mockResolvedValueOnce({})
		const parent = {id: 41, title: 'Parent task', projectId: 7, done: false}
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true, initialRelatedTasks: {parenttask: [parent as never]}}, global})
		await flushPromises()
		await wrapper.get('[aria-label="移除关联：Parent task"]').trigger('click')
		expect(wrapper.text()).toContain('只移除所选关联，事项内容仍会保留')
		await wrapper.get('.confirm-delete').trigger('click')
		await flushPromises()
		expect(taskStore.delete).not.toHaveBeenCalled()
		expect(relationService.remove).toHaveBeenCalledWith(expect.objectContaining({taskId: 42, otherTaskId: 41, relationKind: 'parenttask'}))
		wrapper.unmount()
	})

	it('also passes the parent through quick-add relation creation without a second relation request', async () => {
		taskStore.createNewTask.mockResolvedValueOnce({id: 100, title: 'Quick child', projectId: 7})
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true}, global})
		await flushPromises()
		await wrapper.get('#showRelatedTasksFormButton').trigger('click')
		await wrapper.get('.quick-create').trigger('click')
		await flushPromises()
		expect(taskStore.createNewTask).toHaveBeenCalledWith({title: 'Quick child', projectId: 7, parentTaskId: 42})
		expect(tasksRelationsCreate).not.toHaveBeenCalled()
		expect(wrapper.text()).toContain('子任务已添加')
		wrapper.unmount()
	})

	it('creates the child and parent relation in one request', async () => {
		vi.mocked(tasksCreate).mockResolvedValueOnce({data: {id: 99, title: 'Member child'}} as never)
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true}, global})
		await flushPromises()
		await wrapper.get('#subtask-title-42').setValue('  Member child  ')
		await wrapper.get('.subtask-quick-add').trigger('submit')
		await flushPromises()
		expect(tasksCreate).toHaveBeenCalledWith(expect.objectContaining({path: {project: 7}, body: expect.objectContaining({title: 'Member child', parent_task_id: 42})}))
		expect(tasksRelationsCreate).not.toHaveBeenCalled()
		expect(wrapper.text()).toContain('子任务已添加')
		expect(wrapper.get('#subtask-title-42').element).toHaveProperty('value', '')
		wrapper.unmount()
	})

	it('preserves the title and explains permission denial without reporting partial creation', async () => {
		vi.mocked(tasksCreate).mockRejectedValueOnce(new Error('该共享任务为仅读权限，请联系所属人开放写权限。'))
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true}, global})
		await flushPromises()
		await wrapper.get('#subtask-title-42').setValue('Member child')
		await wrapper.get('.subtask-quick-add').trigger('submit')
		await flushPromises()
		expect(wrapper.text()).toContain('该共享任务为仅读权限')
		expect(wrapper.text()).not.toContain('事项已创建')
		expect(wrapper.get('#subtask-title-42').element).toHaveProperty('value', 'Member child')
		expect(tasksRelationsCreate).not.toHaveBeenCalled()
		expect(tasksCreate).toHaveBeenCalledTimes(1)
		wrapper.unmount()
	})

	it('does not apply a completed creation to another task opened meanwhile', async () => {
		let finish!: (value: never) => void
		vi.mocked(tasksCreate).mockImplementationOnce(() => new Promise<never>(resolve => { finish = resolve }))
		const wrapper = mount(RelatedTasks, {props: {taskId: 42, projectId: 7, editEnabled: true}, global})
		await flushPromises()
		await wrapper.get('#subtask-title-42').setValue('Original child')
		await wrapper.get('.subtask-quick-add').trigger('submit')
		await flushPromises()
		await wrapper.setProps({taskId: 43})
		finish({data: {id: 99, title: 'Original child'}} as never)
		await flushPromises()
		expect(wrapper.text()).not.toContain('Original child')
		expect(wrapper.text()).not.toContain('子任务已添加')
		wrapper.unmount()
	})
})
