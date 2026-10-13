import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@fontsource-variable/commissioner'
import '@fontsource-variable/fraunces'
import './styles.css'
import App from './App'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/service-worker.js')
  })
}
