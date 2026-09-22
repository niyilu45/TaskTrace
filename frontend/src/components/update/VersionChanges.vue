<template>
	<div class="version-changes">
		<dl class="version-range">
			<div><dt>本地版本</dt><dd>{{ currentVersion || '未知' }}</dd></div>
			<div><dt>最新版本</dt><dd>{{ latestVersion || currentVersion || '未知' }}</dd></div>
		</dl>
		<details v-if="available">
			<summary>查看版本改动</summary>
			<pre>{{ releaseNotes || '本次发布未填写更新内容。' }}</pre>
		</details>
		<p
			v-else
			class="up-to-date"
		>
			当前已是最新版本，暂无版本差异。
		</p>
	</div>
</template>

<script setup lang="ts">
defineProps<{
	currentVersion?: string,
	latestVersion?: string,
	releaseNotes?: string,
	available?: boolean,
}>()
</script>

<style lang="scss" scoped>
.version-changes {
	display: grid;
	gap: 1rem;
	text-align: start;
}

.version-range {
	display: grid;
	gap: .5rem;
	margin: 0;

	div {
		display: grid;
		grid-template-columns: 6rem minmax(0, 1fr);
		gap: .75rem;
	}

	dt {
		color: var(--grey-500);
	}

	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
}

details {
	border: 1px solid var(--grey-200);
	border-radius: $radius;
	background: var(--white);
}

summary {
	padding: .75rem 1rem;
	font-weight: 700;
	cursor: pointer;
}

pre {
	max-block-size: 20rem;
	margin: 0;
	padding: 1rem;
	overflow: auto;
	border-block-start: 1px solid var(--grey-200);
	background: var(--grey-100);
	color: var(--text);
	white-space: pre-wrap;
	font: inherit;
}

.up-to-date {
	margin: 0;
	color: var(--success);
}
</style>
