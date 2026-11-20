import {test, expect} from '../../support/fixtures'
import {TaskFactory} from '../../factories/task'
import {TaskCommentFactory} from '../../factories/task_comment'
import {createProjects} from './prepareProjects'

test('switches projects while old progress reads are pending and loads editing tools on demand', async ({authenticatedPage: page}) => {
	await createProjects(2)
	await TaskFactory.create(6, {id: '{increment}', project_id: 1, title: (i: number) => 'Previous task ' + i})
	await TaskFactory.create(1, {id: 101, project_id: 2, title: 'Current project task'}, false)
	await TaskCommentFactory.create(6, {id: '{increment}', task_id: (i: number) => i, comment: '<h3>每日进展 · 2026-11-20</h3><p>Previous progress</p>'})
	await TaskCommentFactory.create(1, {id: 101, task_id: 101, comment: '<h3>每日进展 · 2026-11-20</h3><p>Current project progress</p>'}, false)
	const editingAssets: string[] = []
	page.on('request', request => {
		if (/\/assets\/Project(?:Gantt|Kanban|List|Table)-[^/]+\.js/.test(request.url())) editingAssets.push(request.url())
	})
	let hold = true
	let pending = 0
	let release!: () => void
	const held = new Promise<void>(resolve => { release = resolve })
	await page.route(/\/api\/v[12]\/tasks\/[1-6]\/comments(?:\?|$)/, async route => {
		if (hold) { pending++; await held }
		await route.continue().catch(() => { /* The old view cancelled this request. */ })
	})
	try {
		await page.goto('/projects/1/1?mode=browse')
		await page.locator('.task-own-progress > summary').first().click()
		await expect.poll(() => pending).toBeGreaterThan(0)
		expect(editingAssets).toEqual([])
		const start = Date.now()
		await page.locator('[data-project-id="2"] .list-menu-link').first().click()
		await expect(page.locator('.project-progress')).toContainText('Current project task', {timeout: 3000})
		await page.locator('.task-own-progress > summary').first().click()
		await expect(page.locator('.project-progress')).toContainText('Current project progress', {timeout: 3000})
		await test.info().attach('project-switch-timing', {body: JSON.stringify({milliseconds: Date.now() - start, pendingOldReads: pending}), contentType: 'application/json'})
		await expect(page.locator('.project-progress')).not.toContainText('Previous task')
		expect(editingAssets).toEqual([])
		hold = false; release()
		await page.screenshot({path: test.info().outputPath('project-switch-desktop.png')})
		await page.getByRole('button', {name: '进入编辑模式', exact: true}).click()
		await expect(page.locator('.project-mode-bar')).toContainText('编辑模式')
		await expect(page.locator('.project-list .tasks')).toContainText('Current project task')
		expect(editingAssets.some(url => /ProjectList-/.test(url))).toBe(true)
		expect(editingAssets.some(url => /Project(?:Gantt|Kanban)-/.test(url))).toBe(false)
		await page.getByRole('button', {name: '返回展示模式', exact: true}).click()
		await page.locator('.task-own-progress > summary').first().click()
		await expect(page.locator('.project-progress')).toContainText('Current project progress')
	} finally { hold = false; release() }
})
