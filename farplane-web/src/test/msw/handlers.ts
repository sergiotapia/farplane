import { HttpResponse, http } from 'msw'

import { API_BASE_URL } from '@/lib/api.ts'

export const handlers = [
  http.get(`${API_BASE_URL}/api/v1/setup/status`, () =>
    HttpResponse.json({
      needs_setup: false,
      google_oauth_configured: false,
      setup_token_required: false,
    }),
  ),
  http.get(`${API_BASE_URL}/api/v1/me`, () =>
    HttpResponse.json({ error: 'unauthorized' }, { status: 401 }),
  ),
]
