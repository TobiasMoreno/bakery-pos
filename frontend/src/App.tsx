import { useEffect, useState, type FormEvent } from 'react'
import { APIError, getCurrentUser, login, logout, type User } from './api'
import CatalogDashboard from './CatalogDashboard'
import POSDashboard from './POSDashboard'
import {
  clearOfflineUnlock,
  configureOfflineUnlock,
  getOfflineUnlockUser,
  initializeOfflineStorage,
  verifyOfflineUnlock,
} from './offline'

type SessionState =
  | { status: 'loading' }
  | { status: 'anonymous' }
  | { status: 'offline-locked'; user: User }
  | { status: 'authenticated'; user: User }

function App() {
  const [session, setSession] = useState<SessionState>({ status: 'loading' })
  const [connectionError, setConnectionError] = useState(false)
  const [workspace, setWorkspace] = useState<'pos' | 'catalog'>('pos')

  useEffect(() => {
    let active = true
    initializeOfflineStorage().then(getCurrentUser)
      .then((user) => {
        if (!active) return
        setSession(user ? { status: 'authenticated', user } : { status: 'anonymous' })
      })
      .catch(async () => {
        if (!active) return
        setConnectionError(true)
        const user = await getOfflineUnlockUser().catch(() => null)
        if (active) setSession(user ? { status: 'offline-locked', user } : { status: 'anonymous' })
      })
    return () => {
      active = false
    }
  }, [])

  if (session.status === 'loading') return <LoadingScreen />
  if (session.status === 'offline-locked') {
    return <OfflineUnlockScreen expectedUser={session.user} onUnlocked={(user) => setSession({ status: 'authenticated', user })} />
  }
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
    try {
      if (navigator.onLine) await logout()
    } finally {
      await clearOfflineUnlock()
      setSession({ status: 'anonymous' })
      setWorkspace('pos')
    }
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
      .then(async (user) => {
        if (typeof indexedDB !== 'undefined') await configureOfflineUnlock(user, password)
        onAuthenticated(user)
      })
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
          <p className="eyebrow">Ventas y caja resiliente · fase 4</p>
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

function OfflineUnlockScreen({ expectedUser, onUnlocked }: { expectedUser: User; onUnlocked: (user: User) => void }) {
  const [secret, setSecret] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true); setError(null)
    void verifyOfflineUnlock(secret).then((user) => {
      if (!user || user.id !== expectedUser.id) throw new Error('La clave local no coincide.')
      onUnlocked(user)
    }).catch((caught: unknown) => setError(caught instanceof Error ? caught.message : 'No se pudo desbloquear el puesto.'))
      .finally(() => setSubmitting(false))
  }

  return (
    <main className="auth-layout offline-unlock-layout">
      <section className="auth-story" aria-labelledby="offline-title">
        <div className="brand-line"><span className="brand-mark">BP</span><span>Bakery POS</span></div>
        <div className="story-copy"><p className="eyebrow">Acceso local protegido</p><h1 id="offline-title">El puesto sigue operativo.</h1><p>La API no responde. Desbloqueá el diario cifrado con la misma clave usada en el último acceso online.</p></div>
      </section>
      <section className="auth-panel">
        <form className="login-card" onSubmit={submit}>
          <header><p className="eyebrow">Modo sin conexión</p><h2>{expectedUser.name}</h2><p>La autorización offline tiene vigencia limitada y queda ligada a este dispositivo.</p></header>
          {error && <div className="notice notice-error" role="alert">{error}</div>}
          <label><span>Contraseña o PIN</span><input type="password" autoComplete="current-password" value={secret} onChange={(event) => setSecret(event.target.value)} minLength={8} required autoFocus /></label>
          <button className="primary-button" disabled={submitting}>{submitting ? 'Verificando localmente…' : 'Desbloquear mostrador'}</button>
        </form>
      </section>
    </main>
  )
}

export default App
