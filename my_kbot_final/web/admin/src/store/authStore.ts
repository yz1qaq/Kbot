import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'

export interface User {
  id: string
  email: string
  name: string
  role: string
  status: string
  created_at?: string
}

interface AuthState {
  token: string | null
  user: User | null
  expiresAt: string | null
  setAuth: (token: string, user: User, expiresAt: string) => void
  logout: () => void
}

export function isSessionValid(state: Pick<AuthState, 'token' | 'expiresAt'>) {
  return Boolean(state.token && state.expiresAt && Date.parse(state.expiresAt) > Date.now())
}

// 沿用 Final 的 Zustand 持久化；使用独立键，避免与课程版登录态混用。
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      expiresAt: null,
      setAuth: (token, user, expiresAt) => set({ token, user, expiresAt }),
      logout: () => set({ token: null, user: null, expiresAt: null }),
    }),
    { name: 'my-kbot-final-auth', storage: createJSONStorage(() => localStorage) },
  ),
)
