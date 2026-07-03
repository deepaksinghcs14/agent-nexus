import AgentBuilderPage from '../../new/page'

export default async function AgentEditPage({ params }: { params: Promise<{ agentId: string }> }) {
  const { agentId } = await params
  return <AgentBuilderPage agentId={agentId} />
}
