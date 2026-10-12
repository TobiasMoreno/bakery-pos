import { useEffect, useState, type FormEvent } from 'react'
import { APIError, getCurrentUser, login, logout, type User } from './api'
import CatalogDashboard from './CatalogDashboard'
import POSDashboard from './POSDashboard'

type SessionState =
  | { status: 'loading' }
  | { status: 'anonymous' }
  | { status: 'authenticated'; user: User }

function App() {
  const [session, setSession] = useState<SessionState>({ status: 'loading' })
  const [connectionError, setConnectionError] = useState(false)
  const [workspace, setWorkspace] = useState<'pos' | 'catalog'>('pos')

  useEffect(() => {
    let active = true
    getCurrentUser()
      .then((user) => {
        if (!active) return
        setSession(user ? { status: 'authenticated', user } : { status: 'anonymous' })
      })
      .catch(() => {
        if (!active) return
        setConnectionError(true)
        setSession({ status: 'anonymous' })
      })
    return () => {
      active = false
    }
  }, [])

  if (session.status === 'loading') return <LoadingScreen />
  if (session.status === 'anonymous') {
    return (
      <LoginScreen
        connectionError={connectionError}
        onAuthenticated={(user) => {
          setConnectionError(false)
          setSession({ status: 'authenticated', user })
        }}
      />
    )
  }
  const handleLogout = async () => {
    await logout()
    setSession({ status: 'anonymous' })
    setWorkspace('pos')
  }
  if (workspace === 'catalog') return (
    <CatalogDashboard
      user={session.user}
      onLogout={handleLogout}
      onOpenPOS={() => setWorkspace('pos')}
    />
  )
  return <POSDashboard user={session.user} onLogout={handleLogout} onOpenCatalog={() => setWorkspace('catalog')} />
}

function LoadingScreen() {
  return (
    <main className="loading-screen" aria-live="polite">
      <div className="brand-mark" aria-hidden="true">
        BP
      </div>
      <p>Preparando el mostrador…</p>
    </main>
  )
}

interface LoginScreenProps {
  connectionError: boolean
  onAuthenticated: (user: User) => void
}

function LoginScreen({ connectionError, onAuthenticated }: LoginScreenProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    void login(username, password)
      .then(onAuthenticated)
      .catch((caught: unknown) => {
        setError(caught instanceof APIError ? caught.message : 'No pudimos conectar con la caja.')
      })
      .finally(() => setSubmitting(false))
  }

  return (
    <main className="auth-layout">
      <section className="auth-story" aria-labelledby="welcome-title">
        <div className="brand-line">
          <span className="brand-mark">BP</span>
          <span>Bakery POS</span>
        </div>
        <div className="story-copy">
          <p className="eyebrow">Ventas y caja · fase 3</p>
          <h1 id="welcome-title">El mostrador, listo para vender.</h1>
          <p>
            Una caja táctil, auditable y pensada para el ritmo real del mostrador.
            Cada venta une precio, pago, efectivo y stock en una sola operación.
          </p>
        </div>
        <div className="story-note">
          <span aria-hidden="true">01</span>
          <p>Turnos reproducibles, pagos combinados y comprobantes internos sin efectos duplicados.</p>
        </div>
      </section>

      <section className="auth-panel" aria-labelledby="login-title">
        <form className="login-card" onSubmit={handleSubmit}>
          <header>
            <p className="eyebrow">Acceso al sistema</p>
            <h2 id="login-title">Iniciar sesión</h2>
            <p>Usá las credenciales configuradas para este entorno.</p>
          </header>

          {connectionError && (
            <div className="notice notice-warning" role="status">
              La API no respondió. Podés reintentar cuando el entorno esté listo.
            </div>
          )}
          {error && (
            <div className="notice notice-error" role="alert">
              {error}
            </div>
          )}

          <label>
            <span>Usuario</span>
            <input
              name="username"
              autoComplete="username"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              maxLength={80}
              required
            />
          </label>
          <label>
            <span>Contraseña o PIN</span>
            <input
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              minLength={8}
              maxLength={256}
              required
            />
          </label>
          <button className="primary-button" type="submit" disabled={submitting}>
            {submitting ? 'Verificando…' : 'Entrar al mostrador'}
          </button>
          <p className="form-footnote">El acceso queda registrado para auditoría.</p>
        </form>
      </section>
    </main>
  )
}

export default App
