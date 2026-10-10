import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {effectScope, nextTick, ref} from 'vue'
import {useTasktraceTaskChanges} from './useTasktraceTaskChanges'

const socket = vi.hoisted(() => ({callback: undefined as undefined | ((message: unknown) => void), unsubscribe: vi.fn()}))
vi.mock('./useWebSocket', async () => {
 const {ref} = await import('vue')
 return {useWebSocket: () => ({authenticated: ref(true), subscribe: (_: string, callback: (message: unknown) => void) => {socket.callback = callback; return socket.unsubscribe}})}
})
let scope: ReturnType<typeof effectScope>
beforeEach(() => {vi.useFakeTimers(); scope = effectScope(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)})
afterEach(() => {scope.stop(); vi.useRealTimers(); vi.restoreAllMocks()})
const changed = (id = 42) => socket.callback?.({data: {task_id: id, project_id: 1}})
describe('committed task refresh', () => {
 it('coalesces related changes and never polls idle views', async () => {
  const refresh = vi.fn(async () => {})
  scope.run(() => useTasktraceTaskChanges(refresh, change => change.task_id === 42))
  await vi.advanceTimersByTimeAsync(60000)
  expect(refresh).not.toHaveBeenCalled()
  changed(9); changed(); changed()
  await vi.advanceTimersByTimeAsync(350)
  expect(refresh).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(60000)
  expect(refresh).toHaveBeenCalledTimes(1)
 })
 it('retains changes during an edit and during an in-flight refresh', async () => {
  const blocked = ref(true)
  let finish: (() => void) | undefined
  const refresh = vi.fn(() => new Promise<void>(resolve => {finish = resolve}))
  scope.run(() => useTasktraceTaskChanges(refresh, () => true, () => blocked.value))
  changed(); await vi.advanceTimersByTimeAsync(1000)
  expect(refresh).not.toHaveBeenCalled()
  blocked.value = false; await nextTick(); await vi.advanceTimersByTimeAsync(350)
  expect(refresh).toHaveBeenCalledTimes(1)
  changed(); finish?.(); await vi.advanceTimersByTimeAsync(350)
  expect(refresh).toHaveBeenCalledTimes(2)
  finish?.(); await vi.advanceTimersByTimeAsync(1000)
  expect(refresh).toHaveBeenCalledTimes(2)
 })
 it('retries failed reads, and disposes pending work', async () => {
  const refresh = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(true)
  scope.run(() => useTasktraceTaskChanges(refresh, () => true))
  changed(); await vi.advanceTimersByTimeAsync(350)
  await vi.advanceTimersByTimeAsync(3000)
  expect(refresh).toHaveBeenCalledTimes(2)
  changed(); scope.stop(); await vi.advanceTimersByTimeAsync(60000)
  expect(refresh).toHaveBeenCalledTimes(2)
  expect(socket.unsubscribe).toHaveBeenCalled()
 })
})
