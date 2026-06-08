import { DocPage, Callout, Badge } from '@/components/docs/DocPage'

export const metadata = { title: 'What is a Connector — Docs' }

export default function WhatIsAConnectorDoc() {
  return (
    <DocPage
      title="What is a Connector?"
      subtitle="Connectors bring external knowledge into Agent Nexus — documents from your filesystem, Slack, Jira, GitHub, and more — so agents can retrieve relevant context automatically."
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
      <table>
        <thead><tr><th>Connector</th><th>Availability</th><th>What it indexes</th></tr></thead>
        <tbody>
          <tr>
            <td><strong>Filesystem</strong></td>
            <td><Badge label="v0.1" color="green" /></td>
            <td>Text files from a directory on the server&apos;s storage path</td>
          </tr>
          <tr>
            <td><strong>Slack</strong></td>
            <td><Badge label="v0.2" color="gray" /></td>
            <td>Channel messages and threads from a Slack workspace</td>
          </tr>
          <tr>
            <td><strong>Jira</strong></td>
            <td><Badge label="v0.2" color="gray" /></td>
            <td>Issues, comments, and descriptions from a Jira project</td>
          </tr>
          <tr>
            <td><strong>Confluence</strong></td>
            <td><Badge label="v0.2" color="gray" /></td>
            <td>Pages and comments from a Confluence space</td>
          </tr>
          <tr>
            <td><strong>GitHub</strong></td>
            <td><Badge label="v0.2" color="gray" /></td>
            <td>Repository files, issues, and pull request descriptions</td>
          </tr>
          <tr>
            <td><strong>Google Drive</strong></td>
            <td><Badge label="v0.2" color="gray" /></td>
            <td>Docs, Sheets, and Slides from a Drive folder</td>
          </tr>
        </tbody>
      </table>

      <h2>How Context Retrieval Works</h2>
      <p>
        When an agent has context retrieval enabled and runs a query:
      </p>
      <ol>
        <li>The user&apos;s message is <strong>embedded</strong> into a vector using the agent&apos;s provider.</li>
        <li>The runtime queries <code>connector_chunks</code> using <strong>pgvector similarity search</strong> — finding the chunks whose embeddings are closest to the query embedding.</li>
        <li>The top-N chunks (configured per agent) are included in the <strong>system prompt</strong> as context, along with the source document title, URL, and author.</li>
        <li>A <code>context_retrieval</code> RunStep is logged showing which chunks were used.</li>
      </ol>

      <pre><code>{`-- The actual SQL run during context retrieval:
SELECT cc.content, cc.metadata, cd.title, cd.url, cd.source
FROM connector_chunks cc
JOIN connector_documents cd ON cd.id = cc.document_id
WHERE cd.connector_id = ANY($allowed_connectors)
  AND cd.workspace_id = $workspace_id
ORDER BY cc.embedding <=> $query_embedding
LIMIT $max_chunks`}</code></pre>

      <h2>Setting Up the Filesystem Connector</h2>
      <p>
        The filesystem connector is the simplest way to get started. It indexes files from a directory
        on the Agent Nexus server.
      </p>
      <ol>
        <li>Go to <a href="/connectors">Connectors</a> and click <strong>Add Connector</strong>.</li>
        <li>Select <strong>Filesystem</strong>.</li>
        <li>Enter a directory path (must be within the server&apos;s <code>STORAGE_PATH</code>).</li>
        <li>Click <strong>Sync</strong>. The connector fetches files, chunks them into 512-token pieces with 64-token overlap, embeds each chunk, and stores them in the index.</li>
        <li>In the Agent Builder → <strong>Context tab</strong>, enable context retrieval and select this connector.</li>
      </ol>

      <Callout type="tip">
        Re-sync a connector any time your source files change. Agent Nexus uses content hashes to skip
        unchanged documents — only modified files are re-embedded.
      </Callout>

      <h2>Connector Sync Pipeline</h2>
      <p>
        Every connector follows the same pipeline internally:
      </p>
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
