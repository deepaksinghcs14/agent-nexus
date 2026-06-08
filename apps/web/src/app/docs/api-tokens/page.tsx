import { DocPage, Callout } from '@/components/docs/DocPage'
import ApiPlayground from '@/components/docs/ApiPlayground'

export const metadata = { title: 'API Tokens — Docs' }

export default function ApiTokensDoc() {
  return (
    <DocPage
      title="API Tokens"
      subtitle="Long-lived tokens for programmatic access. Use them in CI/CD pipelines, scripts, or any server-side code."
    >
      <h2>Token Format</h2>
      <p>
        All tokens begin with <code>anx_</code> followed by 40 hex characters. The full token is shown{' '}
        <strong>only once</strong> at creation — store it securely.
      </p>
      <pre><code>{`anx_a3f2c91b4e7d5f8a2c6e9b1d4f7a3c8e2b5d`}</code></pre>

      <Callout type="warning">
        <strong>Save your token immediately.</strong> The raw value is returned once and cannot be retrieved
        again. Agent Nexus only stores a SHA-256 hash of the token.
      </Callout>

      <h2>Using a Token</h2>
      <p>
        Pass the token in the <code>Authorization</code> header as a Bearer token on every request:
      </p>
      <pre><code>{`Authorization: Bearer anx_your_token_here`}</code></pre>

      <h2>Token Scoping</h2>
      <p>
        Each token is scoped to a single workspace. Requests made with the token are authenticated as
        the token owner within that workspace. Tokens cannot access other workspaces.
      </p>

      <h2>API Reference</h2>

      <h3>List tokens</h3>
      <ApiPlayground
        method="GET"
        path="/api-tokens"
        description="Returns all active tokens for the authenticated user in the current workspace."
      />

      <h3>Create a token</h3>
      <ApiPlayground
        method="POST"
        path="/api-tokens"
        description="Creates a new API token. The raw token value is returned once and cannot be retrieved again."
        defaultBody={{ name: 'My integration', scopes: [] }}
      />

      <h3>Revoke a token</h3>
      <ApiPlayground
        method="DELETE"
        path="/api-tokens/{tokenId}"
        description="Permanently revokes a token. Existing in-flight requests are not affected."
        pathParams={[{ name: 'tokenId', label: 'Token ID' }]}
      />

      <Callout type="tip">
        Manage tokens in the UI at <a href="/settings/api-tokens">Settings → API Tokens</a>.
      </Callout>
    </DocPage>
  )
}
