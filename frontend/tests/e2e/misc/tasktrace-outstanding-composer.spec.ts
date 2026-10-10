import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskCommentFactory} from '../../factories/task_comment'
import {TaskRelationFactory} from '../../factories/task_relation'

test('outstanding composer keeps actions inline and preserves collapsed notes', async ({authenticatedPage: page, currentUser}, testInfo) => {
	const [project] = await ProjectFactory.create(1, {owner_id: currentUser.id})
	const [task] = await TaskFactory.create(1, {project_id: project.id, created_by_id: currentUser.id})
	await page.goto(`/tasks/${task.id}`)
	const shared = page.getByRole('region', {name: '共享遗留事项'})
	await shared.getByRole('button', {name: '新增遗留事项', exact: true}).click()
	const header = shared.locator('.outstanding-composer-header')
	const note = shared.locator('.outstanding-note-editor [contenteditable="true"]')
	await expect(note).toHaveCount(0)
	await expect(shared.getByRole('combobox')).toHaveValue('7')
	await expect(shared.getByRole('button', {name: '选择图片', exact: true})).toHaveCount(0)
	await shared.getByRole('textbox', {name: '新增遗留事项', exact: true}).fill('等待确认设备复测结果')
	await shared.getByRole('combobox').selectOption('2')
	for (const width of [1280, 390]) {
		await page.setViewportSize({width, height: 900})
		await header.scrollIntoViewIfNeeded()
		const bounds = await header.locator(':scope > *').evaluateAll(elements => elements.map(element => {
			const rect = element.getBoundingClientRect()
			return {left: rect.left, right: rect.right, center: rect.top + rect.height / 2}
		}))
		expect(Math.max(...bounds.map(rect => rect.center)) - Math.min(...bounds.map(rect => rect.center))).toBeLessThan(2)
		expect(bounds[0].right).toBeLessThanOrEqual(bounds[1].left)
		expect(bounds[1].right).toBeLessThanOrEqual(bounds[2].left)
		expect(bounds[2].right).toBeLessThanOrEqual(width)
		await shared.locator('.outstanding-composer').screenshot({path: testInfo.outputPath(`composer-${width}.png`)})
	}
	await shared.getByRole('button', {name: '添加遗留事项备注', exact: true}).click()
	await note.fill('备注中的处理说明需要保留')
	await shared.getByRole('button', {name: '收起遗留事项备注', exact: true}).click()
	await header.getByRole('button', {name: '添加遗留事项', exact: true}).click()
	await expect(shared.locator('.outstanding-list')).toContainText('等待确认设备复测结果')
	await page.reload()
	await shared.getByRole('button', {name: '编辑', exact: true}).click()
	await expect(note).toHaveCount(0)
	await expect(shared.getByRole('combobox')).toHaveValue('2')
	await shared.getByRole('button', {name: '查看/编辑遗留事项备注', exact: true}).click()
	await expect(note).toContainText('备注中的处理说明需要保留')
	await shared.locator('.outstanding-composer').screenshot({path: testInfo.outputPath('composer-note-expanded.png')})
})


