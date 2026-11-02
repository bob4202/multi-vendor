// Shapes shared by every API the portals call.

export interface Paginated<T> {
  data: T[]
  page: number
  page_size: number
  total_count: number
  total_pages: number
}

// Field-level validation problem from a 422 response.
export interface ApiIssue {
  field: string
  message: string
}

// What a server action hands back to the form that called it.
export type ActionResult<T = undefined> =
  | { ok: true; data: T }
  | { ok: false; error: string; status: number; issues: ApiIssue[] }
