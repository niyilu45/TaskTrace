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
			@click="open = !open"
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
			<CalendarMonth
				:selected="selected"
				:marked-dates="markedDates"
				@pick="pick"
			/>
			<p class="progress-date-picker__legend">
				<span aria-hidden="true" />已有进展
			</p>
		</div>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, useTemplateRef} from 'vue'
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

function dateKey(value: Date) {
	return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`
}
function emitValue(value: string) {
	if (/^\d{4}-\d{2}-\d{2}$/.test(value)) emit('update:modelValue', value)
}
function pick(value: Date) { emitValue(dateKey(value)); open.value = false }
function outside(event: PointerEvent) { if (!root.value?.contains(event.target as Node)) open.value = false }
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
	position: absolute;
	z-index: 20;
	inset-block-start: calc(100% + .35rem);
	inset-inline-start: 0;
	inline-size: min(20rem, calc(100vw - 2rem));
	padding: .5rem;
	border: 1px solid var(--grey-200);
	border-radius: $radius;
	background: var(--white);
	box-shadow: var(--shadow-md);
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
</style>