test('child edits reach parent summaries and another open editor without losing drafts', async ({authenticatedPage: page, apiContext, userToken}) => {
	const headers = {Authorization: `Bearer ${userToken}`}
	const project = await (await apiContext.post('../v2/projects', {headers, data: {title: 'Sync audit'}})).json()
	const parent = await (await apiContext.post(`../v2/projects/${project.id}/tasks`, {headers, data: {title: 'Sync parent'}})).json()
	const child = await (await apiContext.post(`../v2/projects/${project.id}/tasks`, {headers, data: {title: 'Sync child'}})).json()
	expect((await apiContext.post(`../v2/tasks/${parent.id}/relations`, {headers, data: {other_task_id: child.id, relation_kind: 'subtask'}})).ok()).toBeTruthy()
	const initial = '<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="sync-item" data-priority="7" data-done="false"><p>Waiting for review</p></li></ul>'
	const created = await apiContext.post(`../v2/tasks/${child.id}/comments`, {headers, data: {comment: initial}})
	expect(created.ok(), await created.text()).toBeTruthy()
	const comment = await created.json()
	await page.goto(`/projects/${project.id}`)
	await page.locator('.task-own-progress > summary').click()
	const summary = page.locator('.subtask-outstanding .outstanding-source').filter({hasText: 'Sync child'})
	await expect(summary).toContainText('[P7]')
	const editor = await page.context().newPage()
	await editor.goto(`/tasks/${child.id}`)
	const shared = editor.locator('.shared-outstanding')
	await shared.locator('.outstanding-actions button').first().click()
	await shared.getByRole('combobox').selectOption('2')
	await shared.locator('.outstanding-composer-header button').click()
	await page.bringToFront()
	await expect(summary).toContainText('[P2]')
	await expect(page.locator(`.progress-row[data-task-id="${child.id}"] .outstanding-cell`).first()).toContainText('[P2]')
	await editor.bringToFront()
	await shared.locator('.outstanding-actions button').first().click()
	// Simulate a floating-window save against the same canonical comment.
	const changed = initial.replace('data-priority="7"', 'data-priority="0"').replace('Waiting for review', 'Updated outside browser')
	expect((await apiContext.put(`../v2/tasks/${child.id}/comments/${comment.id}`, {headers, data: {comment: changed}})).ok()).toBeTruthy()
	await page.bringToFront()
	await expect(summary).toContainText('[P0]')
	await expect(summary).toContainText('Updated outside browser')
	await editor.bringToFront()
	await expect(shared.getByRole('combobox')).toHaveValue('0')
	await expect(shared.locator('.outstanding-composer textarea').first()).toHaveValue('Updated outside browser')
	await shared.locator('.outstanding-composer textarea').first().fill('My unsaved draft')
	expect((await apiContext.put(`../v2/tasks/${child.id}/comments/${comment.id}`, {headers, data: {comment: changed.replace('Updated outside browser', 'Another saved edit')}})).ok()).toBeTruthy()
	await page.bringToFront()
	await expect(summary).toContainText('Another saved edit')
	await editor.bringToFront()
	await expect(shared.locator('.outstanding-list')).toContainText('Another saved edit')
	await expect(shared.locator('.outstanding-composer textarea').first()).toHaveValue('My unsaved draft')
	// A completion edit removes the item from the ancestor's pending summary.
	expect((await apiContext.put(`../v2/tasks/${child.id}/comments/${comment.id}`, {headers, data: {comment: changed.replace('data-done="false"', 'data-done="true"')}})).ok()).toBeTruthy()
	await page.bringToFront()
	await expect(summary).toHaveCount(0)
	await page.screenshot({path: test.info().outputPath('synchronized-parent.png'), fullPage: true})
	await editor.close()
})


