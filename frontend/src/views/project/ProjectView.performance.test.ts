import {describe, expect, it, vi} from 'vitest'
import {mount} from '@vue/test-utils'
import ProjectView from './ProjectView.vue'

const mocks = vi.hoisted(() => ({get: vi.fn(() => new Promise(() => {}))}))

vi.mock('@/services/project', () => ({default: class {
	loading = true
	get = mocks.get
}}))
vi.mock('vue-router', () => ({
	useRoute: () => ({query: {mode: 'browse'}, params: {projectId: '1'}}),
	useRouter: () => ({replace: vi.fn()}),
}))
vi.mock('@/stores/base', () => ({useBaseStore: () => ({handleSetCurrentProject: vi.fn(), setCurrentProjectViewId: vi.fn()})}))
vi.mock('@/stores/projects', () => ({useProjectStore: () => ({projects: {1: {id: 1, title: '项目一', views: [], maxPermission: 2}}, setProject: vi.fn()})}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({authenticated: false, settings: {frontendSettings: {}}})}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/helpers/projectView', () => ({saveProjectView: vi.fn()}))
vi.mock('@/modules/projectHistory', () => ({saveProjectToHistory: vi.fn()}))
vi.mock('@/components/project/views/ProjectProgressOverview.vue', () => ({default: {template: '<div class="overview-probe">overview</div>'}}))
vi.mock('@/components/project/views/ProjectList.vue', () => ({default: {template: '<div />'}}))
vi.mock('@/components/project/views/ProjectGantt.vue', () => ({default: {template: '<div />'}}))
vi.mock('@/components/project/views/ProjectTable.vue', () => ({default: {template: '<div />'}}))
vi.mock('@/components/project/views/ProjectKanban.vue', () => ({default: {template: '<div />'}}))
vi.mock('@/components/project/TasktraceTeamImportButton.vue', () => ({default: {template: '<button />'}}))

describe('project display mounting', () => {
	it('shows the display view while project metadata refreshes in the background', () => {
		const wrapper = mount(ProjectView, {props: {projectId: 1, viewId: 1}, global: {stubs: {XButton: {template: '<button><slot /></button>'}}}})
		expect(wrapper.find('.overview-probe').exists()).toBe(true)
		wrapper.unmount()
	})
})
