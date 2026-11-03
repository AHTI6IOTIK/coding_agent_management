import { expect, test } from "bun:test"
import goQuality from "../plugin/go-quality"

function createShell(exitCode = 0) {
	const commands: string[] = []
	const shell = (strings: TemplateStringsArray, ...values: string[]) => {
		commands.push(strings.reduce((command, part, index) => command + part + (values[index] ?? ""), ""))
		return Promise.resolve({
			exitCode,
			stdout: Buffer.from(""),
			stderr: Buffer.from(exitCode === 0 ? "" : "check failed"),
		})
	}

	return Object.assign(shell, {
		commands,
		cwd: () => shell,
		nothrow: () => shell,
	})
}

test("запускает форматирование и проверки после правки Go-файла", async () => {
	const $ = createShell()
	const hooks = await goQuality({ directory: "/workspace", $ } as never)
	const output = { title: "edit", output: "", metadata: {} }

	await hooks["tool.execute.after"]?.(
		{ tool: "edit", sessionID: "session", callID: "call", args: { filePath: "/workspace/internal/main.go" } },
		output,
	)

	expect($.commands).toEqual([
		"gofmt -w /workspace/internal/main.go",
		"go vet /workspace/internal/main.go",
		"golangci-lint run /workspace/internal/main.go",
	])
	expect(output.output).toContain("gofmt: пройдено")
})

test("запускает проверки для Go-файлов из apply_patch", async () => {
	const $ = createShell()
	const hooks = await goQuality({ directory: "/workspace", $ } as never)
	const output = { title: "apply_patch", output: "", metadata: {} }

	await hooks["tool.execute.after"]?.(
		{
			tool: "apply_patch",
			sessionID: "session",
			callID: "call",
			args: {
				patchText: "*** Begin Patch\n*** Update File: internal/main.go\n*** Update File: README.md\n*** End Patch",
			},
		},
		output,
	)

	expect($.commands).toEqual([
		"gofmt -w internal/main.go",
		"go vet internal/main.go",
		"golangci-lint run internal/main.go",
	])
})

test("не запускает проверки после правки не-Go-файла", async () => {
	const $ = createShell()
	const hooks = await goQuality({ directory: "/workspace", $ } as never)

	await hooks["tool.execute.after"]?.(
		{ tool: "write", sessionID: "session", callID: "call", args: { filePath: "/workspace/README.md" } },
		{ title: "write", output: "", metadata: {} },
	)

	expect($.commands).toEqual([])
})

test("сообщает об ошибке линтера без отката правки", async () => {
	const $ = createShell(1)
	const hooks = await goQuality({ directory: "/workspace", $ } as never)
	const output = { title: "edit", output: "", metadata: {} }

	await hooks["tool.execute.after"]?.(
		{ tool: "edit", sessionID: "session", callID: "call", args: { filePath: "/workspace/main_test.go" } },
		output,
	)

	expect(output.output).toContain("golangci-lint: не пройдено")
})