test('saved task fields and daily progress refresh without replacing an unsaved progress draft', async ({authenticatedPage: page, apiContext, userToken, currentUser}) => {
	const headers = {Authorization: `Bearer ${userToken}`}
	const [project] = await ProjectFactory.create(1, {owner_id: currentUser.id})
	const [task] = await TaskFactory.create(1, {project_id: project.id, created_by_id: currentUser.id, title: 'Initial title'})
	const day = new Date().toLocaleDateString('sv-SE')
	const progress = (text: string) => `<h3>\u6bcf\u65e5\u8fdb\u5c55 \u00b7 ${day}</h3><p>${text}</p>`
	const created = await apiContext.post(`../v2/tasks/${task.id}/comments`, {headers, data: {comment: progress('Initial daily progress')}})
	expect(created.ok(), await created.text()).toBeTruthy()
	const comment = await created.json()
	await page.goto(`/tasks/${task.id}`)
	const editor = page.locator('.daily-progress__editor [contenteditable="true"]')
	await expect(editor).toContainText('Initial daily progress')
	expect((await apiContext.patch(`../v2/tasks/${task.id}`, {headers: {...headers, 'Content-Type': 'application/merge-patch+json'}, data: {title: 'Externally saved title', description: '<p>Externally saved description</p>', priority: 2, status: 'hold'}})).ok()).toBeTruthy()
	await expect(page.locator('.heading h1')).toContainText('Externally saved title')
	await expect(page.locator('.description')).toContainText('Externally saved description')
	await expect(page.locator('.task-status-select')).toHaveValue('hold')
	expect((await apiContext.put(`../v2/tasks/${task.id}/comments/${comment.id}`, {headers, data: {comment: progress('Externally saved daily progress')}})).ok()).toBeTruthy()
	await expect(editor).toContainText('Externally saved daily progress')
	await editor.fill('Unsaved daily progress draft')
	expect((await apiContext.put(`../v2/tasks/${task.id}/comments/${comment.id}`, {headers, data: {comment: progress('Another saved daily progress')}})).ok()).toBeTruthy()
	await expect(page.locator('.comment')).toContainText(['Another saved daily progress'])
	await expect(editor).toContainText('Unsaved daily progress draft')
})


test('display refresh leaves unrelated histories and unopened editors idle', async ({authenticatedPage: page, apiContext, userToken, currentUser}, testInfo) => {
	const headers = {Authorization: `Bearer ${userToken}`}
	const [project] = await ProjectFactory.create(1, {owner_id: currentUser.id, title: 'Display performance'})
	const tasks = await TaskFactory.create(9, {project_id: project.id, created_by_id: currentUser.id, title: (index: number) => `Performance task ${index}`})
	const parent = tasks[0]
	const children = tasks.slice(1).map(task => Number(task.id))
	const content = (index: number, priority = 7) => `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="p-${index}" data-priority="${priority}"><p>Performance item ${index}</p></li></ul>`
	await TaskRelationFactory.create(16, {task_id: (index: number) => index <= 8 ? parent.id : children[index - 9], other_task_id: (index: number) => index <= 8 ? children[index - 1] : parent.id,
		relation_kind: (index: number) => index <= 8 ? 'subtask' : 'parenttask', created_by_id: currentUser.id})
	const comments = await TaskCommentFactory.create(8, {task_id: (index: number) => children[index - 1], comment: (index: number) => content(index - 1), author_id: currentUser.id})
	const commentIds = comments.map(comment => Number(comment.id))
	await page.goto(`/projects/${project.id}`)
	await page.locator('.task-own-progress > summary').click()
	const summaries = page.locator('.subtask-outstanding .outstanding-source')
	await expect(summaries).toHaveCount(8)
	const reads: number[] = []
	page.on('request', request => {
		const match = new URL(request.url()).pathname.match(/\/tasks\/(\d+)\/comments$/)
		if (request.method() === 'GET' && match) reads.push(Number(match[1]))
	})
	expect((await apiContext.put(`../v2/tasks/${children[0]}/comments/${commentIds[0]}`, {headers, data: {comment: content(0, 2)}})).ok()).toBeTruthy()
	await expect(summaries.filter({hasText: 'Performance item 0'})).toContainText('[P2]')
	expect(reads.filter(id => children.slice(1).includes(id))).toEqual([])
	expect(reads.filter(id => id === children[0])).toHaveLength(1)
	await testInfo.attach('live-refresh-requests', {body: JSON.stringify({projectTasks: 9, changedTasks: 1, historyReads: reads}), contentType: 'application/json'})
	await page.screenshot({path: testInfo.outputPath('display-performance.png'), fullPage: true})
	await page.locator(`.progress-row[data-task-id="${children[7]}"]`).scrollIntoViewIfNeeded()
	await expect(page.locator(`.progress-row[data-task-id="${children[7]}"] .outstanding-cell`)).toContainText('Performance item 7')
})
