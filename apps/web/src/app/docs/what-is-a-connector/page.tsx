import { DocPage, Callout, Badge } from '@/components/docs/DocPage'

export const metadata = { title: 'What is a Connector — Docs' }

export default function WhatIsAConnectorDoc() {
  return (
    <DocPage
      title="What is a Connector?"
      subtitle="Connectors bring external knowledge into Agent Nexus — documents from your filesystem, GitHub repositories, Confluence spaces, and more — so agents can retrieve relevant context automatically."
    >
      <p>
        Agents are only as useful as the context they have access to. A <strong>connector</strong> is a
        data pipeline that pulls documents from an external source, breaks them into chunks, generates
        vector embeddings, and stores them in a searchable index. During a run, agents query this index
        to find the most relevant passages for the user&apos;s message.
      </p>
      <p>
        This is called <strong>Retrieval-Augmented Generation (RAG)</strong> — instead of packing all
        your documents into the system prompt, the agent retrieves only what&apos;s relevant for each query.
      </p>

      <h2>Available Connectors</h2>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Connector</th><th>Availability</th><th>What it indexes</th></tr></thead>
        <tbody>
          <tr>
            <td><strong>Filesystem</strong></td>
            <td><Badge label="Available" color="green" /></td>
            <td>Text files from a directory on the server&apos;s storage path</td>
          </tr>
          <tr>
            <td><strong>GitHub</strong></td>
            <td><Badge label="Available" color="green" /></td>
            <td>All text files across one repo or every repo accessible to a token</td>
          </tr>
          <tr>
            <td><strong>Confluence</strong></td>
            <td><Badge label="Available" color="green" /></td>
            <td>Pages from one or more Confluence Cloud spaces</td>
          </tr>
          <tr>
            <td><strong>Slack</strong></td>
            <td><Badge label="Coming soon" color="gray" /></td>
            <td>Channel messages and threads from a Slack workspace</td>
          </tr>
          <tr>
            <td><strong>Jira</strong></td>
            <td><Badge label="Coming soon" color="gray" /></td>
            <td>Issues, comments, and descriptions from a Jira project</td>
          </tr>
          <tr>
            <td><strong>Google Drive</strong></td>
            <td><Badge label="Coming soon" color="gray" /></td>
            <td>Docs, Sheets, and Slides from a Drive folder</td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>How Context Retrieval Works</h2>
      <p>
        There are two retrieval modes. Both use the same pgvector similarity search under the hood — they differ in <em>when</em> retrieval happens and <em>who decides</em>.
      </p>

      <h3>Standard RAG (default)</h3>
      <p>
        When <code>context_retrieval_enabled=true</code> and <code>agentic_rag=false</code>:
      </p>
      <ol>
        <li>The user&apos;s message is <strong>embedded</strong> into a vector using the agent&apos;s provider.</li>
        <li>The runtime queries <code>connector_chunks</code> using <strong>pgvector cosine similarity</strong> and filters by <code>min_score</code>.</li>
        <li>The top <code>max_chunks</code> results are injected into the <strong>system prompt</strong> before the first LLM turn.</li>
        <li>A <code>context_retrieval</code> RunStep is logged showing which chunks were used.</li>
      </ol>
      <pre><code>{`SELECT cc.content, cd.title, cd.url, cd.source,
       1 - (cc.embedding <=> $query_embedding) AS score
FROM connector_chunks cc
JOIN connector_documents cd ON cd.id = cc.document_id
WHERE cd.workspace_id = $workspace_id
  AND c.id = ANY($connector_ids)
  AND 1 - (cc.embedding <=> $query_embedding) >= $min_score
ORDER BY cc.embedding <=> $query_embedding
LIMIT $max_chunks`}</code></pre>

      <h3>Agentic RAG</h3>
      <p>
        When <code>agentic_rag=true</code>, pre-run retrieval is <strong>skipped entirely</strong>. Instead, the agent receives a <code>native_retrieve_context</code> tool it can call at any point during a run:
      </p>
      <pre><code>{`native_retrieve_context({
  query: "authentication flow for the payments service",
  max_chunks: 20,   // optional, default 8, max 50
  min_score: 0.4    // optional, default 0.5
})`}</code></pre>
      <p>
        This gives the agent full control: it can issue <strong>multiple targeted retrievals</strong> with different queries, request more chunks when it needs broader coverage, or skip retrieval entirely if the answer is already known. The trade-off is one extra LLM turn before the agent has context.
      </p>
      <p>
        <strong>When to use Agentic RAG:</strong> multi-step research tasks, large knowledge bases where a single query would miss relevant chunks, or agents that need to retrieve context mid-task rather than upfront.
      </p>

      <h2>Setting Up the Filesystem Connector</h2>
      <p>
        The filesystem connector is the simplest way to get started. It indexes files from a directory
        on the Agent Nexus server.
      </p>
      <ol>
        <li>Go to <a href="/connectors">Connectors</a> and click <strong>Connect source</strong>.</li>
        <li>Select <strong>Filesystem</strong>.</li>
        <li>Enter a directory path (must be within the server&apos;s <code>STORAGE_PATH</code>).</li>
        <li>Click <strong>Sync</strong>. The connector fetches files, chunks them, embeds each chunk, and stores them in the index.</li>
        <li>In the Agent Builder → <strong>Context tab</strong>, enable context retrieval and select this connector.</li>
      </ol>

      <h2>Setting Up the GitHub Connector</h2>
      <p>
        The GitHub connector indexes all text files from one repository or every repository accessible
        to a personal access token — with no additional configuration required.
      </p>
      <ol>
        <li>Go to <a href="/connectors">Connectors</a> and click <strong>Connect source</strong>.</li>
        <li>Select <strong>GitHub</strong>.</li>
        <li>Enter a <strong>Personal access token</strong> (PAT) with <code>repo</code> read scope.</li>
        <li>
          <strong>Owner</strong> and <strong>Repository</strong> are both optional:
          <ul>
            <li>Leave both blank — every repo accessible to the token is auto-discovered and indexed.</li>
            <li>Fill in Owner only — indexes all repos under that org or user.</li>
            <li>Fill in both — indexes a single specific repository.</li>
          </ul>
        </li>
        <li>Optionally specify a branch; defaults to the repo&apos;s default branch.</li>
        <li>Click <strong>Connect</strong>, then <strong>Sync</strong>.</li>
      </ol>

      <Callout type="tip">
        Binary files (images, archives, compiled assets) and files larger than 500 KB are skipped
        automatically. Only text content is indexed.
      </Callout>

      <p>
        After syncing, the <strong>Documents tab</strong> shows a repository browser — click into a
        repo to navigate its folder tree and see which files are indexed, exactly like a file explorer.
        Use the search box to find any file by name across the whole tree.
      </p>

      <h2>Setting Up the Confluence Connector</h2>
      <p>
        The Confluence connector indexes pages from your Confluence Cloud workspace.
      </p>
      <ol>
        <li>Go to <a href="/connectors">Connectors</a> and click <strong>Connect source</strong>.</li>
        <li>Select <strong>Confluence</strong>.</li>
        <li>Enter your Confluence <strong>URL</strong> (e.g. <code>https://yourorg.atlassian.net</code>).</li>
        <li>Enter your Atlassian account <strong>email</strong> and an <strong>API token</strong> (create one at <code>id.atlassian.com → Security → API tokens</code>).</li>
        <li>
          <strong>Space keys</strong> is optional — leave blank to index all spaces, or enter a
          comma-separated list (e.g. <code>ENG,OPS,HR</code>) to limit the scope.
        </li>
        <li>Click <strong>Connect</strong>, then <strong>Sync</strong>.</li>
      </ol>

      <p>
        After syncing, the <strong>Documents tab</strong> shows a space browser — click into a space to
        see all indexed pages. Use the search box to filter pages by title within any space.
      </p>

      <h2>Browsing Indexed Documents</h2>
      <p>
        The <strong>Documents tab</strong> on any connector card provides a filesystem-style browser for
        GitHub and Confluence connectors:
      </p>
      <ul>
        <li><strong>GitHub</strong> — root shows all indexed repositories with file counts. Click a repo to navigate its folder tree. Click a folder to go deeper. Files show an external link icon on hover to open them on GitHub.</li>
        <li><strong>Confluence</strong> — root shows all indexed spaces with page counts. Click a space to list its pages.</li>
        <li><strong>Search</strong> — type in the search box at any level to filter by filename or title. Results are shown as a flat list; navigating to a folder clears the search automatically.</li>
        <li><strong>Breadcrumb</strong> — click any segment in the breadcrumb to jump back up the tree.</li>
      </ul>

      <h2>Connector Sync Pipeline</h2>
      <p>
        Every connector follows the same pipeline internally:
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Step</th><th>What happens</th></tr></thead>
        <tbody>
          <tr><td>1. Fetch</td><td>Documents are retrieved from the source (files, API, etc.)</td></tr>
          <tr><td>2. Hash check</td><td>Documents unchanged since the last sync are skipped</td></tr>
          <tr><td>3. Chunk</td><td>Content is split into 512-token chunks with 64-token overlap</td></tr>
          <tr><td>4. Embed</td><td>Each chunk is embedded using the workspace provider</td></tr>
          <tr><td>5. Upsert</td><td>Embeddings are stored in <code>connector_chunks</code> with pgvector</td></tr>
          <tr><td>6. Log</td><td>A sync job record is written with counts, duration, and status</td></tr>
        </tbody>
      </table>
      </div>

      <Callout type="tip">
        Re-sync a connector any time your source changes. Agent Nexus uses content hashes to skip
        unchanged documents — only new or modified content is re-embedded. GitHub and Confluence syncs
        also checkpoint their progress, so a pod restart resumes from where it left off.
      </Callout>

      <h2>Configuring Context Retrieval on an Agent</h2>
      <p>
        In the Agent Builder, the <strong>Context tab</strong> lets you:
      </p>
      <ul>
        <li>Enable or disable context retrieval entirely.</li>
        <li>Select which connectors the agent is allowed to query (scoped by workspace).</li>
        <li>Set <code>max_chunks</code> — how many chunks to include per run (default: 5).</li>
        <li>Set <code>min_score</code> — minimum similarity threshold (0–1); chunks below this are ignored.</li>
      </ul>

      <Callout type="warning">
        Context chunks count toward the model&apos;s context window. If you set <code>max_chunks</code> too high
        with large documents, you may hit token limits. Start with 5 and increase if needed.
      </Callout>

      <h2>Next Steps</h2>
      <ul>
        <li><a href="/connectors">Add a connector to your workspace</a></li>
        <li><a href="/docs/what-is-an-agent">Enable context retrieval on an agent</a></li>
        <li><a href="/docs/run-states">See context retrieval steps in run traces</a></li>
      </ul>
    </DocPage>
  )
}
