<template>
	<div
		class="subtask-scroll"
		tabindex="0"
		:aria-label="`${label}，可横向滚动`"
	>
		<table
			class="subtask-table"
			:style="{inlineSize: `${widths.reduce((total, width) => total + width, 0)}px`}"
		>
			<colgroup>
				<col
					v-for="(width, index) in widths"
					:key="index"
					:style="{width: `${width}px`}"
				>
			</colgroup>
			<thead>
				<tr>
					<th
						v-for="(name, index) in labels"
						:key="name"
						scope="col"
					>
						{{ name }}
						<span
							class="column-resizer"
							:class="{'is-resizing': drag?.index === index}"
							role="separator"
							tabindex="0"
							aria-orientation="vertical"
							:aria-label="`调整${name}列宽`"
							:aria-valuenow="widths[index]"
							:aria-valuemin="minimumWidth(index)"
							:aria-valuemax="MAX_WIDTH"
							:aria-valuetext="`${widths[index]} 像素`"
							title="拖动调整列宽，也可用左右方向键"
							@pointerdown="startResize($event, index)"
							@pointermove="resize"
							@pointerup="finishResize"
							@pointercancel="finishResize"
							@lostpointercapture="finishResize"
							@keydown="resizeWithKeyboard($event, index)"
						/>
					</th>
				</tr>
			</thead>
			<tbody><slot /></tbody>
		</table>
	</div>
</template>

<script setup lang="ts">
import {onBeforeUnmount, shallowRef} from 'vue'
const props = defineProps<{labels: string[], widths: number[], label: string}>()
const emit = defineEmits<{resize: [widths: number[]], resized: []}>()
// Five task levels need room for indentation, the toggle and a readable name.
function minimumWidth(index: number) { return index === 0 ? 144 : 96 }
const MAX_WIDTH = 1600
const drag = shallowRef<{index: number, x: number, widths: number[], direction: number, handle: HTMLElement, pointer: number, cursor: string, selection: string} | null>(null)
function clamp(width: number, index: number) { return Math.round(Math.min(MAX_WIDTH, Math.max(minimumWidth(index), width))) }
function direction(element: HTMLElement) { return getComputedStyle(element).direction === 'rtl' ? -1 : 1 }
function startResize(event: PointerEvent, index: number) {
	if (event.button !== 0 || drag.value) return
	event.preventDefault()
	const handle = event.currentTarget as HTMLElement
	handle.focus({preventScroll: true})
	drag.value = {index, x: event.clientX, widths: [...props.widths], direction: direction(handle), handle, pointer: event.pointerId, cursor: document.body.style.cursor, selection: document.body.style.userSelect}
	handle.setPointerCapture(event.pointerId)
	document.body.style.cursor = 'col-resize'
	document.body.style.userSelect = 'none'
}
function resize(event: PointerEvent) {
	const active = drag.value
	if (!active || active.pointer !== event.pointerId) return
	const widths = [...active.widths]
	widths[active.index] = clamp(widths[active.index] + (event.clientX - active.x) * active.direction, active.index)
	emit('resize', widths)
}
function finishResize() {
	const active = drag.value
	if (!active) return
	drag.value = null
	document.body.style.cursor = active.cursor
	document.body.style.userSelect = active.selection
	if (active.handle.hasPointerCapture(active.pointer)) active.handle.releasePointerCapture(active.pointer)
	emit('resized')
}
function resizeWithKeyboard(event: KeyboardEvent, index: number) {
	if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key) || drag.value) return
	event.preventDefault()
	event.stopPropagation()
	const widths = [...props.widths]
	const delta = (event.key === 'ArrowRight' ? 1 : -1) * direction(event.currentTarget as HTMLElement) * (event.shiftKey ? 50 : 10)
	widths[index] = event.key === 'Home' ? minimumWidth(index) : event.key === 'End' ? MAX_WIDTH : clamp(widths[index] + delta, index)
	emit('resize', widths)
	emit('resized')
}
onBeforeUnmount(finishResize)
</script>

<style scoped lang="scss">
.subtask-scroll {
	max-inline-size: 100%;
	overflow-x: auto;
}
.subtask-table {
	table-layout: fixed;
	border-collapse: collapse;
	th {
		position: relative;
		padding: .6rem .75rem;
		border-block-start: 1px solid var(--grey-200);
		background: var(--grey-100);
		font-size: .8125rem;
		text-align: start;
		overflow-wrap: anywhere;
	}
}
.column-resizer {
	position: absolute;
	inset-block: 0;
	inset-inline-end: 0;
	inline-size: 12px;
	cursor: col-resize;
	touch-action: none;
	user-select: none;
	&::after {
		position: absolute;
		inset-block: .4rem;
		inset-inline-end: 3px;
		border-inline-end: 1px solid var(--grey-500);
		content: '';
	}
	&:hover, &:focus-visible, &.is-resizing {
		background: var(--grey-200);
		&::after { border-color: var(--primary); }
	}
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: -2px;
	}
}
@media (pointer: coarse) {
	.column-resizer { inline-size: 24px; }
	.subtask-table th { padding-inline-end: 1.75rem; }
}
</style>
