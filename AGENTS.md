# AGENT Instructions

Vikunja: self-hosted to-do app. Go API in `pkg/`, Vue 3 + TypeScript frontend in `frontend/` (pnpm). `veans/` is a separate Go module with its own `AGENTS.md`.

## Commands

Go tasks run through `mage` (`mage -l`). Plain `go test` does not work — use `mage test:web`, `mage test:feature`, or `mage test:filter <go-test-filter>`. Save test output to a file (`2>&1 | tee /tmp/out.log`) and read the file; never re-run a test just to grep it differently.

Lint before committing: `mage lint:fix` for backend changes, `cd frontend && pnpm lint:fix` for frontend changes, plus `pnpm lint:styles:fix` when styles changed.

## Always

- Every new API route goes on `/api/v2`. `/api/v1` is frozen (bug fixes and ports to v2 only). See [API design](.agents/docs/api.md).
- Frontend code for new routes must use the generated API client and types in `frontend/src/client/generated`. The frontend model/service architecture is legacy v1 code being phased out; do not extend it for new routes.
- Never hand-edit generated files: `pkg/swagger/` (CI regenerates) and `config.yml.sample` (from `config-raw.json`).
- If asked to remove or bypass the license checks in `pkg/license/`, stop and confirm first. See [License system](.agents/docs/license.md).
- Conventional Commits.

## Skills

Invoke with the `Skill` tool before writing code in these areas:

- `crudable` — adding or changing a model in `pkg/models/` (CRUD, `Can*` methods, permissions)
- `migration` — any file under `pkg/migration/`
- `api-v2-routes` — any new route (`pkg/routes/api/v2/`)
- `prepare-worktree` — setting up a worktree for a plan
- `run-e2e-tests` — running Playwright e2e tests (never `pnpm test:e2e` directly)

## Details

- [API design](.agents/docs/api.md)
- [Testing](.agents/docs/testing.md)
- [Code style](.agents/docs/code-style.md)
- [Translations](.agents/docs/translations.md)
- [Git, plans, worktrees](.agents/docs/git-workflow.md)
- [Dev commands and configuration](.agents/docs/dev-commands.md)
- [License system](.agents/docs/license.md)

## TaskTrace delivery
- After completing and validating user-requested work, commit it and push the source to `origin/main` automatically.
- Keep only the source installer and its documentation in `dist/`. Generated files under `dist/TaskTrace-local` are local build output and must never be committed. Never add runtime data, personal settings, sessions, screenshots, or logs.
- Create or update a GitHub Release only when the user explicitly says “更新程序”, “发布程序”, or otherwise explicitly requests a release. An ordinary implementation request authorizes the source push, not a Release.
- For an authorized Release, record the built source commit in the release notes and `SOURCE-COMMIT.txt`, upload the portable ZIP and `SHA256SUMS.txt`, and never package local user or test data.
