import { useCallback, useEffect, useReducer } from 'react'
import { initialState, reducer } from '@/lib/store'
import type { SendRequest, ServerEvent } from '@/lib/types'
import { parseHash, viewHash, type View } from '@/lib/view'

// useHikari signs in with the token from the URL, follows the server's event
// stream, and keeps the open view in the URL.
export function useHikari() {
  const [state, dispatch] = useReducer(reducer, parseHash(location.hash), initialState)

  useEffect(() => {
    let stream: EventSource | undefined
    let retry: number | undefined
    let stopped = false

    const signedIn = async () => {
      const res = await fetch('/api/ui/session').catch(() => undefined)
      return res === undefined ? undefined : res.ok
    }

    const open = () => {
      stream = new EventSource('/api/ui/events')
      stream.onopen = () => dispatch({ type: 'connection', connection: 'live' })
      stream.onmessage = (e) => dispatch({ type: 'event', event: JSON.parse(e.data) as ServerEvent })
      stream.onerror = () => {
        dispatch({ type: 'connection', connection: 'reconnecting' })
        // The browser retries by itself unless the server refused the stream,
        // for example after a restart with a new token.
        if (stream?.readyState === EventSource.CLOSED) recover()
      }
    }

    const recover = async () => {
      const ok = await signedIn()
      if (stopped) return
      if (ok === false) dispatch({ type: 'connection', connection: 'signed-out' })
      else if (ok) open()
      else retry = window.setTimeout(recover, 2000)
    }

    const start = async () => {
      const token = new URLSearchParams(location.search).get('token')
      if (token) {
        await fetch('/api/ui/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token }),
        }).catch(() => undefined)
        history.replaceState(null, '', location.pathname + location.hash)
      }
      if (!stopped) recover()
    }

    start()
    return () => {
      stopped = true
      stream?.close()
      window.clearTimeout(retry)
    }
  }, [])

  useEffect(() => {
    const hash = viewHash(state.view)
    if (location.hash !== hash) history.replaceState(null, '', location.pathname + location.search + hash)
  }, [state.view])

  useEffect(() => {
    const onHash = () => dispatch({ type: 'view', view: parseHash(location.hash) })
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const show = useCallback((view: View) => dispatch({ type: 'view', view }), [])

  // send returns an error message, or null when the server accepted the message.
  const send = useCallback(async (req: SendRequest): Promise<string | null> => {
    const res = await fetch('/api/ui/send', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }).catch(() => undefined)
    if (!res) return 'The hikari server is not reachable.'
    if (res.ok) return null
    const body = (await res.json().catch(() => ({}))) as { error?: string }
    return body.error ?? res.statusText
  }, [])

  return { state, show, send }
}
