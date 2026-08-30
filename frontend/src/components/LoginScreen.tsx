import { AuthScreen } from '@biotron/style'
import { useAuth } from '../auth/context'

export default function LoginScreen() {
  const { login, loginGuest } = useAuth()
  const denied = new URLSearchParams(window.location.search).get('auth') === 'denied'

  return (
    <AuthScreen
      productName="Exo Telemetry"
      action={{ label: 'Sign in with GitHub', onClick: login, icon: 'github' }}
      notices={denied ? [{ content: "Access denied — your GitHub account isn't a BioTronDesignTeam member.", tone: 'error' }] : []}
      guestAccess={{ onSubmit: loginGuest }}
    />
  )
}
