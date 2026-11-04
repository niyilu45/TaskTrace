
<template>
	<div class="weekly-reminder">
		<fieldset>
			<legend>每周提醒（可多选）</legend>
			<div class="weekday-options">
				<label
					v-for="day in REMINDER_WEEKDAYS"
					:key="day.value"
				>
					<input
						type="checkbox"
						:checked="modelValue.weekdays.includes(day.value)"
						@change="toggleDay(day.value, ($event.target as HTMLInputElement).checked)"
					>
					{{ day.label }}
				</label>
			</div>
		</fieldset>
		<label>默认提醒时间<input
			class="input"
			type="time"
			:value="modelValue.time"
			@input="update('time', ($event.target as HTMLInputElement).value)"
		></label>
		<fieldset
			v-if="selectedDays.length"
			class="weekday-times"
		>
			<legend>各日提醒时间</legend>
			<div class="weekday-time-inputs">
				<label
					v-for="day in selectedDays"
					:key="day.value"
				>
					{{ day.label }}
					<input
						class="input"
						type="time"
						:value="reminderTimeOnDay(modelValue, day.value)"
						@input="updateDayTime(day.value, ($event.target as HTMLInputElement).value)"
					>
				</label>
			</div>
		</fieldset>
		<div class="date-range">
			<label>开始日期<input
				class="input"
				type="date"
				:value="modelValue.startDate"
				@input="update('startDate', ($event.target as HTMLInputElement).value)"
			></label>
			<label>结束日期<input
				class="input"
				type="date"
				:value="modelValue.endDate"
				@input="update('endDate', ($event.target as HTMLInputElement).value)"
			></label>
		</div>
		<p class="hint">
			包含开始和结束当天，按本机时间提醒。关闭一次提醒不影响后续日期。
		</p>
		<p
			v-if="error"
			role="alert"
			class="repeat-error"
		>
			{{ error }}
		</p>
	</div>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {REMINDER_WEEKDAYS, reminderTimeOnDay, repeatRuleError, type ReminderRepeatRule} from '@/helpers/reminderRepeat'
const props = defineProps<{modelValue: ReminderRepeatRule}>()
const emit = defineEmits<{'update:modelValue': [ReminderRepeatRule]}>()
const error = computed(() => repeatRuleError(props.modelValue))
const selectedDays = computed(() => REMINDER_WEEKDAYS.filter(day => props.modelValue.weekdays.includes(day.value)))

function update(key: 'time' | 'startDate' | 'endDate', value: string) {
	emit('update:modelValue', {...props.modelValue, [key]: value})
}
function toggleDay(day: number, checked: boolean) {
	const days = props.modelValue.weekdays.filter(value => value !== day)
	const times = {...props.modelValue.times}
	if (checked) days.push(day)
	else delete times[day]
	emit('update:modelValue', {...props.modelValue, weekdays: days, times})
}
function updateDayTime(day: number, value: string) {
	const times = {...props.modelValue.times, [day]: value}
	if (value === props.modelValue.time) delete times[day]
	emit('update:modelValue', {...props.modelValue, times})
}
</script>

<style lang="scss" scoped>
.weekly-reminder {
	padding: .75rem;
	color: var(--text);
}
fieldset {
	padding: 0;
	border: 0;
	margin: 0 0 .75rem;
}
legend {
	margin-block-end: .5rem;
	font-weight: 600;
}
.weekday-options {
	display: flex;
	flex-wrap: wrap;
	gap: .5rem .75rem;
}
label {
	display: flex;
	flex-direction: column;
	gap: .3rem;
}
.weekday-options label {
	flex-direction: row;
	align-items: center;
	min-block-size: 1.75rem;
}
.weekday-times {
	margin-block-start: .75rem;
}
.weekday-time-inputs {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(min(100%, 8.5rem), 1fr));
	gap: .5rem;
}
.date-range {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: .5rem;
	margin-block-start: .75rem;
}
.input {
	min-inline-size: 0;
	inline-size: 100%;
}
.hint, .repeat-error {
	margin: .75rem 0 0;
	font-size: .85rem;
	line-height: 1.5;
}
.repeat-error { color: var(--danger-text); }
</style>
