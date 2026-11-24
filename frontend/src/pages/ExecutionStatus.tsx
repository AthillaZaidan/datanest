import { useParams } from 'react-router'
import { getExecution } from '../api/executions'
import ExecutionMonitor from '../components/executions/ExecutionMonitor'
import { PageHeader } from '../components/kit'
import { useCrumbs } from '../context/crumbs'
import { dummyFetcher } from '../dev/dummyExecutions'

export default function ExecutionStatus() {
  const { executionId = '' } = useParams()
  const fetcher = import.meta.env.DEV && executionId.startsWith('dummy-') ? dummyFetcher : getExecution
  useCrumbs([{ label: `Eksekusi ${executionId}`, mono: true }])

  return (
    <>
      <PageHeader title="Status eksekusi" subtitle="Pemantauan job migration." />
      <ExecutionMonitor executionId={executionId} fetcher={fetcher} />
    </>
  )
}
