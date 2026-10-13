import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {mount, flushPromises, type VueWrapper} from '@vue/test-utils'
import {reactive} from 'vue'
import TeamCollaboration from './TeamCollaboration.vue'

const mocks = vi.hoisted(() => ({store: {} as any}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => mocks.store}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/helpers/time/formatDate', () => ({formatDisplayDate: (value: string) => value}))
vi.mock('@/message', () => ({error: vi.fn(), success: vi.fn(), getErrorText: (error: Error) => error.message}))
let wrapper: VueWrapper | undefined
beforeEach(() => {
 const binding = {share_id: 'shared', root_task_id: 1, members: ['me', 'member'], last_sync: '2026-10-13T00:00:00Z', last_error: ''}
 mocks.store = reactive({
  status: {enabled: true, username: 'me', bindings: [binding]}, loaded: true, loading: false, memberRoster: ['me', 'member'],
  bindingForTask: () => mocks.store.status.bindings[0], uniqueMembers: (members: string[]) => members, memberKey: (name: string) => name,
  avatarFor: () => '', refresh: vi.fn().mockResolvedValue({}), sync: vi.fn(),
 })
 mocks.store.sync.mockImplementation(async () => mocks.store.status)
 wrapper = mount(TeamCollaboration, {props: {taskId: 1, canWrite: true}, global: {stubs: {
  Icon: true, TeamMemberIdentity: true, TeamMemberPicker: true, TeamPermissionEditor: true,
  XButton: {props: ['loading', 'disabled'], template: '<button :disabled="disabled || loading"><slot /></button>'},
 }}})
})
afterEach(() => wrapper?.unmount())
async function syncButton() {
 await wrapper!.get('.team-collaboration__toggle').trigger('click')
 await flushPromises()
 return wrapper!.findAll('button').find(button => button.text() === '立即同步')!
}
describe('explicit collaboration synchronization', () => {
 it('can sync without opening permissions and reports completion', async () => {
  await (await syncButton()).trigger('click'); await flushPromises()
  expect(mocks.store.sync).toHaveBeenCalledTimes(1)
  expect(wrapper!.get('[role="status"]').text()).toContain('本机与共享目录同步完成')
 })
 it('shows per-task failures from successful HTTP responses instead of reporting success', async () => {
  mocks.store.sync.mockImplementation(async () => {
   mocks.store.status.bindings[0].last_error = 'read member updates: denied'
   return mocks.store.status
  })
  await (await syncButton()).trigger('click'); await flushPromises()
  expect(wrapper!.get('[role="status"]').text()).toContain('read member updates: denied')
  await wrapper!.get('.team-collaboration__toggle').trigger('click')
  expect(wrapper!.get('.team-collaboration__toggle').text()).toContain('同步异常')
 })
 it('allows retry after a network failure', async () => {
  mocks.store.sync.mockRejectedValueOnce(new Error('connection timed out'))
  const button = await syncButton()
  await button.trigger('click'); await flushPromises()
  expect(wrapper!.get('[role="status"]').text()).toContain('connection timed out')
  expect(button.attributes('disabled')).toBeUndefined()
  await button.trigger('click'); await flushPromises()
  expect(wrapper!.get('[role="status"]').text()).toContain('同步完成')
 })
 it('does not report a stale result after changing tasks during synchronization', async () => {
  let finish!: (value: unknown) => void
  mocks.store.sync.mockImplementation(() => new Promise(resolve => {finish = resolve}))
  await (await syncButton()).trigger('click')
  await wrapper!.setProps({taskId: 2})
  finish(mocks.store.status); await flushPromises()
  await wrapper!.get('.team-collaboration__toggle').trigger('click')
  expect(wrapper!.find('[role="status"]').exists()).toBe(false)
 })
})
