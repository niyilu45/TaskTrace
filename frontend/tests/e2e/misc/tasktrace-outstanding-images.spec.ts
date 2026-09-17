import {test, expect, type Locator} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'
import {execFileSync} from 'node:child_process'

test.use({serviceWorkers: 'block'})

async function pastePicture(input: Locator) {
	await input.evaluate(element => {
		const canvas = document.createElement('canvas')
		canvas.width = 320
		canvas.height = 160
		const context = canvas.getContext('2d')!
		context.fillStyle = '#eaf1fa'
		context.fillRect(0, 0, 320, 160)
		context.fillStyle = '#285b92'
		context.font = '22px sans-serif'
		context.fillText('TaskTrace image test', 24, 75)
		const bytes = Uint8Array.from(atob(canvas.toDataURL().split(',')[1]), value => value.charCodeAt(0))
		const clipboard = new DataTransfer()
		clipboard.items.add(new File([bytes], 'outstanding-clipboard.png', {type: 'image/png'}))
		element.dispatchEvent(new ClipboardEvent('paste', {clipboardData: clipboard, bubbles: true, cancelable: true}))
	})
}

test('outstanding images support paste, append, numbered display and persistence without becoming daily progress', async ({page, request}) => {
	test.setTimeout(90000)
	const root = process.env.TASKTRACE_LOCAL_TEST_DIR
	test.skip(!root, 'Requires a running isolated portable test instance')
	const sessionPath = path.join(root!, 'data/outstanding-images-browser-session.json')
	execFileSync(path.join(root!, 'TaskTrace-server.exe'), ['--config', path.join(root!, 'data/local-config.yml'), 'tasktrace-local-session', '--output', sessionPath, '--user-id', readFileSync(path.join(root!, 'data/local-user-id.txt'), 'utf8').trim()], {windowsHide: true, stdio: 'pipe'})
	const session = JSON.parse(readFileSync(sessionPath, 'utf8').replace(/^\uFEFF/, ''))
	const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
	const headers = {Authorization: 'Bearer ' + session.token}
	const projectResponse = await request.post(`${base}/api/v2/projects`, {headers, data: {title: '遗留事项图片验收'}})
	expect(projectResponse.ok()).toBeTruthy()
	const project = await projectResponse.json()
	try {
		const taskResponse = await request.post(`${base}/api/v2/projects/${project.id}/tasks`, {headers, data: {title: '设备调试与截图记录'}})
		expect(taskResponse.ok()).toBeTruthy()
		const task = await taskResponse.json()
		await page.goto(`${base}/tasks/${task.id}#tasktrace-local=` + encodeURIComponent(JSON.stringify(session)))
		await page.getByRole('button', {name: /^(设置优先级|Set Priority)$/}).click()
		const priority = page.getByRole('combobox', {name: /^(优先级|Priority)$/})
		await expect(priority).toHaveValue('9')
		await expect(priority.locator('option')).toHaveCount(10)
		const readPriority = async () => (await (await request.get(`${base}/api/v2/tasks/${task.id}`, {headers})).json()).priority
		expect(await readPriority()).toBe(0)
		await priority.selectOption('0')
		await expect.poll(readPriority).toBe(10)
		await page.reload()
		await expect(priority).toHaveValue('0')
		await priority.selectOption('9')
		await expect.poll(readPriority).toBe(1)
		await page.reload()
		await expect(priority).toHaveValue('9')
		await priority.screenshot({path: path.join(root!, 'priority-local-default.png')})
		const daily = page.locator('.daily-progress')
		const shared = daily.getByRole('region', {name: '共享遗留事项'})
		await expect(shared.getByRole('textbox', {name: '新增遗留事项', exact: true})).toBeEnabled()
		await pastePicture(shared.getByRole('textbox', {name: '新增遗留事项', exact: true}))
		await expect(shared.locator('.outstanding-images img')).toHaveCount(1)
		await expect(daily.locator('.progress-images img')).toHaveCount(0)
		await shared.getByRole('button', {name: '添加遗留事项', exact: true}).click()
		await expect(shared.locator('ol.outstanding-list > li')).toHaveCount(1)
		await expect(shared.locator('.outstanding-images img')).toHaveCount(0)
		await expect.poll(() => shared.locator('.readonly-rich-text img').first().evaluate((image: HTMLImageElement) => image.naturalWidth)).toBe(320)
		await expect(shared.locator('ol')).toHaveCSS('list-style-type', 'decimal')

		await shared.getByRole('button', {name: '添加图片', exact: true}).click()
		await shared.getByRole('textbox', {name: '为第 1 条遗留事项添加图片', exact: true}).fill('追加图片说明：需要确认屏幕显示异常。')
		await shared.locator('input[type="file"]').setInputFiles({name: 'detail.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6AAAAAElFTkSuQmCC', 'base64')})
		await expect(shared.locator('.outstanding-images img')).toHaveCount(1)
		await shared.getByRole('button', {name: '保存图片', exact: true}).click()
		await expect(shared.locator('ol.outstanding-list > li')).toHaveCount(1)
		await expect(shared.locator('.readonly-rich-text img')).toHaveCount(2)
		await expect(shared).toContainText('追加图片说明')
		await expect(daily.locator('.progress-images img')).toHaveCount(0)

		await shared.getByRole('textbox', {name: '新增遗留事项', exact: true}).fill('等待复测结果，明天继续跟进。')
		await shared.getByRole('button', {name: '添加遗留事项', exact: true}).click()
		await expect(shared.locator('ol.outstanding-list > li')).toHaveCount(2)
		await page.reload()
		await expect(shared.locator('ol.outstanding-list > li')).toHaveCount(2)
		await expect(shared.locator('.readonly-rich-text img')).toHaveCount(2)
		await expect.poll(() => shared.locator('.readonly-rich-text img').evaluateAll(nodes => nodes.every(node => (node as HTMLImageElement).naturalWidth > 0))).toBeTruthy()
		await expect(shared.locator('ol > li').first()).toContainText('追加图片说明')
		await expect(shared.locator('ol > li').nth(1)).toContainText('等待复测结果')
		await daily.getByLabel('记录日期').fill('2026-09-16')
		await expect(shared.locator('ol.outstanding-list > li')).toHaveCount(2)
		await expect(daily.getByLabel('今日进展', {exact: true})).toHaveValue('')
		const commentsResponse = await request.get(`${base}/api/v2/tasks/${task.id}/comments`, {headers})
		expect(commentsResponse.ok()).toBeTruthy()
		const comments = await commentsResponse.json()
		expect(comments.items.filter((note: {comment: string}) => note.comment.includes('TaskTrace 遗留事项清单'))).toHaveLength(1)
		expect(comments.items.filter((note: {comment: string}) => note.comment.includes('每日进展 ·'))).toHaveLength(0)
		const installBanner = page.locator('.add-to-home-screen .hide-button')
		if (await installBanner.isVisible()) await installBanner.click()
		await shared.screenshot({path: path.join(root!, 'outstanding-images-desktop.png')})
		await page.setViewportSize({width: 390, height: 844})
		await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy()
		await shared.screenshot({path: path.join(root!, 'outstanding-images-mobile.png')})
	} finally {
		await request.delete(`${base}/api/v2/projects/${project.id}`, {headers})
	}
})
