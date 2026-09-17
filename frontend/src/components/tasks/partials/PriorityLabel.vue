<template>
	<span
		v-if="!done && (showAll || priority >= minimumPriority)"
		:class="{
			'negligible': priority <= lowThreshold,
			'not-so-high': priority > lowThreshold && priority < highThreshold,
			'high-priority': priority >= highThreshold
		}"
		class="priority-label"
	>
		<span class="icon">
			<Icon
				v-if="priority >= highThreshold"
				icon="exclamation-circle"
			/>
			<Icon
				v-else
				icon="exclamation"
			/>
		</span>
		<span>
			<template v-if="isLocalBuild">优先级 {{ tasktracePriorityNumber(priority) }}</template>
			<template v-else>
				<template v-if="priority === priorities.UNSET">{{ $t('task.priority.unset') }}</template>
				<template v-if="priority === priorities.LOW">{{ $t('task.priority.low') }}</template>
				<template v-if="priority === priorities.MEDIUM">{{ $t('task.priority.medium') }}</template>
				<template v-if="priority === priorities.HIGH">{{ $t('task.priority.high') }}</template>
				<template v-if="priority === priorities.URGENT">{{ $t('task.priority.urgent') }}</template>
				<template v-if="priority === priorities.DO_NOW">{{ $t('task.priority.doNow') }}</template>
			</template>
		</span>
	</span>
</template>

<script setup lang="ts">
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {tasktracePriorityNumber} from '@/helpers/tasktracePriority'
import {computed} from 'vue'
import {PRIORITIES as priorities} from '@/constants/priorities'
import {useAuthStore} from '@/stores/auth'
	
withDefaults(defineProps<{
	priority: number,
	showAll?: boolean,
	done?: boolean
}>(), {
	showAll: false,
	done: false,
})

const authStore = useAuthStore()
const lowThreshold = isLocalBuild ? 4 : priorities.LOW
const highThreshold = isLocalBuild ? 8 : priorities.HIGH

const minimumPriority = computed(() => {
	return authStore.settings.frontendSettings.minimumPriority || priorities.MEDIUM
})
</script>

<style lang="scss" scoped>
.high-priority {
	color: var(--danger-text);
	inline-size: auto !important; // To override the width set in tasks
}

.not-so-high {
	color: var(--warning);
}

.negligible {
	color: var(--info);
}

.icon {
	vertical-align: top;
	inline-size: auto !important;
	padding-inline-end: .5rem;
}
</style>
