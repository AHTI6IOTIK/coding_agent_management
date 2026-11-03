import path from "node:path"
import type { Plugin } from "@opencode-ai/plugin"

const editingTools = new Set(["edit", "write"])
const patchFilePattern = /^\*\*\* (?:Add|Update|Delete) File: (.+)$|^\*\*\* Move to: (.+)$/gm
const lockFiles = new Set([
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"bun.lockb",
	"bun.lock",
	"go.sum",
	"Cargo.lock",
	"composer.lock",
	"Gemfile.lock",
	"Podfile.lock",
])

function isLockFile(name: string): boolean {
	return lockFiles.has(name) || name.endsWith(".lock") || name.includes(".lock.") || name.includes("-lock.")
}

export function isProtectedPath(filePath: string, directory: string): boolean {
	const parts = path.relative(directory, path.resolve(directory, filePath)).split(path.sep)
	const name = parts.at(-1) ?? ""

	return name === ".env" || parts.includes(".git") || isLockFile(name)
}

function protectedFilePath(tool: string, args: { filePath?: unknown; patchText?: unknown }, directory: string): string | undefined {
	if (editingTools.has(tool) && typeof args.filePath === "string" && isProtectedPath(args.filePath, directory)) {
		return args.filePath
	}

	if (tool !== "apply_patch" || typeof args.patchText !== "string") {
		return undefined
	}

	return [...args.patchText.matchAll(patchFilePattern)]
		.map((match) => match[1] ?? match[2])
		.find((filePath) => isProtectedPath(filePath, directory))
}

const protectFiles: Plugin = async ({ client, directory }) => ({
	"tool.execute.before": async (input, output) => {
		const filePath = protectedFilePath(input.tool, output.args ?? {}, directory)
		if (filePath) {
			await client.tui.showToast({
				body: {
					title: "Изменение отменено",
					message: `Защищённый файл нельзя изменять: ${filePath}`,
					variant: "error",
				},
				query: { directory },
			})
			throw new Error(`Защищённый файл нельзя изменять: ${filePath}`)
		}
	},
})

export default protectFiles
