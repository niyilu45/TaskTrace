<template>
	<fieldset class="task-status-filter">
		<legend>{{ label }}</legend>
		<div class="task-status-filter__choices">
			<label>
				<input
					type="checkbox"
					:checked="allSelected"
					:indeterminate="modelValue.length > 0 && !allSelected"
					@change="emit('update:modelValue', allSelected ? [] : [...allTaskStatuses])"
				>
				全选
			</label>
			<label
				v-for="option in TASK_STATUS_OPTIONS"
				:key="option.value"
			>
				<input
					type="checkbox"
					:checked="modelValue.includes(option.value)"
					@change="toggle(option.value)"
				>
				{{ option.label }}
			</label>
		</div>
	</fieldset>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {TASK_STATUS_OPTIONS, type TaskStatus} from '@/types/ITaskStatus'
import {allTaskStatuses} from '@/helpers/taskStatusFilter'

const props = withDefaults(defineProps<{modelValue: TaskStatus[], label?: string}>(), {label: '显示选项'})
const emit = defineEmits<{'update:modelValue': [value: TaskStatus[]]}>()
const allSelected = computed(() => allTaskStatuses.every(state => props.modelValue.includes(state)))
function toggle(state: TaskStatus) {
	emit('update:modelValue', allTaskStatuses.filter(value => value === state ? !props.modelValue.includes(value) : props.modelValue.includes(value)))
}
</script>

<style scoped lang="scss">
.task-status-filter {
	margin: 0;
	padding: 0;
	border: 0;
	min-inline-size: 0;
	color: var(--text);

	legend {
		padding: 0;
		margin-block-end: .25rem;
		font-size: .875rem;
	}

	&__choices {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: .25rem .875rem;
	}

	label {
		display: inline-flex;
		align-items: center;
		gap: .375rem;
		min-block-size: 1.75rem;
		white-space: nowrap;
		cursor: pointer;
	}

	input {
		margin: 0;
		accent-color: var(--primary);
	}
}
</style>
