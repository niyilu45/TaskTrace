import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {Editor} from '@tiptap/core'
import {TextSelection} from '@tiptap/pm/state'
import {Slice} from '@tiptap/pm/model'
import {ref} from 'vue'
import {createPinia, setActivePinia} from 'pinia'
import {createEditorExtensions, type EditorExtensionDeps} from './editorExtensions'
import {insertClipboardHTMLImages, portableClipboardHTML} from './pasteFormatting'
import {fetchAttachmentBlobUrl} from '@/helpers/attachments'
import {dataUrlAsFile, fileAsDataUrl} from '@/helpers/tasktraceDraftCache'

vi.mock('@/message', () => ({error: vi.fn()}))
vi.mock('@/helpers/attachments', () => ({fetchAttachmentBlobUrl: vi.fn(async () => 'blob:source-image')}))
vi.mock('@/helpers/tasktraceDraftCache', () => ({
	dataUrlAsFile: vi.fn(async (_url: string, name: string, type: string) => new File([new Uint8Array([1, 2, 3])], name, {type})),
	fileAsDataUrl: vi.fn(async (file: File) => `data:${file.type};base64,AQID`),
}))
const editors: Editor[] = []
function editor(uploadAndInsertFiles: EditorExtensionDeps['uploadAndInsertFiles'] = () => {}) {
	let instance: Editor
	const deps: EditorExtensionDeps = {
		t: key => key, isEditing: ref(true), isEditEnabled: () => true, placeholder: '', contentHasChanged: ref(false),
		bubbleSave: () => {}, getEditor: () => instance, uploadCallback: async () => [], uploadAndInsertFiles, allowBase64Images: true,
	}
	const element = document.createElement('div'); document.body.append(element)
	instance = new Editor({element, extensions: createEditorExtensions(deps), content: '<p>前后</p>'})
	instance.commands.setTextSelection(2)
	editors.push(instance)
	return instance
}
beforeEach(() => {
	setActivePinia(createPinia()); vi.clearAllMocks()
	window.API_URL = `${window.location.origin}/api/v1`
	let id = 0
	vi.spyOn(URL, 'createObjectURL').mockImplementation(() => `blob:clipboard-${++id}`)
	vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
})
afterEach(() => {editors.forEach(editor => editor.destroy()); editors.length = 0; document.body.innerHTML = ''; vi.restoreAllMocks(); vi.unstubAllGlobals()})

