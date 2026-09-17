<template>
	<div class="tasktrace-undo">
		<BaseButton
			class="undo-button"
			:disabled="!store.canUndo"
			:title="hint"
			aria-label="撤销上一步操作"
			@click="store.undo"
		>
			<Icon icon="undo" />
			<span>撤销</span>
		</BaseButton>
		<span
			v-if="store.error"
			class="undo-error"
			role="alert"
		>{{ store.error }}</span>
		<span
			class="is-sr-only"
			aria-live="polite"
		>{{ store.message }}</span>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted} from 'vue'
import BaseButton from '@/components/base/BaseButton.vue'
import {useTasktraceUndoStore} from '@/stores/tasktraceUndo'
import {isUndoTextTarget, setUndoRefreshHandler, undoBlockReason} from '@/helpers/tasktraceUndo'
const store = useTasktraceUndoStore()
const hint = computed(() => undoBlockReason.value || (store.status.id ? `撤销：${store.status.label}（Ctrl+Z）` : '暂无可撤销操作'))
let timer: ReturnType<typeof setInterval>
function refresh() { if (!document.hidden) void store.refresh().catch(() => {}) }
function keydown(event: KeyboardEvent) {
	if (event.defaultPrevented || event.isComposing || event.repeat || event.altKey || event.shiftKey || !(event.ctrlKey || event.metaKey) || event.code !== 'KeyZ' || isUndoTextTarget(event.target)) return
	if (!store.canUndo) return
	event.preventDefault()
	void store.undo()
}
onMounted(() => {
	setUndoRefreshHandler(store.refresh)
	refresh()
	timer = setInterval(refresh, 5000)
	window.addEventListener('focus', refresh)
	window.addEventListener('keydown', keydown)
})
onBeforeUnmount(() => {
	clearInterval(timer)
	setUndoRefreshHandler()
	window.removeEventListener('focus', refresh)
	window.removeEventListener('keydown', keydown)
})
</script>

<style scoped lang="scss">
.tasktrace-undo { position: relative; }
.undo-button {
	display: inline-flex;
	align-items: center;
	gap: .4rem;
	min-block-size: 40px;
	padding-inline: .6rem;
}
.undo-error {
	position: fixed;
	inset-block-start: 4rem;
	inset-inline-end: 1rem;
	max-inline-size: min(28rem, calc(100vw - 2rem));
	padding: .75rem;
	background: var(--site-background);
	color: var(--danger-text);
	border: 1px solid var(--grey-300);
}
</style>
