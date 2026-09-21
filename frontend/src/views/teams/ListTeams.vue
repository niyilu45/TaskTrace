<template>
	<div
		class="content loader-container is-max-width-desktop"
		:class="{ 'is-loading': teamService.loading}"
	>
		<header class="team-page-header">
			<div>
				<h1>{{ $t('team.title') }}</h1>
				<p class="has-text-grey">
					统一管理团队、协作成员和共享任务。
				</p>
			</div>
			<XButton
				:to="{name:'teams.create'}"
				icon="plus"
			>
				{{ $t('team.create.title') }}
			</XButton>
		</header>

		<section class="team-page-section">
			<h2>团队列表</h2>
			<Card
				v-if="teams.length > 0"
				:padding="false"
				:has-content="false"
			>
				<ul class="teams">
					<li
						v-for="team in teams"
						:key="team.id"
					>
						<RouterLink :to="{name: 'teams.edit', params: {id: team.id}}">
							<p>{{ team.name }}</p>
						</RouterLink>
					</li>
				</ul>
			</Card>
			<p
				v-else-if="!teamService.loading"
				class="team-empty has-text-grey is-italic"
			>
				{{ $t('team.noTeams') }}
				<RouterLink :to="{name: 'teams.create'}">
					{{ $t('team.create.title') }}.
				</RouterLink>
			</p>
		</section>

		<section
			v-if="isLocalBuild"
			class="team-page-section"
		>
			<div class="team-section-heading">
				<h2>局域网协作</h2>
				<p class="has-text-grey">
					管理 teamData 权限、协作任务团队和团队链接。
				</p>
			</div>
			<TasktraceTeamCenter />
		</section>
	</div>
</template>

<script setup lang="ts">
import {ref, shallowReactive} from 'vue'
import { useI18n } from 'vue-i18n'

import Card from '@/components/misc/Card.vue'
import TasktraceTeamCenter from '@/components/home/TasktraceTeamCenter.vue'
import TeamService from '@/services/team'
import { useTitle } from '@/composables/useTitle'
import {isLocalBuild} from '@/helpers/tasktraceLocal'

const { t } = useI18n({useScope: 'global'})
useTitle(() => t('team.title'))

const teams = ref([])
const teamService = shallowReactive(new TeamService())
teamService.getAll().then((result) => {
	teams.value = result
})
</script>

<style lang="scss" scoped>
ul.teams {
  padding: 0;
  margin-block-start: 0;
  margin-inline-start: 0;
  border-radius: $radius;
  overflow: hidden;

  li {
    list-style: none;
    margin: 0;
    border-inline-end: 1px solid var(--grey-200);

    a {
      color: var(--text);
      display: block;
      padding: 0.5rem 1rem;
      transition: background-color $transition;

      &:hover {
        background: var(--grey-100);
      }
    }
  }

  li:last-child {
    border-inline-end: none;
  }
}

.team-page-header,
.team-section-heading {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
	gap: 1rem;
}

.team-page-header {
	margin-block-end: 2rem;
}

.team-page-header h1,
.team-page-header p,
.team-section-heading h2,
.team-section-heading p {
	margin: 0;
}

.team-page-header p,
.team-section-heading p {
	margin-block-start: .25rem;
}

.team-page-section + .team-page-section {
	margin-block-start: 2.5rem;
}

.team-page-section > h2,
.team-section-heading {
	margin-block-end: 1rem;
}

.team-empty {
	padding: 1rem;
	text-align: center;
	background: var(--grey-50);
	border-radius: $radius;
}

@media screen and (max-width: $tablet) {
	.team-page-header,
	.team-section-heading {
		align-items: stretch;
		flex-direction: column;
	}
}
</style>
