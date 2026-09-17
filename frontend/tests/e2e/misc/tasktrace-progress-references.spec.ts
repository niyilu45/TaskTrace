import {test, expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'
import {execFileSync} from 'node:child_process'
import {setTimeout as delay} from 'node:timers/promises'

test.use({serviceWorkers: 'block'})

type Comment = {id: number, comment: string}

test('daily progress references preserve original snapshots through drafts, editing, autosave, undo and filtered overview', async ({page, request, context}) => {
	test.setTimeout(120000)
	page.setDefaultTimeout(10000)
	const root = process.env.TASKTRACE_LOCAL_TEST_DIR
	test.skip(!root, 'Requires a running isolated portable test instance')
	const sessionPath = path.join(root!, 'data/progress-references-browser-session.json')
	execFileSync(path.join(root!, 'TaskTrace-server.exe'), ['--config', path.join(root!, 'data/local-config.yml'), 'tasktrace-local-session', '--output', sessionPath, '--user-id', readFileSync(path.join(root!, 'data/local-user-id.txt'), 'utf8').trim()], {windowsHide: true, stdio: 'pipe'})
	const session = JSON.parse(readFileSync(sessionPath, 'utf8').replace(/^\uFEFF/, ''))
	const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
	const headers = {Authorization: 'Bearer ' + session.token, 'X-TaskTrace-Undo': '1'}
	async function write(url: string, data: object, method: 'post' | 'put' = 'post') {
		const response = await request[method](base + '/api/v2' + url, {headers, data})
		if (!response.ok()) throw new Error(`${url}: HTTP ${response.status()} ${await response.text()}`)
		const value = await response.json()
		await delay(100)
		return value
	}
	async function get(url: string) {
		const response = await request.get(base + '/api/v2' + url, {headers})
		if (!response.ok()) throw new Error(`${url}: HTTP ${response.status()} ${await response.text()}`)
		return response.json()
	}
	const project = await write('/projects', {title: '每日进展引用验收'})
	try {
		const task = await write(`/projects/${project.id}/tasks`, {title: '设备测试与结果更正'})
		const upload = await request.post(`${base}/api/v2/tasks/${task.id}/attachments`, {headers, multipart: {files: {name: 'reference.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6AAAAAElFTkSuQmCC', 'base64')}}})
		expect(upload.ok()).toBeTruthy()
		const attachmentId = (await upload.json()).success[0].id
		await delay(100)
		const firstDate = '2022-03-01'
		const secondDate = '2022-03-03'
		const targetDate = '2022-03-07'
		const draftDate = '2022-03-08'
		const oldOne = `<h3>每日进展 · ${firstDate}</h3><p>初次测试认为设备已稳定。</p><p><img src="/api/v1/tasks/${task.id}/attachments/${attachmentId}" alt="测试截图"></p>`
		const oldTwo = `<h3>每日进展 · ${firstDate}</h3><p>同日补充：已记录第一轮测试环境。</p>`
		const secondBody = `<h3>每日进展 · ${secondDate}</h3><p>复核记录：仍需延长连续运行测试。</p>`
		const first = await write(`/tasks/${task.id}/comments`, {comment: oldOne})
		const sameDay = await write(`/tasks/${task.id}/comments`, {comment: oldTwo})
		const second = await write(`/tasks/${task.id}/comments`, {comment: secondBody})
		await write(`/tasks/${task.id}/comments`, {comment: '<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="confirm-results">继续确认长时间运行结果</li></ul>'})
		const comments = async (): Promise<Comment[]> => (await get(`/tasks/${task.id}/comments?per_page=100&order_by=desc`)).items
		const savedDay = async () => (await comments()).find(note => note.comment.includes(`每日进展 · ${targetDate}</h3>`))
		const originalById = new Map((await comments()).map(note => [note.id, note.comment]))
		const correction = '更正：前次测试时间不足，补充连续运行测试后再确认稳定性。'
		await page.addInitScript(() => {
			if (!localStorage.getItem('tasktrace-autosave')) localStorage.setItem('tasktrace-autosave', JSON.stringify({enabled: false, seconds: 5}))
		})
		await page.setViewportSize({width: 1440, height: 1000})
		await page.goto(`${base}/tasks/${task.id}#tasktrace-local=` + encodeURIComponent(JSON.stringify(session)))
		const daily = page.locator('.daily-progress')
		const date = daily.getByLabel('记录日期', {exact: true})
		const progress = daily.getByLabel('今日进展', {exact: true})
		const picker = daily.getByRole('combobox', {name: '选择要引用的进展日期', exact: true})
		const addReference = daily.getByRole('button', {name: '添加引用', exact: true})
		const references = daily.getByRole('region', {name: '已引用的历史进展', exact: true})
		const cards = references.locator('.progress-reference')
		const save = daily.getByRole('button', {name: '保存进展', exact: true})
		const undo = page.getByRole('button', {name: '撤销上一步操作', exact: true})
		async function switchDate(value: string) {
			await date.fill(value)
			await date.dispatchEvent('change')
			await expect(progress).toBeEnabled()
			await expect(date).toHaveValue(value)
		}
		async function saveProgress() {
			await save.click()
			await expect(daily.locator('.daily-progress-actions [role=status]')).toContainText('当天进展已保存')
		}
		async function readReferences(comment: string) {
			return page.evaluate(html => Array.from(new DOMParser().parseFromString(html, 'text/html').querySelectorAll('blockquote[data-tasktrace-reference="1"]')).map(element => ({
				id: element.getAttribute('data-reference-id'),
				date: element.getAttribute('data-date'),
				task: element.getAttribute('data-task-id'),
				commentIds: (element.getAttribute('data-comment-ids') || '').split(',').map(Number).sort((a, b) => a - b),
				text: element.querySelector('[data-tasktrace-reference-content="1"]')?.textContent,
				html: element.querySelector('[data-tasktrace-reference-content="1"]')?.innerHTML,
			})), comment)
		}
		await switchDate(firstDate)
		await expect(picker.locator('option')).toHaveCount(1)
		await switchDate(targetDate)
		await picker.selectOption(firstDate)
		await addReference.click()
		await expect(cards).toHaveCount(1)
		await expect(picker.locator(`option[value="${firstDate}"]:not([disabled])`)).toHaveCount(0)
		await picker.selectOption(secondDate)
		await addReference.click()
		await expect(cards).toHaveCount(2)
		await expect(references).toContainText('同日补充：已记录第一轮测试环境。')
		await expect(references.locator('img')).toHaveCount(1)
		await expect.poll(() => references.locator('img').evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0)
		await expect(daily.locator(':scope > .readonly-rich-text img')).toHaveCount(0)
		await expect(save).toBeEnabled()
		await progress.fill(correction)
		await expect(undo).toBeDisabled()
		await switchDate(draftDate)
		await expect(progress).toHaveValue('')
		await expect(cards).toHaveCount(0)
		await switchDate(targetDate)
		await expect(progress).toHaveValue(correction)
		await expect(cards).toHaveCount(2)
		await page.reload()
		await switchDate(targetDate)
		await expect(progress).toHaveValue(correction)
		await expect(cards).toHaveCount(2)
		await saveProgress()
		const saved = await savedDay()
		expect(saved).toBeDefined()
		const snapshots = await readReferences(saved!.comment)
		expect(snapshots).toHaveLength(2)
		const firstSnapshot = snapshots.find(reference => reference.date === firstDate)!
		expect(firstSnapshot.commentIds).toEqual([first.id, sameDay.id].sort((a, b) => a - b))
		expect(firstSnapshot.task).toBe(String(task.id))
		expect(firstSnapshot.id).toMatch(/^r[a-f0-9]{32}$/)
		expect(firstSnapshot.html).toContain(`/attachments/${attachmentId}`)
		expect(snapshots.find(reference => reference.date === secondDate)!.commentIds).toEqual([second.id])
		for (const original of (await comments()).filter(note => originalById.has(note.id))) expect(original.comment).toBe(originalById.get(original.id))
		console.info('Progress references: creation, daily drafts and original preservation passed')
		await page.reload()
		await page.locator('#comment-' + saved!.id).getByRole('button', {name: '编辑当天进展', exact: true}).click()
		await expect(date).toHaveValue(targetDate)
		await expect(progress).toHaveValue(correction)
		await expect(cards).toHaveCount(2)
		await expect(daily.locator(':scope > .readonly-rich-text img')).toHaveCount(0)
		await progress.fill(correction + ' 已安排复测。')
		await saveProgress()
		expect(await readReferences((await savedDay())!.comment)).toEqual(snapshots)
		const editedSource = `<h3>每日进展 · ${firstDate}</h3><p>来源后来修订：增加温度条件。</p>`
		await write(`/tasks/${task.id}/comments/${first.id}`, {comment: editedSource}, 'put')
		await page.reload()
		await switchDate(targetDate)
		await expect(cards).toHaveCount(2)
		await expect(references).toContainText('初次测试认为设备已稳定。')
		await expect(references).not.toContainText('来源后来修订')
		expect(await readReferences((await savedDay())!.comment)).toEqual(snapshots)
		const originalLink = references.locator(`[data-reference-date="${firstDate}"] a[href$="#comment-${sameDay.id}"]`)
		await expect(originalLink).toHaveAttribute('target', '_blank')
		// Simulate a paginated comment list that does not include the referenced record.
		const sourceLists = new RegExp(`/api/v[12]/tasks/${task.id}(?:/comments)?(?:[?]|$)`)
		const omitSourceFromList = async (route: import('@playwright/test').Route) => {
			const response = await route.fetch()
			const body = await response.json()
			const visible = (items: Comment[]) => items.filter(note => note.id !== sameDay.id)
			const json = Array.isArray(body) ? visible(body) : Array.isArray(body.comments) ? {...body, comments: visible(body.comments)} : {...body, items: visible(body.items || [])}
			await route.fulfill({response, json})
		}
		await context.route(sourceLists, omitSourceFromList)
		const popupPromise = context.waitForEvent('page')
		await originalLink.click()
		const popup = await popupPromise
		await expect(popup.locator(`#comment-${sameDay.id}`)).toContainText('同日补充：已记录第一轮测试环境。')
		await expect(popup.locator('#comment-' + sameDay.id + '.linked-comment')).toBeVisible()
		await popup.close()
		await context.unroute(sourceLists, omitSourceFromList)
		await references.getByRole('button', {name: `移除 ${secondDate} 的引用`, exact: true}).click()
		await expect(cards).toHaveCount(1)
		await saveProgress()
		expect(await readReferences((await savedDay())!.comment)).toHaveLength(1)
		await expect(undo).toBeEnabled()
		const settings = daily.locator('.auto-save-settings')
		await settings.locator('summary').click()
		await settings.getByRole('checkbox', {name: '启用自动保存', exact: true}).check()
		await expect(settings.getByRole('spinbutton')).toHaveValue('5')
		const undone = page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith('/api/v2/tasktrace/undo'))
		await undo.click()
		expect((await undone).ok()).toBeTruthy()
		await expect(date).not.toHaveValue(targetDate)
		await expect(progress).toBeEnabled()
		await switchDate(targetDate)
		await expect(cards).toHaveCount(2)
		expect(await readReferences((await savedDay())!.comment)).toEqual(snapshots)
		let staleWrites = 0
		const detectWrite = (req: import('@playwright/test').Request) => { if (['POST', 'PUT'].includes(req.method()) && new URL(req.url()).pathname.includes(`/tasks/${task.id}/comments`)) staleWrites++ }
		page.on('request', detectWrite)
		await page.waitForTimeout(5500)
		page.off('request', detectWrite)
		expect(staleWrites).toBe(0)
		expect(await readReferences((await savedDay())!.comment)).toEqual(snapshots)
		await progress.fill(correction + ' 自动保存补充复测安排。')
		await expect.poll(async () => (await savedDay())?.comment, {timeout: 12000}).toContain('自动保存补充复测安排。')
		expect(await readReferences((await savedDay())!.comment)).toEqual(snapshots)
		expect((await get(`/tasks/${task.id}/attachments`)).items.map((item: {id: number}) => item.id)).toEqual([attachmentId])
		console.info('Progress references: snapshot independence, removal undo and autosave passed')
		await page.goto(`${base}/projects/${project.id}`)
		const overview = page.getByRole('region', {name: '项目展示模式', exact: true})
		await overview.getByRole('combobox', {name: '进展显示范围', exact: true}).selectOption('1')
		await overview.locator('.task-own-progress > summary').click()
		const entries = overview.locator(`[data-task-id="${task.id}"] .history-entry`)
		await entries.scrollIntoViewIfNeeded()
		await expect(entries).toHaveCount(1)
		await expect(entries).toContainText('自动保存补充复测安排。')
		await expect(entries).toContainText('初次测试认为设备已稳定。')
		await expect(entries).toContainText('复核记录：仍需延长连续运行测试。')
		await expect(entries).not.toContainText('来源后来修订')
		await expect(entries.locator('img')).toHaveCount(1)
		await expect(overview.locator('.outstanding-cell')).toContainText('继续确认长时间运行结果')
		const bannerClose = page.getByRole('button', {name: 'Close banner', exact: true})
		if (await bannerClose.isVisible()) await bannerClose.click()
		await page.evaluate(() => window.scrollTo(0, 0))
		await page.screenshot({path: path.join(root!, 'progress-references-desktop.png'), fullPage: true, animations: 'disabled'})
		await page.setViewportSize({width: 390, height: 844})
		await expect(page.getByRole('button', {name: '显示菜单', exact: true})).toHaveAttribute('aria-expanded', 'false')
		if (await bannerClose.isVisible()) await bannerClose.click()
		await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy()
		await page.evaluate(() => window.scrollTo(0, 0))
		await page.screenshot({path: path.join(root!, 'progress-references-mobile.png'), fullPage: true, animations: 'disabled'})
		const deleted = await request.delete(`${base}/api/v2/tasks/${task.id}/comments/${second.id}`, {headers})
		expect(deleted.ok()).toBeTruthy()
		await page.setViewportSize({width: 1440, height: 1000})
		const unavailablePopup = context.waitForEvent('page')
		await entries.locator(`a[href$="#comment-${second.id}"]`).click()
		const missingSource = await unavailablePopup
		await expect(missingSource.getByText(/原记录暂时无法读取，可能已删除或无访问权限/)).toBeVisible()
		await missingSource.close()
		await expect(entries).toContainText('复核记录：仍需延长连续运行测试。')
	} finally {
		await request.delete(`${base}/api/v2/projects/${project.id}`, {headers}).catch(() => undefined)
	}
})

