import { describe, expect, mock, test } from "bun:test"
import protectFiles, { isProtectedPath } from "../plugin/protect-files"

describe("isProtectedPath", () => {
	const directory = "/workspace"

	test("защищает .env, .git и типовые lock-файлы", () => {
		expect(isProtectedPath("/workspace/service/.env", directory)).toBe(true)
		expect(isProtectedPath("/workspace/.git/config", directory)).toBe(true)
		expect(isProtectedPath("/workspace/go.sum", directory)).toBe(true)
		expect(isProtectedPath("/workspace/node_modules/example/package-lock.json", directory)).toBe(true)
		expect(isProtectedPath("/workspace/cache/build.lock.tmp", directory)).toBe(true)
	})

	test("пропускает обычные файлы и .env.example", () => {
		expect(isProtectedPath("/workspace/internal/main.go", directory)).toBe(false)
		expect(isProtectedPath("/workspace/.env.example", directory)).toBe(false)
	})
})

function createClient() {
	const showToast = mock(() => Promise.resolve({}))

	return { client: { tui: { showToast } }, showToast }
}

test("показывает toast и блокирует защищённый файл до записи", async () => {
	const { client, showToast } = createClient()
	const hooks = await protectFiles({ client, directory: "/workspace" } as never)

	await expect(
		hooks["tool.execute.before"]?.(
			{ tool: "write", sessionID: "session", callID: "call" },
			{ args: { filePath: "/workspace/.env" } },
		),
	).rejects.toThrow("Защищённый файл")
	expect(showToast).toHaveBeenCalledWith({
		body: {
			title: "Изменение отменено",
			message: "Защищённый файл нельзя изменять: /workspace/.env",
			variant: "error",
		},
		query: { directory: "/workspace" },
	})
})

test("показывает toast и блокирует patch для защищённого файла", async () => {
	const { client, showToast } = createClient()
	const hooks = await protectFiles({ client, directory: "/workspace" } as never)

	await expect(
		hooks["tool.execute.before"]?.(
			{ tool: "apply_patch", sessionID: "session", callID: "call" },
			{ args: { patchText: "*** Begin Patch\n*** Update File: .env\n*** End Patch" } },
		),
	).rejects.toThrow("Защищённый файл")
	expect(showToast).toHaveBeenCalledTimes(1)
})

test("не блокирует patch только с обычным файлом", async () => {
	const { client, showToast } = createClient()
	const hooks = await protectFiles({ client, directory: "/workspace" } as never)

	await hooks["tool.execute.before"]?.(
		{ tool: "apply_patch", sessionID: "session", callID: "call" },
		{ args: { patchText: "*** Begin Patch\n*** Update File: README.md\n*** End Patch" } },
	)
	expect(showToast).not.toHaveBeenCalled()
})
