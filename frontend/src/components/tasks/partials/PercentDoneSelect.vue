<template>
	<div>
		<span v-if="done">当前完成度：100%（已勾选完成） · </span>
		<label>填写完成度
			<span class="select">
				<select
					v-model.number="percentDone"
					aria-label="填写完成度，勾选完成不会清除填写值"
					:disabled="disabled || undefined"
				>
					<option
						v-for="option in PERCENT_OPTIONS"
						:key="option"
						:value="option"
					>
						{{ option * 100 }}%
					</option>
				</select>
			</span>
		</label>
	</div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
	done?: boolean
	disabled?: boolean
}>(), {
	done: false,
	disabled: false,
})

const percentDone = defineModel<number>({ required: true })

const PERCENT_OPTIONS = [0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1] as const
</script>
