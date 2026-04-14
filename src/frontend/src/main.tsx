import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createHashRouter, RouterProvider, Navigate } from 'react-router-dom'
import './index.css'
import App from './App.tsx'
import Auth from './pages/Auth.tsx'
import Dashboard from './pages/Dashboard.tsx'
import Chat from './pages/Chat.tsx'
import Results from './pages/Results.tsx'
import Privacy from './pages/legal/Privacy.tsx'
import Account from './pages/Account.tsx'
import Billing from './pages/Billing.tsx'
import Recover from './pages/Recover.tsx'

const router = createHashRouter([
  { path: '/', element: <App /> },
  { path: '/auth', element: <Auth /> },
  { path: '/auth/recover', element: <Recover /> },
  { path: '/dashboard', element: <Dashboard /> },
  { path: '/chat/:id', element: <Chat /> },
  { path: '/results/:id', element: <Results /> },
  { path: '/privacy-policy', element: <Privacy /> },
  { path: '/account', element: <Account /> },
  { path: '/billing', element: <Billing /> },
  { path: '*', element: <Navigate to="/" replace /> },
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
