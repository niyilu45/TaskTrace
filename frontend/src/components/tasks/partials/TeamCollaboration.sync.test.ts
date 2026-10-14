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
 const binding = {share_id: 'shared', root_task_id: 1, root_task_title: 'Shared child', task_ids: [1], owner: 'owner', repository: '\\\\server\\teamData', link: 'tasktrace-team://private-link-secret', members: ['me', 'member'], last_sync: '2026-10-13T00:00:00Z', last_error: ''}
 mocks.store = reactive({
  status: {enabled: true, username: 'me', bindings: [binding]}, loaded: true, loading: false, memberRoster: ['me', 'member'],
  bindingForTask: () => mocks.store.status.bindings[0], uniqueMembers: (members: string[]) => members, memberKey: (name: string) => name,
  avatarFor: () => '', refresh: vi.fn().mockResolvedValue({}), sync: vi.fn(),
 })
 mocks.store.sync.mockImplementation(async () => mocks.store.status)
 wrapper = mount(TeamCollaboration, {props: {taskId: 1, canWrite: true}, global: {stubs: {
  Icon: true, TeamMemberIdentity: true, TeamMemberPicker: true,
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
 it('shows a per-task failure inside the permission dialog, along with connection identity', async () => {
  mocks.store.sync.mockImplementation(async () => {
   mocks.store.status.bindings[0].last_error = 'read member updates: access denied'
   return mocks.store.status
  })
  await wrapper!.get('.team-collaboration__toggle').trigger('click')
  const button = wrapper!.findAll('button').find(button => button.text() === '编辑权限')!
  await button.trigger('click'); await flushPromises()
  const dialog = wrapper!.get('[role="dialog"]')
  expect(dialog.get('[role="alert"]').text()).toContain('read member updates: access denied')
  expect(dialog.text()).toContain('协作标识')
  expect(dialog.text()).toContain(mocks.store.status.bindings[0].repository)
  expect(dialog.text()).not.toContain('private-link-secret')
 })
 it('keeps the collaboration information visible without suggesting sync succeeded when no error is available', async () => {
  await wrapper!.get('.team-collaboration__toggle').trigger('click')
  await wrapper!.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click')
  await flushPromises()
  const dialog = wrapper!.get('[role="dialog"]')
  expect(dialog.find('[role="alert"]').exists()).toBe(false)
  expect(dialog.text()).toContain('仍保留协作关系')
  expect(dialog.text()).toContain('2026-10-13T00:00:00Z')
  expect(dialog.text()).toContain('Shared child及其子任务')
 })
 it('identifies the original child and the overlapping parent shares without merging their identities', async () => {
  mocks.store.status.bindings.push({share_id: 'parent-scope', root_task_id: 9, root_task_title: 'Later shared parent', task_ids: [9, 1], repository: '\\\\server\\teamData', last_error: 'parent share unavailable'})
  mocks.store.status.bindings.push({share_id: 'unrelated-scope', root_task_id: 20, task_ids: [20]})
  await wrapper!.get('.team-collaboration__toggle').trigger('click')
  await wrapper!.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click')
  await flushPromises()
  const details = wrapper!.get('[role="dialog"] .team-sync-details')
  expect(details.text()).toContain('Shared child及其子任务')
  expect(details.text()).toContain('Later shared parent及其子任务')
  expect(details.text()).toContain('parent-scope')
  expect(details.text()).toContain('旧子任务链接不会自动扩展到父任务')
  expect(details.text()).toContain('parent share unavailable')
  expect(details.text()).not.toContain('unrelated-scope')
 })
 it('does not open permissions for another task after an in-flight sync', async () => {
  let finish!: (value: unknown) => void
  mocks.store.sync.mockImplementation(() => new Promise(resolve => {finish = resolve}))
  await wrapper!.get('.team-collaboration__toggle').trigger('click')
  await wrapper!.findAll('button').find(button => button.text() === '编辑权限')!.trigger('click')
  await wrapper!.setProps({taskId: 2})
  finish(mocks.store.status); await flushPromises()
  expect(wrapper!.find('[role="dialog"]').exists()).toBe(false)
 })

})
