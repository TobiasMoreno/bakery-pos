import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

const owner = {
  id: '2cd7d77f-9f56-4c84-879d-b29879d599aa',
  name: 'Dueño Demo',
  username: 'owner',
  role: 'owner',
  permissions: ['sales:write', 'catalog:read', 'pricing:write', 'users:manage'],
}

const employee = {
  ...owner,
  id: 'aec4c047-a075-4e4a-97db-4b3ab9008701',
  name: 'Empleado Demo',
  username: 'employee',
  role: 'seller',
  permissions: ['sales:write', 'catalog:read'],
}

const device = {
  id: '4ce927a4-a259-4c5a-94a4-5ee579e481a1',
  device_id: '5706dd37-542f-4a41-8b5f-58935030393c',
  device_name: 'Mostrador test',
  user_id: owner.id,
  status: 'active',
  fencing_version: 1,
  acquired_at: '2026-10-12T12:00:00Z',
}

function posResponses() {
  return [
    new Response(JSON.stringify(device), { status: 200 }),
    new Response(JSON.stringify({ version: 1, generated_at: '2026-10-12T12:00:00Z', products: [] }), { status: 200 }),
    new Response(JSON.stringify({ shift: null }), { status: 200 }),
    new Response(JSON.stringify({ items: [], total: 0, limit: 30, offset: 0 }), { status: 200 }),
  ]
}

describe('App', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('shows login when there is no active session', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 401 })))
    render(<App />)
    expect(await screen.findByRole('heading', { name: 'Iniciar sesión' })).toBeInTheDocument()
  })

  it('logs in as owner, shows the POS and allows opening catalog management', async () => {
    const [acquire, snapshot, shift, sales] = posResponses()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{}', { status: 401 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify(owner), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(acquire)
      .mockResolvedValueOnce(snapshot)
      .mockResolvedValueOnce(shift)
      .mockResolvedValueOnce(sales)
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    render(<App />)

    await user.type(await screen.findByLabelText('Usuario'), 'owner')
    await user.type(screen.getByLabelText('Contraseña o PIN'), 'correct-horse')
    await user.click(screen.getByRole('button', { name: 'Entrar al mostrador' }))

    expect(await screen.findByRole('heading', { name: '¿Qué sale hoy?' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Catálogo' }))
    expect(await screen.findByRole('heading', { name: 'Buen día, Dueño Demo.' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '+ Nuevo producto' })).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(8))
  })

  it('keeps catalog read-only for an employee', async () => {
    const [acquire, snapshot, shift, sales] = posResponses()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(employee), { status: 200 }))
      .mockResolvedValueOnce(acquire)
      .mockResolvedValueOnce(snapshot)
      .mockResolvedValueOnce(shift)
      .mockResolvedValueOnce(sales)
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Catálogo' }))
    expect(await screen.findByText('Vista de empleado')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '+ Nuevo producto' })).not.toBeInTheDocument()
  })

  it('shows the API message for invalid credentials', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{}', { status: 401 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ message: 'Usuario o contraseña incorrectos.' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    render(<App />)

    await user.type(await screen.findByLabelText('Usuario'), 'owner')
    await user.type(screen.getByLabelText('Contraseña o PIN'), 'wrong-password')
    await user.click(screen.getByRole('button', { name: 'Entrar al mostrador' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Usuario o contraseña incorrectos.')
  })
})
