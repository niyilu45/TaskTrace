<template>
	<details
		v-if="hasNote"
		class="outstanding-note"
		@click.stop
		@dblclick.stop
		@keydown.stop
		@toggle="expanded = ($event.target as HTMLDetailsElement).open"
	>
		<summary :aria-expanded="expanded">
			{{ expanded ? '收起备注' : '展开备注' }}
		</summary>
		<ReadonlyRichText
			v-if="expanded"
			:html="html"
		/>
	</details>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {hasOutstandingNote} from '@/helpers/sharedOutstanding'
import ReadonlyRichText from './ReadonlyRichText.vue'

const props = withDefaults(defineProps<{html?: string}>(), {html: ''})
const hasNote = computed(() => hasOutstandingNote(props.html))
const expanded = ref(false)
</script>

<style scoped lang="scss">
.outstanding-note {
	min-inline-size: 0;
	margin-block-start: .35rem;
	color: var(--text);
	font-weight: 400;
	cursor: auto;

	summary {
		inline-size: fit-content;
		color: var(--primary);
		font-size: .875rem;
		cursor: pointer;

		&:hover { text-decoration: underline; }

		&:focus-visible {
			outline: 2px solid var(--primary);
			outline-offset: 2px;
		}
	}

	.readonly-rich-text { margin-block-start: .4rem; }
}
</style>
