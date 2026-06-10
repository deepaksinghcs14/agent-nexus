import AgentBuilderPage from '../../new/page'

export default function AgentEditPage({ params }: { params: { agentId: string } }) {
  return <AgentBuilderPage params={{ id: params.agentId }} />
}
