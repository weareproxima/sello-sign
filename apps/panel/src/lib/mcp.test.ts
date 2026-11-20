import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { API_KEY_SCOPES } from './api'
import {
	MCP_ANY_KEY_TOOLS,
	MCP_TOOLS,
	claudeCodeCommand,
	keyPlaceholder,
	mcpJsonConfig,
	mcpUrl,
	toolsFor,
} from './mcp'

const read = (path: string) =>
	readFileSync(fileURLToPath(new URL(path, import.meta.url)), 'utf8')

/**
 * The tools `mcp.go` registers, with the scope each needs ("" for any key):
 * each entry of `mcpTools` is `{ scope: model.ScopeX, tool: &mcp.Tool{ Name:
 * "…"`, and the scopes' strings are the constants of `model.go`.
 */
function toolsOfTheApi(): Map<string, string> {
	const model = read('../../../api/internal/model/model.go')
	const scopes = new Map<string, string>()
	for (const [, name, value] of model.matchAll(
		/\b(Scope\w+)\s+Scope = "([^"]+)"/g
	))
		scopes.set(name!, value!)
	const mcp = read('../../../api/internal/httpapi/mcp.go')
	const list = mcp.slice(mcp.indexOf('var mcpTools = []mcpTool{'))
	const out = new Map<string, string>()
	for (const entry of list.split(/\n\t\{\n/).slice(1)) {
		const name = /\bName:\s+"(\w+)"/.exec(entry)?.[1]
		if (!name) continue
		const scope = /^\s*scope:\s+model\.(\w+),/m.exec(entry)?.[1]
		out.set(name, scope ? (scopes.get(scope) ?? `?${scope}`) : '')
	}
	return out
}

describe('MCP_TOOLS', () => {
	it('are the tools of the API, each under its scope', () => {
		const api = toolsOfTheApi()
		expect(api.size).toBeGreaterThan(10)
		const ours = new Map<string, string>()
		for (const tool of MCP_ANY_KEY_TOOLS) ours.set(tool, '')
		for (const [scope, tools] of Object.entries(MCP_TOOLS))
			for (const tool of tools) ours.set(tool, scope)
		expect(Object.fromEntries(ours)).toEqual(Object.fromEntries(api))
	})

	it('cover every scope of a key', () => {
		expect(Object.keys(MCP_TOOLS).sort()).toEqual([...API_KEY_SCOPES].sort())
	})

	it("list a key's tools by its scopes", () => {
		expect(toolsFor(['sign'])).toEqual([
			'get_workspace',
			'sign_document',
			'verify_signature',
		])
		expect(toolsFor([])).toEqual(['get_workspace'])
	})
})

describe('the connection', () => {
	const url = mcpUrl('https://api.sign.sello.tax/')

	it('is the MCP server of the API', () => {
		expect(url).toBe('https://api.sign.sello.tax/mcp')
	})

	it("is Claude Code's command, with the key's header", () => {
		expect(claudeCodeCommand(url, keyPlaceholder('test'))).toBe(
			'claude mcp add --transport http sello-sign https://api.sign.sello.tax/mcp --header "X-API-Key: sign_test_…"'
		)
	})

	it('is the JSON of the other clients', () => {
		expect(JSON.parse(mcpJsonConfig(url, 'sign_prod_x'))).toEqual({
			mcpServers: {
				'sello-sign': {
					type: 'http',
					url: 'https://api.sign.sello.tax/mcp',
					headers: { 'X-API-Key': 'sign_prod_x' },
				},
			},
		})
	})

	it('takes the header mcp.go reads', () => {
		const mcp = read('../../../api/internal/httpapi/mcp.go')
		expect(mcp).toContain('r.Header.Get("X-API-Key")')
	})
})