test('reference-only progress saves and reference editor controls fit desktop and mobile', async ({page, request}) => {
	test.setTimeout(45000)
	page.setDefaultTimeout(10000)
	const root = process.env.TASKTRACE_LOCAL_TEST_DIR
	test.skip(!root, 'Requires a running isolated portable test instance')
	const sessionPath = path.join(root!, 'data/reference-editor-browser-session.json')
	execFileSync(path.join(root!, 'TaskTrace-server.exe'), ['--config', path.join(root!, 'data/local-config.yml'), 'tasktrace-local-session', '--output', sessionPath, '--user-id', readFileSync(path.join(root!, 'data/local-user-id.txt'), 'utf8').trim()], {windowsHide: true, stdio: 'pipe'})
	const session = JSON.parse(readFileSync(sessionPath, 'utf8').replace(/^\uFEFF/, ''))
	const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
	const headers = {Authorization: 'Bearer ' + session.token, 'X-TaskTrace-Undo': '1'}
	async function create(url: string, data: object) {
		const response = await request.post(base + '/api/v2' + url, {headers, data})
		if (!response.ok()) throw new Error(`${url}: HTTP ${response.status()} ${await response.text()}`)
		const value = await response.json()
		await delay(100)
		return value
	}
	const project = await create('/projects', {title: '引用表单界面验收'})
	try {
		const task = await create(`/projects/${project.id}/tasks`, {title: '保留原记录并补充更正'})
		const upload = await request.post(`${base}/api/v2/tasks/${task.id}/attachments`, {headers, multipart: {files: {name: 'reference.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6AAAAAElFTkSuQmCC', 'base64')}}})
		expect(upload.ok()).toBeTruthy()
		const attachmentId = (await upload.json()).success[0].id
		await delay(100)
		await create(`/tasks/${task.id}/comments`, {comment: `<h3>每日进展 · 2022-03-01</h3><p>第一轮测试暂未发现异常，已记录测试截图。</p><img src="/api/v1/tasks/${task.id}/attachments/${attachmentId}">`})
		await create(`/tasks/${task.id}/comments`, {comment: '<h3>每日进展 · 2022-03-03</h3><p>补充持续运行测试，确认温度变化时的表现。</p>'})
		await create(`/tasks/${task.id}/comments`, {comment: '<h3>TaskTrace 遗留事项清单</h3><ul></ul>'})
		await page.addInitScript(() => localStorage.setItem('tasktrace-autosave', JSON.stringify({enabled: false, seconds: 5})))
		await page.setViewportSize({width: 1440, height: 1300})
		await page.goto(`${base}/tasks/${task.id}#tasktrace-local=` + encodeURIComponent(JSON.stringify(session)))
		const daily = page.locator('.daily-progress')
		await daily.getByLabel('记录日期', {exact: true}).fill('2022-03-07')
		await expect(daily.getByLabel('今日进展', {exact: true})).toBeEnabled()
		const picker = daily.getByRole('combobox', {name: '选择要引用的进展日期', exact: true})
		const add = daily.getByRole('button', {name: '添加引用', exact: true})
		const references = daily.getByRole('region', {name: '已引用的历史进展', exact: true})
		for (const date of ['2022-03-01', '2022-03-03']) {
			await picker.selectOption(date)
			await add.click()
		}
		await expect(references.locator('.progress-reference')).toHaveCount(2)
		await expect(daily.getByLabel('今日进展', {exact: true})).toHaveValue('')
		await daily.getByRole('button', {name: '保存进展', exact: true}).click()
		await expect(daily.locator('.daily-progress-actions [role=status]')).toContainText('当天进展已保存')
		const history = await (await request.get(`${base}/api/v2/tasks/${task.id}/comments`, {headers})).json()
		const saved = history.items.find((note: Comment) => note.comment.includes('每日进展 · 2022-03-07</h3>'))
		expect(saved.comment.match(/data-tasktrace-reference="1"/g)).toHaveLength(2)
		await expect.poll(() => references.locator('img').evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0)
		await daily.screenshot({path: path.join(root!, 'progress-reference-editor-desktop.png'), animations: 'disabled'})
		await page.setViewportSize({width: 390, height: 844})
		await expect(page.getByRole('button', {name: '显示菜单', exact: true})).toHaveAttribute('aria-expanded', 'false')
		const bannerClose = page.getByRole('button', {name: 'Close banner', exact: true})
		if (await bannerClose.isVisible()) await bannerClose.click()
		await references.getByRole('button', {name: '移除 2022-03-03 的引用', exact: true}).click()
		await expect(references.locator('.progress-reference')).toHaveCount(1)
		await picker.selectOption('2022-03-03')
		await add.click()
		await expect(references.locator('.progress-reference')).toHaveCount(2)
		await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy()
		const controls = daily.locator('.reference-picker select, .reference-picker button, .progress-reference button, .progress-reference a, .progress-reference img')
		for (const control of await controls.all()) {
			const box = await control.boundingBox()
			expect(box).not.toBeNull()
			expect(box!.x).toBeGreaterThanOrEqual(0)
			expect(box!.x + box!.width).toBeLessThanOrEqual(390)
		}
		await daily.screenshot({path: path.join(root!, 'progress-reference-editor-mobile.png'), animations: 'disabled'})
	} finally {
		await request.delete(`${base}/api/v2/projects/${project.id}`, {headers}).catch(() => undefined)
	}
})
