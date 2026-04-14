import { createBrowserRouter, RouterProvider, Navigate, Outlet } from 'react-router-dom'
import { getToken } from './services/api'
import Layout from './components/Layout'
import Login from './pages/Login'
import AdminDashboard from './pages/AdminDashboard'
import Questions from './pages/Questions'
import Knowledge from './pages/Knowledge'
import Upload from './pages/Upload'
import Drafts from './pages/Drafts'
import LoginWords from './pages/LoginWords'
import Metrics from './pages/Metrics'

function RequireAuth() {
  if (!getToken()) return <Navigate to="/login" replace />
  return (
    <Layout>
      <Outlet />
    </Layout>
  )
}

const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  {
    element: <RequireAuth />,
    children: [
      { path: '/', element: <AdminDashboard /> },
      { path: '/questions', element: <Questions /> },
      { path: '/knowledge', element: <Knowledge /> },
      { path: '/upload', element: <Upload /> },
      { path: '/drafts', element: <Drafts /> },
      { path: '/login-words', element: <LoginWords /> },
      { path: '/metrics', element: <Metrics /> },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])

export default function App() {
  return <RouterProvider router={router} />
}
