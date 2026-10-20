import { Redirect, Route, Switch } from 'wouter'
import { AppLayout } from '@/components/AppLayout'
import { SystemStatusPage } from '@/pages/SystemStatusPage'

export default function App() {
  return <AppLayout>
    <Switch>
      <Route path="/" component={SystemStatusPage} />
      <Route><Redirect to="/" /></Route>
    </Switch>
  </AppLayout>
}
