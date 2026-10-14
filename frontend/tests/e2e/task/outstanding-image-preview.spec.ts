import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskCommentFactory} from '../../factories/task_comment'
import {TaskRelationFactory} from '../../factories/task_relation'
import {createDefaultViews} from '../project/prepareProjects'

test('outstanding images preview without opening task editors in rows and parent summaries', async ({authenticatedPage: page}) => {
	await ProjectFactory.create(1, {title: 'Picture project'})
	await createDefaultViews(1)
	await TaskFactory.create(1, {id: 1, title: 'Parent task'})
	await TaskFactory.create(1, {id: 2, title: 'Child task'}, false)
	await TaskRelationFactory.create(1, {id: 1, task_id: 1, other_task_id: 2, relation_kind: 'subtask'})
	await TaskRelationFactory.create(1, {id: 2, task_id: 2, other_task_id: 1, relation_kind: 'parenttask'}, false)
	const picture = await page.evaluate(() => {
		const canvas = document.createElement('canvas')
		canvas.width = 160; canvas.height = 80
		const context = canvas.getContext('2d')!
		context.fillStyle = '#cbddeb'; context.fillRect(0, 0, 160, 80)
		return canvas.toDataURL('image/png')
	})
	await TaskCommentFactory.create(1, {task_id: 2, comment: '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="open"><p>Pending picture</p><img src="' + picture + '"></li><li data-id="closed" data-done="true"><p>Finished picture</p><img src="' + picture + '"></li></ul>'})
	await page.goto('/projects/1/1?mode=browse')
	const row = page.locator('.progress-row[data-task-id="2"]')
	const image = row.locator('.outstanding-edit-link img').first()
	await expect(image).toBeVisible()
	const projectUrl = page.url()
	async function assertPreview() {
		await expect(page.locator('.image-lightbox')).toBeVisible()
		await expect(page).toHaveURL(projectUrl)
		await expect(page.locator('.task-view')).toHaveCount(0)
		await page.keyboard.press('Escape')
		await expect(page.locator('.image-lightbox')).toHaveCount(0)
		await expect(page).toHaveURL(projectUrl)
	}
	await image.click()
	await assertPreview()
	await image.dblclick()
	await assertPreview()
	await row.getByRole('button', {name: /展开已完成的遗留事项/}).click()
	await row.locator('.completed-outstanding img').click()
	await assertPreview()
	await page.locator('.task-own-progress > summary').click()
	await page.locator('.subtask-outstanding img').click()
	await assertPreview()
	// Clicking the item's text still opens its task card.
	await row.getByText('Pending picture', {exact: true}).click()
	await expect(page).toHaveURL(/\/tasks\/2/)
	await expect(page.locator('.task-view')).toBeVisible()
})
