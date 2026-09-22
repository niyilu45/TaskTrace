import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {mount, type VueWrapper} from '@vue/test-utils'
import {defineComponent} from 'vue'
import {useVisiblePolling} from './useVisiblePolling'

let wrapper: VueWrapper | undefined
let visible = true
beforeEach(() => {
	vi.useFakeTimers()
	visible = true
	vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visible ? 'visible' : 'hidden')
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.restoreAllMocks(); vi.useRealTimers() })
function start(read: () => Promise<unknown>) {
	wrapper = mount(defineComponent({setup() { useVisiblePolling(read, 15000); return () => null }}))
}
async function visibility(value: boolean) {
	visible = value
	document.dispatchEvent(new Event('visibilitychange'))
	await vi.advanceTimersByTimeAsync(0)
}
describe('visible background polling', () => {
	it('makes no calls for a hidden minute and catches up once on return', async () => {
		const read = vi.fn(async () => {})
		start(read)
		await vi.advanceTimersByTimeAsync(0)
		expect(read).toHaveBeenCalledTimes(1)
		await visibility(false)
		await vi.advanceTimersByTimeAsync(60000)
		expect(read).toHaveBeenCalledTimes(1)
		await visibility(true)
		window.dispatchEvent(new Event('focus'))
		await vi.advanceTimersByTimeAsync(0)
		expect(read).toHaveBeenCalledTimes(2)
	})
	it('does not overlap slow requests or rearm after unmount', async () => {
		let finish!: () => void
		const read = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
		start(read)
		await vi.advanceTimersByTimeAsync(60000)
		window.dispatchEvent(new Event('focus'))
		expect(read).toHaveBeenCalledTimes(1)
		wrapper!.unmount(); wrapper = undefined
		finish()
		await vi.advanceTimersByTimeAsync(60000)
		expect(read).toHaveBeenCalledTimes(1)
		expect(vi.getTimerCount()).toBe(0)
	})
	it('retries a failed read on the next interval without a tight loop', async () => {
		const read = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined)
		start(read)
		await vi.advanceTimersByTimeAsync(14999)
		expect(read).toHaveBeenCalledTimes(1)
		await vi.advanceTimersByTimeAsync(1)
		expect(read).toHaveBeenCalledTimes(2)
	})
})

describe('websocket fallback catch-up', () => {
	it('catches up once after a hidden reconnect while keeping connected interval polls disabled', async () => {
		const read = vi.fn(async () => {})
		wrapper = mount(defineComponent({setup() {
			useVisiblePolling(read, 15000, {immediate: false, enabled: () => false, catchUpOnResume: true})
			return () => null
		}}))
		await vi.advanceTimersByTimeAsync(30000)
		expect(read).not.toHaveBeenCalled()
		await visibility(false)
		await visibility(true)
		window.dispatchEvent(new Event('focus'))
		await vi.advanceTimersByTimeAsync(0)
		expect(read).toHaveBeenCalledTimes(1)
		// A real hidden/visible transition must not be suppressed by focus-event debouncing.
		await visibility(false)
		await visibility(true)
		expect(read).toHaveBeenCalledTimes(2)
		await vi.advanceTimersByTimeAsync(30000)
		expect(read).toHaveBeenCalledTimes(2)
	})
})
describe('catch-up with a pre-hidden request in flight', () => {
	it('follows a pre-hidden response with one fresh read after visibility returns', async () => {
		let finish!: () => void
		const read = vi.fn().mockImplementationOnce(() => new Promise<void>(resolve => {finish = resolve})).mockResolvedValue(undefined)
		wrapper = mount(defineComponent({setup() {useVisiblePolling(read, 15000, {catchUpOnResume: true}); return () => null}}))
		await vi.advanceTimersByTimeAsync(0)
		await visibility(false); await vi.advanceTimersByTimeAsync(60000); await visibility(true)
		window.dispatchEvent(new Event('focus'))
		expect(read).toHaveBeenCalledTimes(1)
		finish(); await vi.advanceTimersByTimeAsync(0)
		expect(read).toHaveBeenCalledTimes(2)
	})
	it('does not run queued catch-up while hidden or after unmount', async () => {
		let finish!: () => void
		const read = vi.fn(() => new Promise<void>(resolve => {finish = resolve}))
		wrapper = mount(defineComponent({setup() {useVisiblePolling(read, 15000, {catchUpOnResume: true}); return () => null}}))
		await vi.advanceTimersByTimeAsync(0)
		await visibility(false); await visibility(true); await visibility(false)
		finish(); await vi.advanceTimersByTimeAsync(60000)
		expect(read).toHaveBeenCalledTimes(1)
		await visibility(true)
		expect(read).toHaveBeenCalledTimes(2)
		await visibility(false); await visibility(true)
		wrapper!.unmount(); wrapper = undefined
		finish(); await vi.advanceTimersByTimeAsync(60000)
		expect(read).toHaveBeenCalledTimes(2)
		expect(vi.getTimerCount()).toBe(0)
	})
})