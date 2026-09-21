<template>
	<span
		class="team-member-identity"
		:title="teamStore.identityTitleFor(username)"
	>
		{{ teamStore.displayNameFor(username) }}
	</span>
</template>

<script setup lang="ts">
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

defineProps<{username: string}>()
const teamStore = useTasktraceTeamStore()
</script>

<style scoped>
.team-member-identity {
	overflow-wrap: anywhere;
}
</style>
