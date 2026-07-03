import { redirect } from 'next/navigation'

export default async function TraceDetailRedirect({ params }: { params: Promise<{ traceId: string }> }) {
  const { traceId } = await params
  redirect(`/runs/${traceId}`)
}
