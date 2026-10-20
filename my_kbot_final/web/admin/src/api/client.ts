import axios from 'axios'

// 沿用 Final 的统一 Axios 入口。业务接口后续使用 /api/v1。
export const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})
