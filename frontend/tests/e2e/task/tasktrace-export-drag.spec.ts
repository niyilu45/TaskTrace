import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskCommentFactory} from '../../factories/task_comment'
import {TaskFactory} from '../../factories/task'
import {TaskRelationFactory} from '../../factories/task_relation'
import {createDefaultViews} from '../project/prepareProjects'

test('move a child while completed tasks are shown, then export and preview the subtree', async ({authenticatedPage: page, apiContext, userToken}, testInfo) => {
 await ProjectFactory.create(1, {title: 'Export project'})
 await createDefaultViews(1)
 await TaskFactory.create(1, {id: 1, title: 'Original parent'})
 await TaskFactory.create(1, {id: 2, title: 'Child task', description: '<p>First line<br>Second line</p>'}, false)
 await TaskFactory.create(1, {id: 3, title: 'Target parent'}, false)
 await TaskFactory.create(1, {id: 4, title: 'Completed task', done: true}, false)
 await TaskRelationFactory.create(1, {id: 1, task_id: 1, other_task_id: 2, relation_kind: 'subtask'})
 await TaskRelationFactory.create(1, {id: 2, task_id: 2, other_task_id: 1, relation_kind: 'parenttask'}, false)
 await page.goto('/projects/1/1?mode=edit')
 await page.getByRole('checkbox', {name: '显示已完成任务'}).check()
 const source = page.locator('[data-task-id="2"] > .single-task .task-link').first()
 const destination = page.locator('[data-task-id="3"] > .single-task').first()
 await expect(source).toHaveAttribute('draggable', 'true')
 const response = page.waitForResponse(response => response.url().includes('/tasks/2/move') && response.request().method() === 'POST')
 await source.dragTo(destination)
 expect((await response).ok()).toBeTruthy()
 const api = (process.env.API_URL || '').replace(/\/api\/v1\/?$/, '/api/v2')
 const saved = await (await apiContext.get(`${api}/tasks/2`, {headers: {Authorization: `Bearer ${userToken}`}})).json()
 expect(saved.related_tasks.parenttask.map((item: {id: number}) => item.id)).toEqual([3])
 const picture = await page.evaluate(() => {const canvas=document.createElement('canvas');canvas.width=260;canvas.height=75;const context=canvas.getContext('2d')!;context.fillStyle='#d6edf8';context.fillRect(0,0,260,75);context.fillStyle='#17212d';context.font='18px sans-serif';context.fillText('Progress picture',18,44);return canvas.toDataURL('image/png')})
 await TaskCommentFactory.create(1, {id: 1, task_id: 3, comment: '<h3>每日进展 · 2026-10-14</h3><p>Before the picture</p><img src="'+picture+'"><p>After the picture</p>'})
 await TaskCommentFactory.create(1, {id: 2, task_id: 3, comment: '<h3>每日进展 · 2026-10-13</h3><p>Earlier date with same picture</p><img src="'+picture+'">'}, false)
 await TaskCommentFactory.create(1, {id: 3, task_id: 3, comment: '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-done="true"><p>Finished item</p></li></ul>'}, false)
 await page.goto('/tasks/3')
 await page.getByRole('button', {name: '导出 Markdown', exact: true}).click()
 await page.evaluate(() => {
  (window as any).showSaveFilePicker = async (options: any) => {
   (window as any).__savedName = options.suggestedName
   return {createWritable: async () => ({write: async (blob: Blob) => {(window as any).__savedMarkdown = await blob.text()}, close: async () => {}, abort: async () => {}})}
  }
 })
 await page.getByRole('button', {name: '选择位置并保存'}).click()
 await expect(page.getByRole('status').filter({hasText: 'Markdown 文件已保存'})).toBeVisible()
 expect(await page.evaluate(() => (window as any).__savedName)).toBe('Target parent.md')
 expect(await page.evaluate(() => (window as any).__savedMarkdown)).toContain('Child task')
 const opened = page.context().waitForEvent('page')
 await page.getByRole('button', {name: '预览（新标签页）'}).click()
 const preview = await opened
 await expect(preview.getByRole('article')).toContainText('Child task')
 await expect(preview.getByRole('article')).toContainText('Second line')
 await expect(preview.locator('.vikunja-loading')).toBeHidden()
 await expect(preview.getByRole('article')).toContainText('☑')
 await expect(preview.getByRole('article').locator('img')).toHaveCount(2)
 await expect.poll(() => preview.getByRole('article').locator('img').evaluateAll(images => images.every(image => (image as HTMLImageElement).naturalWidth === 260))).toBe(true)
 await preview.screenshot({path: testInfo.outputPath('markdown-desktop.png'), fullPage: true, animations: 'disabled'})
 await preview.setViewportSize({width: 480, height: 780})
 await expect(preview.locator('.menu-container')).not.toHaveClass(/is-active/)
 await expect.poll(() => preview.getByRole('article').evaluate(element => element.getBoundingClientRect().left)).toBeGreaterThanOrEqual(0)
 await preview.screenshot({path: testInfo.outputPath('markdown-narrow.png'), fullPage: true, animations: 'disabled'})
 await preview.close()
})
