import { useEffect, useState } from 'react'

type Result<T> = { data?: T; error?: string; loading: boolean }

// useFetch loads a UI API resource, and loads it again when url changes. A
// null url loads nothing.
export function useFetch<T>(url: string | null, parse: (r: Response) => Promise<T>): Result<T> {
  const [result, setResult] = useState<{ url: string | null; data?: T; error?: string }>({ url: null })
  useEffect(() => {
    if (!url) return
    let stale = false
    fetch(url)
      .then(async (res) => {
        if (!res.ok) throw new Error(((await res.json().catch(() => ({}))) as { error?: string }).error ?? res.statusText)
        return parse(res)
      })
      .then((data) => !stale && setResult({ url, data }))
      .catch((e: Error) => !stale && setResult({ url, error: e.message }))
    return () => {
      stale = true
    }
  }, [url, parse])
  const loading = url !== null && result.url !== url
  return loading ? { loading } : { data: result.data, error: result.error, loading }
}

export const asJSON = <T,>(r: Response) => r.json() as Promise<T>
export const asText = (r: Response) => r.text()

export function storeFileURL(path: string): string {
  return `/api/ui/store/file?path=${encodeURIComponent(path)}`
}
