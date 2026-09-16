// This module runs before router/auth imports so the private fragment never reaches telemetry.
export const LOCAL_REFRESH_KEY = 'tasktraceLocalRefresh'
export const isLocalBuild = import.meta.env.VITE_TASKTRACE_LOCAL === 'true'
	&& window.location.protocol === 'http:'
	&& window.location.hostname === '127.0.0.1'

export function consumeLocalSession(): void {
	if (!isLocalBuild || !window.location.hash.startsWith('#tasktrace-local=')) return
	const encoded = window.location.hash.slice('#tasktrace-local='.length)
	window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search)
	try {
		const session = JSON.parse(decodeURIComponent(encoded))
		if (typeof session.token !== 'string' || typeof session.refresh_token !== 'string'
			|| session.token.split('.').length !== 3 || !session.refresh_token) return
		localStorage.setItem('token', session.token)
		localStorage.setItem(LOCAL_REFRESH_KEY, session.refresh_token)
		localStorage.removeItem('API_URL')
		localStorage.removeItem('desktopOAuthRefreshToken')
		sessionStorage.removeItem('justLoggedOut')
	} catch {
		// A malformed launch link must not change an existing session.
	}
}

export function isLocalWorkspace(): boolean {
	return isLocalBuild && !!localStorage.getItem(LOCAL_REFRESH_KEY)
}

consumeLocalSession()
