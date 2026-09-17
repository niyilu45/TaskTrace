import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils'
import AddTask from './AddTask.vue'
import {undoBlockReason} from '@/helpers/tasktraceUndo'

const taskStore = vi.hoisted(() => ({isLoading: false, ensureLabelsExist: vi.fn(async () => []), createNewTasksBulk: vi.fn()}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('vue-i18n', () => ({useI18n: () => ({t: (key: string) => key})}))
vi.mock('vue-router', () => ({useRouter: () => ({currentRoute: {value: {params: {projectId: '1'}}}})}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({settings: {defaultProjectId: 1, frontendSettings: {quickAddMagicMode: 0}}})}))
vi.mock('@/stores/config', () => ({useConfigStore: () => ({concurrentWrites: 1})}))
vi.mock('@/stores/tasks', () => ({useTaskStore: () => taskStore}))
vi.mock('@/components/tasks/partials/QuickAddMagic.vue', () => ({default: {template: '<span />'}}))
vi.mock('@/helpers/parseSubtasksViaIndention', () => ({parseSubtasksViaIndention: (title: string) => [{title, project: null, parent: null}]}))
vi.mock('@/modules/quickAddMagic', () => ({getLabelsFromPrefix: () => []}))
vi.mock('@/services/taskRelation', () => ({default: class {}}))
vi.mock('@/models/taskRelation', () => ({default: class {constructor(data: object) {Object.assign(this, data)}}}))
vi.mock('@/message', () => ({error: vi.fn()}))
vi.mock('@/client/generated', () => ({tasksRelationsCreate: vi.fn()}))
let wrapper: VueWrapper
beforeEach(() => { vi.clearAllMocks() })
afterEach(() => { wrapper?.unmount() })

describe('new task Undo protection', () => {
	it('keeps global Undo blocked while a draft is filled or its creation is pending', async () => {
		let finish!: (result: unknown) => void
		taskStore.createNewTasksBulk.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
		wrapper = mount(AddTask, {global: {mocks: {$t: (key: string) => key}, directives: {focus: () => {}}, stubs: {Icon: true, Expandable: true, XButton: {template: '<button><slot /></button>'}}}})
		await wrapper.get('textarea').setValue('Do not discard this draft')
		expect(undoBlockReason.value).toContain('新任务草稿')
		await wrapper.get('textarea').setValue('')
		expect(undoBlockReason.value).toBe('')
		await wrapper.get('textarea').setValue('Create me')
		await wrapper.get('button').trigger('click')
		await flushPromises()
		expect(wrapper.get('textarea').element.value).toBe('')
		expect(undoBlockReason.value).toContain('新任务草稿')
		finish({tasks: [{id: 10, title: 'Create me'}], error: null})
		await flushPromises()
		expect(undoBlockReason.value).toBe('')
	})
})
