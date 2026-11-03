import type { Plugin } from "@opencode-ai/plugin"

const editingTools = new Set(["edit", "write"])
const patchFilePattern = /^\*\*\* (?:Add|Update) File: (.+)$|^\*\*\* Move to: (.+)$/gm

function changedGoFiles(input: { tool: string; args?: { filePath?: unknown; patchText?: unknown } }): string[] {
	if (editingTools.has(input.tool) && typeof input.args?.filePath === "string" && input.args.filePath.endsWith(".go")) {
		return [input.args.filePath]
	}

	if (input.tool !== "apply_patch" || typeof input.args?.patchText !== "string") {
		return []
	}

	return [...new Set([...input.args.patchText.matchAll(patchFilePattern)].map((match) => match[1] ?? match[2]).filter((path) => path.endsWith(".go")))]
}

function formatResult(name: string, result: { exitCode: number; stdout: Buffer; stderr: Buffer }): string {
	const details = [result.stdout.toString().trim(), result.stderr.toString().trim()].filter(Boolean).join("\n")
	const status = result.exitCode === 0 ? "пройдено" : `не пройдено (код выхода ${result.exitCode})`

	return `${name}: ${status}${details ? `\n${details}` : ""}`
}

const goQuality: Plugin = async ({ directory, $ }) => ({
	"tool.execute.after": async (input, output) => {
		const filePaths = changedGoFiles(input)
		if (filePaths.length === 0) {
			return
		}

		const shell = $.cwd(directory).nothrow()
		const results = (
			await Promise.all(
				filePaths.flatMap(async (filePath) => [
					["gofmt", await shell`gofmt -w ${filePath}`],
					["go vet", await shell`go vet ${filePath}`],
					["golangci-lint", await shell`golangci-lint run ${filePath}`],
				] as const),
			),
		).flat()

		output.output += `${output.output ? "\n\n" : ""}Проверки Go:\n${results
			.map(([name, result]) => formatResult(name, result))
			.join("\n")}`
	},
})

export default goQuality
