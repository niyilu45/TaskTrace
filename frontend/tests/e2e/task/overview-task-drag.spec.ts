import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskRelationFactory} from '../../factories/task_relation'
import {TaskCommentFactory} from '../../factories/task_comment'
import {createDefaultViews} from '../project/prepareProjects'

test('overview reparents a subtree and restores it to the project root without moving its old parent', async ({authenticatedPage: page, apiContext, userToken}, testInfo) => {
	await ProjectFactory.create(1, {title: 'Overview drag project'})
	await createDefaultViews(1)
	await TaskFactory.create(1, {id: 1, title: 'Completed original parent', done: true})
	await TaskFactory.create(1, {id: 2, title: 'Move this task'}, false)
	await TaskFactory.create(1, {id: 3, title: 'New parent'}, false)
	await TaskFactory.create(1, {id: 4, title: 'Keep this descendant'}, false)
	await TaskRelationFactory.create(1, {id: 1, task_id: 1, other_task_id: 2, relation_kind: 'subtask'})
	await TaskRelationFactory.create(1, {id: 2, task_id: 2, other_task_id: 1, relation_kind: 'parenttask'}, false)
	await TaskRelationFactory.create(1, {id: 3, task_id: 2, other_task_id: 4, relation_kind: 'subtask'}, false)
	await TaskRelationFactory.create(1, {id: 4, task_id: 4, other_task_id: 2, relation_kind: 'parenttask'}, false)
	const outstanding = '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="keep"><p>Keep my outstanding item</p></li></ul>'
	await TaskCommentFactory.create(1, {id: 1, task_id: 2, comment: outstanding})
	await page.addInitScript(() => localStorage.setItem('tasktrace-completion-scope', 'pending'))
	await page.goto('/')
	const row = (id: number) => page.locator(`[data-overview-task="${id}"]`)
	const api = (process.env.API_URL || '').replace(/\/api\/v1\/?$/, '/api/v2')
	const headers = {Authorization: `Bearer ${userToken}`}
	const readTask = async (id: number) => (await apiContext.get(`${api}/tasks/${id}`, {headers})).json()
	const parents = async (id: number) => (await readTask(id)).related_tasks?.parenttask?.map((task: {id: number}) => task.id) || []
	await expect(row(1)).toHaveClass(/is-context/)
	await expect(row(2)).toHaveAttribute('data-depth', '1')
	const moved = page.waitForResponse(response => response.url().includes('/tasks/2/move') && response.request().method() === 'POST')
	await row(2).locator('.task-link').dragTo(row(3).locator('.single-task'))
	expect((await moved).ok()).toBe(true)
	await expect(row(2)).toHaveAttribute('data-depth', '1')
	await expect(row(4)).toHaveAttribute('data-depth', '2')
	await expect.poll(() => parents(2)).toEqual([3])
	expect(await parents(1)).toEqual([])
	expect(await parents(4)).toEqual([2])
	const comments = await apiContext.get(`${api}/tasks/2/comments`, {headers})
	expect(await comments.text()).toContain('Keep my outstanding item')
	await page.reload()
	await expect(row(4)).toHaveAttribute('data-depth', '2')
	// A parent cannot become a child of its descendant.
	await row(3).locator('.task-link').dragTo(row(4).locator('.single-task'))
	await expect(page.getByText('不能将任务移到自身或自己的子任务下。')).toBeVisible()
	expect(await parents(3)).toEqual([])
	const rootMove = page.waitForResponse(response => response.url().includes('/tasks/2/move') && response.request().method() === 'POST')

	const rootDrop = page.locator('[data-overview-project="1"] .overview-project-root-drop')
	await expect(rootDrop).toBeVisible()
	const sourceBounds = (await row(2).locator('.task-link').boundingBox())!
	await page.mouse.move(sourceBounds.x + 8, sourceBounds.y + sourceBounds.height / 2)
	await page.mouse.down()
	await page.mouse.move(sourceBounds.x + 30, sourceBounds.y + sourceBounds.height / 2, {steps: 5})
	await expect(rootDrop).toBeVisible()
	const targetBounds = (await rootDrop.boundingBox())!
	await page.mouse.move(targetBounds.x + targetBounds.width / 2, targetBounds.y + targetBounds.height / 2, {steps: 5})
	await page.mouse.move(targetBounds.x + targetBounds.width / 2 + 1, targetBounds.y + targetBounds.height / 2)
	await expect(rootDrop).toHaveClass(/is-drop-target/)
	await page.screenshot({path: testInfo.outputPath('overview-project-drop-active.png'), fullPage: true})
	await page.mouse.up()
	expect((await rootMove).ok()).toBe(true)
	await expect(rootDrop).not.toHaveClass(/is-drop-target/)
	await expect(row(2)).toHaveAttribute('data-depth', '0')
	await expect(row(4)).toHaveAttribute('data-depth', '1')
	await page.reload()
	await expect(row(2)).toHaveAttribute('data-depth', '0')
	expect(await parents(2)).toEqual([])
	expect(await parents(4)).toEqual([2])
	// A denied save leaves the persisted and displayed parent unchanged.
	await page.route('**/api/v2/tasks/2/move', route => route.fulfill({status: 403, contentType: 'application/problem+json', body: JSON.stringify({detail: 'Write permission denied'})}))
	await row(2).locator('.task-link').dragTo(row(3).locator('.single-task'))
	await expect(page.getByText(/任务移动失败，请检查编辑权限/)).toBeVisible()
	await expect(row(2)).toHaveAttribute('data-depth', '0')
	expect(await parents(2)).toEqual([])
	await page.unroute('**/api/v2/tasks/2/move')
	await page.getByRole('combobox', {name: '任务显示范围'}).selectOption('all')
	await expect(row(1)).toHaveAttribute('data-depth', '0')
	await expect(row(4)).toHaveAttribute('data-depth', '1')
	await page.screenshot({path: testInfo.outputPath('overview-drag-desktop.png'), fullPage: true})
	await page.setViewportSize({width: 390, height: 844})
	await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
	await page.screenshot({path: testInfo.outputPath('overview-drag-mobile.png'), fullPage: true})
})
