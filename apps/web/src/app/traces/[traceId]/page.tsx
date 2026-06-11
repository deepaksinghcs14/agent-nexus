import { redirect } from 'next/navigation'

export default function TraceDetailRedirect({ params }: { params: { traceId: string } }) {
  redirect(`/runs/${params.traceId}`)
}
