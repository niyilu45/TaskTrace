<template>
	<div
		ref="root"
		class="progress-date-picker"
	>
		<button
			:id="id"
			type="button"
			class="input progress-date-picker__trigger"
			:disabled="disabled"
			aria-haspopup="dialog"
			:aria-expanded="open"
			@click="toggle"
			@keydown.escape.stop="open = false"
		>
			<span>{{ modelValue }}</span>
			<Icon icon="calendar" />
		</button>
		<input
			:value="modelValue"
			class="is-sr-only"
			type="date"
			tabindex="-1"
			aria-hidden="true"
			:disabled="disabled"
			@change="emitValue(($event.target as HTMLInputElement).value)"
		>
		<div
			v-if="open"
			class="progress-date-picker__popup"
			role="dialog"
			aria-label="选择记录日期"
			@keydown.escape.stop="open = false"
		>
			<div class="progress-date-picker__months">
				<CalendarMonth
					v-for="offset in [-1, 0, 1]"
					:key="dateKey(monthAt(offset))"
					:selected="selected"
					:marked-dates="markedDates"
					:view-date="monthAt(offset)"
					:navigation="offset === -1 ? 'previous' : offset === 1 ? 'next' : 'none'"
					hide-outside-days
					@navigate="moveMonths"
					@pick="pick"
				/>
			</div>
			<p class="progress-date-picker__legend">
				<span aria-hidden="true" />已有进展
			</p>
		</div>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, useTemplateRef, watch} from 'vue'
import CalendarMonth from '@/components/input/datepicker/CalendarMonth.vue'

const props = withDefaults(defineProps<{
	id: string
	modelValue: string
	markedDates?: string[]
	disabled?: boolean
}>(), {
	markedDates: () => [],
	disabled: false,
})
const emit = defineEmits<{'update:modelValue': [value: string]}>()
const open = ref(false)
const root = useTemplateRef<HTMLElement>('root')
const selected = computed(() => /^\d{4}-\d{2}-\d{2}$/.test(props.modelValue) ? new Date(`${props.modelValue}T00:00:00`) : null)
const centerMonth = ref(monthStart(new Date()))

function monthStart(value: Date) { return new Date(value.getFullYear(), value.getMonth(), 1) }
function monthAt(offset: number) { return new Date(centerMonth.value.getFullYear(), centerMonth.value.getMonth() + offset, 1) }
function moveMonths(delta: number) { centerMonth.value = monthAt(delta) }
function toggle() {
	if (!open.value) centerMonth.value = monthStart(selected.value ?? new Date())
	open.value = !open.value
}

function dateKey(value: Date) {
	return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`
}
function emitValue(value: string) {
	if (/^\d{4}-\d{2}-\d{2}$/.test(value)) emit('update:modelValue', value)
}
function pick(value: Date) { emitValue(dateKey(value)); open.value = false }
function outside(event: PointerEvent) { if (!root.value?.contains(event.target as Node)) open.value = false }
watch(selected, value => { if (!open.value && value) centerMonth.value = monthStart(value) }, {immediate: true})
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
</script>

<style scoped lang="scss">
.progress-date-picker {
	position: relative;
	max-inline-size: 20rem;
}
.progress-date-picker__trigger {
	display: flex;
	align-items: center;
	justify-content: space-between;
	inline-size: 100%;
	font-variant-numeric: tabular-nums;
	background: var(--white);
	cursor: pointer;
}
.progress-date-picker__popup {
	position: fixed;
	z-index: 20;
	inset-block-start: max(4.5rem, 12vh);
	inset-inline-start: 50%;
	inline-size: min(60rem, calc(100vw - 2rem));
	max-block-size: calc(100vh - 6rem);
	padding: .75rem;
	border: 1px solid var(--grey-200);
	border-radius: $radius;
	background: var(--white);
	box-shadow: var(--shadow-md);
	transform: translateX(-50%);
	overflow: auto;
}
.progress-date-picker__months {
	display: grid;
	grid-template-columns: repeat(3, minmax(0, 1fr));
	gap: 1rem;

	:deep(.calendar-month) {
		min-inline-size: 0;
	}
}
.progress-date-picker__legend {
	display: flex;
	align-items: center;
	justify-content: center;
	gap: .4rem;
	margin: .15rem 0 .25rem;
	font-size: .8rem;
	color: var(--grey-600);

	span {
		inline-size: .4rem;
		block-size: .4rem;
		border-radius: 50%;
		background: var(--primary);
	}
}

@media (width <= 52rem) {
	.progress-date-picker__popup {
		inset-block-start: 4rem;
	}
	.progress-date-picker__months {
		gap: .35rem;
	}
	.progress-date-picker__months :deep(.calendar-month) {
		--calendar-day-size: clamp(.95rem, calc((100vw - 6rem) / 21), 1.85rem);
	}
	.progress-date-picker__months :deep(.calendar-month__nav) {
		grid-template-columns: 1.25rem minmax(0, 1fr) 1.25rem;
	}
	.progress-date-picker__months :deep(.calendar-month__nav-button),
	.progress-date-picker__months :deep(.calendar-month__nav-spacer) {
		inline-size: 1.25rem;
		block-size: 1.5rem;
	}
	.progress-date-picker__months :deep(.calendar-month__title) {
		font-size: .75rem;
	}
	.progress-date-picker__months :deep(.calendar-month__weekday),
	.progress-date-picker__months :deep(.calendar-month__day) {
		font-size: .62rem;
	}
}
</style>
