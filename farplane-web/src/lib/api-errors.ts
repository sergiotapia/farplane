/** Extract a human-readable message from an API error body. */
export function errorMessage(body: unknown, fallback: string): string {
  if (body && typeof body === 'object' && 'error' in body) {
    const value = (body as { error: unknown }).error
    if (typeof value === 'string' && value.length > 0) return value
  }
  if (body && typeof body === 'object' && 'message' in body) {
    const value = (body as { message: unknown }).message
    if (typeof value === 'string' && value.length > 0) return value
  }
  return fallback
}
