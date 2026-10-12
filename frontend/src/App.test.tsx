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

  it('logs in as owner and shows catalog management', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{}', { status: 401 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify(owner), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    render(<App />)

    await user.type(await screen.findByLabelText('Usuario'), 'owner')
    await user.type(screen.getByLabelText('Contraseña o PIN'), 'correct-horse')
    await user.click(screen.getByRole('button', { name: 'Entrar al mostrador' }))

    expect(await screen.findByRole('heading', { name: 'Buen día, Dueño Demo.' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '+ Nuevo producto' })).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(4))
  })

  it('keeps catalog read-only for an employee', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(employee), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    render(<App />)

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
