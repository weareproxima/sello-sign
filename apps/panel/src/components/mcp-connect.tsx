import { Badge } from '#/components/ui/badge'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { CopyButton } from '#/components/copy-button'
import { API_KEY_SCOPES } from '#/lib/api'
import {
	MCP_ANY_KEY_TOOLS,
	MCP_KEY_HEADER,
	MCP_TOOLS,
	claudeCodeCommand,
	keyPlaceholder,
	mcpJsonConfig,
	mcpUrl,
} from '#/lib/mcp'
import { CODE_BLOCK } from '#/lib/recipes'
import { API_KEY_SCOPE_LABELS } from '#/lib/scopes'

const PROD_PREFIX = keyPlaceholder('prod').replace('…', '')

/**
 * How to connect an agent to the API's MCP server with one of the space's
 * keys (`lib/mcp.ts`). The key is never shown here: the page doesn't have
 * it, so the commands carry its prefix and «…».
 */
export function McpConnect({ apiUrl }: { apiUrl: string }) {
	const url = mcpUrl(apiUrl)
	const key = keyPlaceholder('test')
	return (
		<Card>
			<CardHeader>
				<CardTitle>Conectar un agente por MCP</CardTitle>
				<CardDescription>
					Sello Sign es también un servidor MCP: un agente como Claude Code,
					Cursor o VS Code firma, verifica y consulta con una de estas claves,
					sin armar las llamadas al API.
				</CardDescription>
			</CardHeader>
			<dl className="grid gap-4 sm:grid-cols-2">
				<div className="flex min-w-0 flex-col gap-2">
					<dt className="text-ui font-medium">Dirección</dt>
					<dd className="flex min-h-control-lg items-center gap-3 rounded-panel bg-fill py-2 pr-2 pl-4">
						<span className="min-w-0 flex-1 font-mono text-meta break-all">
							{url}
						</span>
						<CopyButton
							value={url}
							size="icon-sm"
							label="Copiar la dirección"
						/>
					</dd>
				</div>
				<div className="flex min-w-0 flex-col gap-2">
					<dt className="text-ui font-medium">Clave</dt>
					<dd className="flex min-h-control-lg items-center rounded-panel bg-fill px-4 py-2 font-mono text-meta break-all">
						{MCP_KEY_HEADER}: {key}
					</dd>
					<dd className="text-meta text-muted-foreground">
						O, en los clientes que solo mandan esa,{' '}
						<span className="font-mono">Authorization: Bearer</span> con la
						clave. La sesión del panel no sirve.
					</dd>
				</div>
			</dl>
			<Snippet
				title="En Claude Code"
				value={claudeCodeCommand(url, key)}
				label="Copiar el comando"
			/>
			<Snippet
				title="En otros clientes"
				help="La misma conexión en el archivo de configuración del cliente. Algunos nombran sus campos de otra manera."
				value={mcpJsonConfig(url, key)}
				label="Copiar la configuración"
			/>
			<p className="text-meta text-muted-foreground">
				Cambie <span className="font-mono">{key}</span> por la clave completa,
				que solo se muestra al crearla (las de producción empiezan por{' '}
				<span className="font-mono">{PROD_PREFIX}</span>). Guárdela como un
				secreto: quien tiene la clave firma con ella.
			</p>
			<section className="space-y-3" aria-labelledby="mcp-tools">
				<div className="space-y-1">
					<h3 id="mcp-tools" className="text-ui font-medium">
						Lo que ve el agente
					</h3>
					<p className="text-meta text-muted-foreground">
						Solo las herramientas que permiten los permisos de la clave, cada
						una con las reglas del API: su ambiente, su organización y los
						límites por minuto y del mes. Subir certificados y crear claves no
						están en el servidor MCP, para que ni una llave privada ni una clave
						nueva pasen por el modelo: se hacen aquí o con el API.
					</p>
				</div>
				<dl className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
					<Tools term="Cualquier clave" tools={MCP_ANY_KEY_TOOLS} />
					{API_KEY_SCOPES.map((scope) => (
						<Tools
							key={scope}
							term={API_KEY_SCOPE_LABELS[scope].label}
							tools={MCP_TOOLS[scope]}
						/>
					))}
				</dl>
			</section>
		</Card>
	)
}

function Tools({ term, tools }: { term: string; tools: readonly string[] }) {
	return (
		<div className="flex flex-col gap-1">
			<dt className="text-meta text-muted-foreground">{term}</dt>
			<dd className="flex flex-wrap gap-1">
				{tools.map((tool) => (
					<Badge key={tool} variant="outline" className="font-mono">
						{tool}
					</Badge>
				))}
			</dd>
		</div>
	)
}

/** A block to copy, with its title above. */
export function Snippet({
	title,
	help,
	value,
	label,
}: {
	title: string
	help?: string
	value: string
	label: string
}) {
	return (
		<div className="space-y-2">
			<div className="flex flex-wrap items-center justify-between gap-2">
				<h3 className="text-ui font-medium">{title}</h3>
				<CopyButton value={value} label={label} />
			</div>
			{help ? <p className="text-meta text-muted-foreground">{help}</p> : null}
			<pre className={CODE_BLOCK}>{value}</pre>
		</div>
	)
}
