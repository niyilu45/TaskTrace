import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskRelationFactory} from '../../factories/task_relation'
import {createDefaultViews} from '../project/prepareProjects'

test('checkbox status filters separate todo and doing, keep ancestors, persist and work in all web views', async ({authenticatedPage: page}, testInfo) => {
	await ProjectFactory.create(1, {title: 'Status selection project'})
	await createDefaultViews(1)
	const states = ['to-do', 'doing', 'done', 'hold']
	const labels = ['待办', '进行中', '已完成', '暂停']
	for (let i = 0; i < states.length; i++) {
		await TaskFactory.create(1, {id: i + 1, title: 'Status task ' + states[i], status: states[i], done: states[i] === 'done'}, i === 0)
	}
	await TaskFactory.create(1, {id: 5, title: 'Paused parent', status: 'hold'}, false)
	await TaskFactory.create(1, {id: 6, title: 'Doing child', status: 'doing'}, false)
	await TaskRelationFactory.create(1, {id: 1, task_id: 5, other_task_id: 6, relation_kind: 'subtask'})
	await TaskRelationFactory.create(1, {id: 2, task_id: 6, other_task_id: 5, relation_kind: 'parenttask'}, false)
	await page.goto('/')
	const group = page.getByRole('group', {name: '任务显示范围'})
	const all = group.getByRole('checkbox', {name: '全选', exact: true})
	await all.check()
	for (let mask = 0; mask < 16; mask++) {
		for (let i = 0; i < labels.length; i++) {
			await group.getByRole('checkbox', {name: labels[i], exact: true}).setChecked(!!(mask & (1 << i)))
		}
		const expected = [1, 2, 3, 4].filter((_, i) => mask & (1 << i))
		if (mask & 2) expected.push(5, 6)
		else if (mask & 8) expected.push(5)
		await expect.poll(async () => page.locator('[data-overview-task]').evaluateAll(elements => elements.map(el => Number(el.getAttribute('data-overview-task'))).sort((a, b) => a - b))).toEqual(expected)
	}
	await all.uncheck()
	await group.getByRole('checkbox', {name: '进行中', exact: true}).check()
	await expect(page.locator('[data-overview-task="5"]')).toHaveClass(/is-context/)
	await page.reload()
	await expect(group.getByRole('checkbox', {name: '进行中', exact: true})).toBeChecked()
	await expect(group.getByRole('checkbox', {name: '待办', exact: true})).not.toBeChecked()
	await expect(page.locator('.vikunja-loading')).toBeHidden()
	await page.screenshot({path: testInfo.outputPath('overview-status-filter.png'), fullPage: true})
	await page.setViewportSize({width: 390, height: 844})
	await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
	await group.screenshot({path: testInfo.outputPath('status-filter-mobile.png')})
	await page.setViewportSize({width: 1280, height: 900})
	await page.goto('/projects/1/1')
	const project = page.getByRole('region', {name: '项目展示模式', exact: true})
	const projectFilter = project.getByRole('group', {name: '显示选项'})
	await projectFilter.getByRole('checkbox', {name: '全选', exact: true}).uncheck()
	await projectFilter.getByRole('checkbox', {name: '进行中', exact: true}).check()
	await expect(project).toContainText('Status task doing')
	await expect(project).not.toContainText('Status task to-do')
	await expect(project).not.toContainText('Status task hold')
	await expect(project).toContainText('Paused parent')
	await expect(project).toContainText('Doing child')
	await project.screenshot({path: testInfo.outputPath('project-status-filter.png')})
	await page.goto('/projects/1/1?mode=edit')
	const editFilter = page.getByRole('group', {name: '显示选项'})
	await editFilter.getByRole('checkbox', {name: '全选', exact: true}).check()
	await editFilter.getByRole('checkbox', {name: '全选', exact: true}).uncheck()
	await editFilter.getByRole('checkbox', {name: '已完成', exact: true}).check()
	const list = page.locator('.project-list .list-view .tasks')
	await expect(list).toContainText('Status task done')
	await expect(list).not.toContainText('Status task doing')
	await editFilter.getByRole('checkbox', {name: '进行中', exact: true}).check()
	await expect(list).toContainText('Status task doing')
	await expect(list).toContainText('Status task done')
	await expect(list).not.toContainText('Status task to-do')
	const checkboxes = editFilter.getByRole('checkbox')
	await expect.poll(async () => checkboxes.evaluateAll(inputs => Math.max(...inputs.map(el => el.getBoundingClientRect().top)) - Math.min(...inputs.map(el => el.getBoundingClientRect().top)))).toBeLessThan(2)
	await page.screenshot({path: testInfo.outputPath('edit-status-filter.png'), fullPage: true})
	await page.setViewportSize({width: 390, height: 844})
	await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
	await editFilter.screenshot({path: testInfo.outputPath('edit-status-mobile.png')})
})
