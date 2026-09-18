<template>
	<div
		v-if="items.length"
		class="progress-backlinks"
	>
		<button
			type="button"
			class="progress-backlinks__toggle"
			:aria-expanded="expanded"
			@click="expanded = !expanded"
		>
			{{ expanded ? '收起引用方的进展' : `展开引用方的进展（${items.length}）` }}
		</button>
		<div
			v-if="expanded"
			class="progress-backlinks__items"
		>
			<article
				v-for="item in items"
				:key="item.id"
				class="progress-backlinks__item"
			>
				<strong>{{ item.date }}：</strong>
				<ReadonlyRichText :html="item.html" />
			</article>
		</div>
	</div>
</template>

<script setup lang="ts">
import {ref} from 'vue'
import type {ProgressBacklink} from '@/helpers/progressNotes'
import ReadonlyRichText from './ReadonlyRichText.vue'

defineProps<{items: ProgressBacklink[]}>()
const expanded = ref(false)
</script>

<style scoped lang="scss">
.progress-backlinks { margin-block-start: .35rem; }
.progress-backlinks__toggle {
	border: 0;
	padding: 0;
	background: transparent;
	color: var(--primary);
	font: inherit;
	cursor: pointer;
	text-decoration: underline;
	text-underline-offset: .15em;
}
.progress-backlinks__items {
	display: grid;
	gap: .45rem;
	margin-block-start: .45rem;
	padding-inline-start: .7rem;
	border-inline-start: 3px solid var(--primary);
}
.progress-backlinks__item {
	:deep(.readonly-rich-text), :deep(.readonly-rich-text > p:first-child) { display: inline; }
}
</style>