describe('clipboard media between floating and web editors', () => {
	it('reserves the original caret and every image/file position while the cursor moves during upload', async () => {
		const target = editor()
		let finish!: (urls: string[]) => void
		const upload = vi.fn((_files: File[] | FileList) => new Promise<string[]>(resolve => {finish = resolve}))
		const work = insertClipboardHTMLImages(target, '<p>甲<img src="data:image/png;base64,AQID">乙<img src="data:image/png;base64,BAUG">丙<a download="记录.txt" href="data:application/octet-stream;base64,AQID">记录.txt</a>丁</p>', [], upload)
		await vi.waitFor(() => expect(upload).toHaveBeenCalledTimes(1))
		expect(upload.mock.calls[0][0]).toHaveLength(3)
		target.view.dispatch(target.state.tr.setSelection(TextSelection.atEnd(target.state.doc)))
		target.commands.insertContent('继续输入')
		finish(['data:image/png;base64,AQID', 'data:image/png;base64,BAUG', 'data:application/octet-stream;base64,AQID'])
		await work
		const result = target.getHTML()
		expect(result.match(/<img /g)).toHaveLength(2)
		expect(result).toContain('download="记录.txt"')
		expect(result).toContain('href="data:application/octet-stream;base64,AQID"')
		expect(result.indexOf('甲')).toBeLessThan(result.indexOf('AQID'))
		expect(result.indexOf('AQID')).toBeLessThan(result.indexOf('乙'))
		expect(result.indexOf('BAUG')).toBeLessThan(result.indexOf('丙'))
		expect(result.indexOf('丁')).toBeLessThan(result.indexOf('继续输入'))
		expect(result).not.toMatch(/blob:clipboard|UPLOAD_PLACEHOLDER/)
	})
	it('pastes all clipboard files once, and does not duplicate CF_HTML plus the same bitmap', async () => {
		const target = editor()
		const bitmap = new File(['image'], 'clip.png', {type: 'image/png'})
		const upload = vi.fn(async (files: File[] | FileList) => Array.from(files).map(file => file.type.startsWith('image/') ? 'data:image/png;base64,AQID' : 'data:application/octet-stream;base64,AQID'))
		await insertClipboardHTMLImages(target, '<p>图<img src="data:image/png;base64,AQID">文</p>', [bitmap], upload)
		expect(upload.mock.calls[0][0]).toHaveLength(1)
		expect(target.getHTML().match(/<img /g)).toHaveLength(1)
		await insertClipboardHTMLImages(target, '', [new File(['report'], 'report.txt', {type: 'text/plain'}), bitmap], upload)
		expect(upload.mock.calls[1][0]).toHaveLength(2)
		expect(target.getHTML()).toContain('download="report.txt"')
		expect(target.getHTML().match(/<img /g)).toHaveLength(2)
	})
	it('keeps recoverable draft bytes when a media upload fails', async () => {
		const target = editor()
		await expect(insertClipboardHTMLImages(target, '', [new File(['image'], 'clip.png', {type: 'image/png'})], async () => {throw new Error('offline')})).rejects.toThrow('offline')
		expect(target.getHTML()).toContain('data:image/png;base64,AQID')
		expect(target.getHTML()).not.toContain('blob:clipboard')
	})
	it('turns authenticated attachment images/files into portable bytes for a native window', async () => {
		const copied = await portableClipboardHTML('<p>前<img data-src="/api/v1/tasks/8/attachments/2" src="#">后<a href="/api/v1/tasks/8/attachments/3" download="报告.txt">报告.txt</a></p>')
		expect(fetchAttachmentBlobUrl).toHaveBeenCalledWith({taskId: 8, id: 2})
		expect(fetchAttachmentBlobUrl).toHaveBeenCalledWith({taskId: 8, id: 3})
		expect(copied).toContain('src="data:image/png;base64,AQID"')
		expect(copied).toContain('href="data:application/octet-stream;base64,AQID"')
		expect(copied).not.toMatch(/blob:|data-src|\/tasks\/8/)
		expect(fileAsDataUrl).toHaveBeenCalledTimes(2)
	})
	it('passes every file through the real editor paste handler rather than stopping at the first bitmap', () => {
		const receive = vi.fn(); const target = editor(receive)
		const files = [new File(['a'], 'a.png', {type: 'image/png'}), new File(['b'], 'b.txt', {type: 'text/plain'})]
		const event = {clipboardData: {files, items: [], getData: () => ''}} as unknown as ClipboardEvent
		target.view.someProp('handlePaste', handler => handler(target.view, event, Slice.empty))
		expect(receive).toHaveBeenCalledOnce()
		expect(receive).toHaveBeenCalledWith(files, '')
	})
	it('exports the actual copy gesture as standard HTML and plain Unicode text', async () => {
		const target = editor()
		target.commands.setContent('<p>图前</p><img src="data:image/png;base64,AQID"><p>图后</p>')
		target.commands.selectAll()
		let copied = ''
		class TestClipboardItem {constructor(public values: Record<string, Blob | Promise<Blob>>) {}}
		vi.stubGlobal('ClipboardItem', TestClipboardItem)
		const write = vi.fn(async (items: TestClipboardItem[]) => {copied = await (await items[0].values['text/html']).text()})
		vi.stubGlobal('navigator', {...navigator, clipboard: {write}})
		const data = new Map<string, string>()
		const event = {clipboardData: {setData: (type: string, value: string) => data.set(type, value)}, preventDefault: vi.fn()} as unknown as ClipboardEvent
		target.view.someProp('handleDOMEvents', handlers => handlers.copy?.(target.view, event))
		await vi.waitFor(() => expect(copied).toContain('data:image/png;base64,AQID'))
		expect(data.get('text/plain')).toContain('图前')
		expect(data.get('text/plain')).toContain('图后')
		expect(event.preventDefault).toHaveBeenCalledOnce()
		expect(target.getHTML()).toContain('data:image/png;base64,AQID')
	})
	it('does not leave a broken temporary image after clipboard bytes cannot be read', async () => {
		const target = editor()
		vi.mocked(dataUrlAsFile).mockRejectedValueOnce(new Error('unavailable'))
		await expect(insertClipboardHTMLImages(target, '<img src="blob:unavailable">', [], async () => [])).rejects.toThrow('unavailable')
		expect(target.getHTML()).not.toMatch(/<img|blob:clipboard/)
		expect(target.getText()).toContain('前')
		expect(target.getText()).toContain('后')
	})
	it('rejects authenticated attachment lookup on another server', async () => {
		await expect(portableClipboardHTML('<img src="https://unrelated.invalid/api/v1/tasks/8/attachments/2">')).rejects.toThrow('其它服务器')
		expect(fetchAttachmentBlobUrl).not.toHaveBeenCalled()
		expect(dataUrlAsFile).not.toHaveBeenCalled()
	})
})
