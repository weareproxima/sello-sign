/**
 * How to connect an agent to the API's MCP server (`POST /mcp`,
 * `apps/api/internal/httpapi/mcp.go`; «Servidor MCP» of
 * `contracts/README.md`): Streamable HTTP at `<API_URL>/mcp`, with one of the
 * space's API keys in `X-API-Key` (or `Authorization: Bearer sign_…`); the
 * panel's token isn't one. Each request lists only the tools the key's
 * scopes allow, and uploading certificates and creating keys are left out on
 * purpose.
 *
 * Pure: the «Claves de API» page shows what this builds, and `mcp.test.ts`
 * compares `MCP_TOOLS` with the tools `mcp.go` registers.
 */
import type { ApiKeyScope, Environment } from './api'

/** The server's name in the agent's configuration. */
export const MCP_SERVER_NAME = 'sello-sign'

/** The header that carries the key. */
export const MCP_KEY_HEADER = 'X-API-Key'

/** The MCP server of the API whose public origin is `apiUrl`. */
export function mcpUrl(apiUrl: string): string {
	return `${apiUrl.replace(/\/+$/, '')}/mcp`
}

/** What stands for a key that the page doesn't show: its prefix and «…». */
export function keyPlaceholder(environment: Environment): string {
	return `sign_${environment}_…`
}

/** Claude Code's command to add the server. */
export function claudeCodeCommand(url: string, key: string): string {
	return `claude mcp add --transport http ${MCP_SERVER_NAME} ${url} --header "${MCP_KEY_HEADER}: ${key}"`
}

/** The same connection as the `mcpServers` JSON that other clients read. */
export function mcpJsonConfig(url: string, key: string): string {
	return JSON.stringify(
		{
			mcpServers: {
				[MCP_SERVER_NAME]: {
					type: 'http',
					url,
					headers: { [MCP_KEY_HEADER]: key },
				},
			},
		},
		null,
		2
	)
}

/** The tools any key sees, whatever its scopes. */
export const MCP_ANY_KEY_TOOLS = ['get_workspace'] as const

/** The tools each scope lets a key see (the table of «Servidor MCP» in
 * `contracts/README.md`). */
export const MCP_TOOLS: Record<ApiKeyScope, readonly string[]> = {
	sign: ['sign_document', 'verify_signature'],
	'organizations:read': ['list_organizations', 'get_organization'],
	'organizations:write': [
		'create_organization',
		'update_organization',
		'delete_organization',
	],
	'certificates:read': ['list_certificates'],
	'certificates:write': [
		'activate_certificate',
		'deactivate_certificate',
		'delete_certificate',
	],
	'delegations:read': ['list_delegations'],
	'delegations:write': ['create_delegation', 'delete_delegation'],
	'keys:read': ['list_api_keys'],
	'keys:write': ['revoke_api_key'],
	'logs:read': ['list_signatures', 'get_signature', 'get_usage'],
}

/** The tools a key with `scopes` sees, in the order of `MCP_TOOLS`. */
export function toolsFor(scopes: readonly ApiKeyScope[]): string[] {
	const out: string[] = [...MCP_ANY_KEY_TOOLS]
	for (const [scope, tools] of Object.entries(MCP_TOOLS)) {
		if (scopes.includes(scope as ApiKeyScope)) out.push(...tools)
	}
	return out
}
