import { parse, stringify } from 'lossless-json'
import { ref } from 'vue'

export type Integer = number | bigint
export type Role = 'operator' | 'merchant' | 'buyer' | 'leader'
export interface Session { role: Role; id: Integer }
export type Row = { id: Integer } & Record<string, unknown>
export type Method = 'GET' | 'POST' | 'PUT'
type Envelope<T> = { code: number; message: string; data: T; traceId?: string }

let token = ''
const pendingKeys = new Map<string, string>()
export const alert = ref('')
export const expired = ref(false)

const messages: Record<number, string> = {
  400: '请求数据无效，请确认填写内容。',
  401: '登录凭据无效，请重新登录。',
  403: '当前身份没有权限执行此操作，或提货码校验失败。',
  404: '该记录暂时不可用，请刷新后确认。',
  409: '记录状态已变化、库存不足或操作已处理，请刷新后确认。',
  503: '服务暂时无法处理，请稍后重试。',
}

export async function request<T>(path: string, method: Method = 'GET', body?: unknown): Promise<T> {
  let response: Response
  try {
    response = await fetch('/v1' + path, {
      method,
      headers: {
        Authorization: 'Bearer ' + token,
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      },
      body: body === undefined ? undefined : stringify(body),
    })
  } catch {
    throw new Error('网络连接失败，请重试。')
  }
  const raw = await response.text()
  let result: Envelope<T> | undefined
  try {
    result = parse(raw, null, {
      parseNumber: (value) => {
        const number = Number(value)
        return Number.isSafeInteger(number) || !/^-?\d+$/.test(value) ? number : BigInt(value)
      },
    }) as Envelope<T>
  } catch {
    // A proxy can return a non-JSON error page.
  }
  if (!response.ok || result?.code !== 0) {
    if (response.status === 401) expired.value = true
    const message = messages[response.status] ?? '服务暂时无法处理请求。'
    throw new Error(result?.traceId ? message + ' 请求编号 ' + result.traceId : message)
  }
  return result.data
}

export async function list<T extends { id: Integer } = Row>(path: string): Promise<T[]> {
  const result: T[] = []
  let cursor: Integer = 0
  for (;;) {
    const page: T[] = await request<T[]>(path + '?after=' + cursor)
    result.push(...page)
    if (page.length < 100) return result
    cursor = page[page.length - 1].id
  }
}

export async function signIn(value: string) {
  token = value
  try {
    return await request<Session>('/session')
  } catch (error) {
    token = ''
    throw error
  }
}

export function signOut() {
  token = ''
  pendingKeys.clear()
}

export function retryKey(operation: string, fingerprint: string) {
  const lookup = operation + ':' + fingerprint
  const key = pendingKeys.get(lookup) ?? uuid()
  pendingKeys.set(lookup, key)
  return { key, complete: () => pendingKeys.delete(lookup) }
}

export function useTask() {
  const busy = ref(false)
  async function run(action: () => Promise<void>) {
    if (busy.value) return
    busy.value = true
    try {
      await action()
    } catch (error) {
      alert.value = error instanceof Error ? error.message : '操作失败，请重试。'
    } finally {
      busy.value = false
    }
  }
  return { busy, run }
}

export function id(value: unknown) {
  return String(value ?? '')
}

export function integer(value: string) {
  return BigInt(value)
}

export function uuid() {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 15) | 64
  bytes[8] = (bytes[8] & 63) | 128
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join('-')
}

export function money(value: unknown) {
  if (typeof value !== 'number' && typeof value !== 'bigint') return '—'
  const cents = BigInt(value)
  const absolute = cents < 0n ? -cents : cents
  return (cents < 0n ? '−' : '') + '¥' + (absolute / 100n).toLocaleString('zh-CN') + '.' + String(absolute % 100n).padStart(2, '0')
}

export function date(value: unknown) {
  return typeof value === 'string' && value
    ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
    : '—'
}

export function title(row: Row, key = 'name') {
  return String(row[key] ?? ('#' + row.id))
}

export function choices(rows: Row[], key = 'name') {
  return rows.map(row => ({ value: id(row.id), label: title(row, key) }))
}
