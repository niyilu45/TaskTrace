<template>
	<div class="select">
		<select
			v-model="selection"
			:disabled="disabled || undefined"
			:aria-label="$t('task.attributes.priority')"
		>
			<template v-if="isLocalBuild">
				<option
					v-for="level in 10"
					:key="level - 1"
					:value="level - 1"
				>
					优先级 {{ level - 1 }}{{ level === 1 ? '（最高）' : level === 10 ? '（默认）' : '' }}
				</option>
			</template>
			<template v-else>
				<option :value="PRIORITIES.UNSET">
					{{ $t('task.priority.unset') }}
				</option>
				<option :value="PRIORITIES.LOW">
					{{ $t('task.priority.low') }}
				</option>
				<option :value="PRIORITIES.MEDIUM">
					{{ $t('task.priority.medium') }}
				</option>
				<option :value="PRIORITIES.HIGH">
					{{ $t('task.priority.high') }}
				</option>
				<option :value="PRIORITIES.URGENT">
					{{ $t('task.priority.urgent') }}
				</option>
				<option :value="PRIORITIES.DO_NOW">
					{{ $t('task.priority.doNow') }}
				</option>
			</template>
		</select>
	</div>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {tasktracePriorityNumber, tasktraceStoredPriority} from '@/helpers/tasktracePriority'
import {PRIORITIES} from '@/constants/priorities'

withDefaults(defineProps<{
	disabled?: boolean
}>(), {
	disabled: false,
})

const priority = defineModel<number>({
	required: true,
	default: 0,
})

const selection = computed({
	get: () => isLocalBuild ? tasktracePriorityNumber(priority.value) : priority.value,
	set: value => { priority.value = isLocalBuild ? tasktraceStoredPriority(Number(value)) : Number(value) },
})
</script>
