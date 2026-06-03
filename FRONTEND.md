# mynance — Frontend Spec

Backend reference: [README.md](./README.md). This doc covers everything an agent or human needs to design and build the Next.js frontend.

---

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Framework | **Next.js 15** (App Router) | Server Components, RSC streaming, modern React 19 |
| Auth | **Auth.js v5** (`next-auth@beta`) | Credentials provider wraps our `POST /login`; session JWT cookie holds backend token |
| Styling | **Tailwind CSS + shadcn/ui** | Fast scaffold, exchange-grade dark mode |
| Forms | **react-hook-form + zod** | Mirrors backend `validator/v10` constraints |
| Data | **TanStack Query v5** | Polling, stale-while-revalidate, retry, optimistic updates |
| Charts | **lightweight-charts** (TradingView OSS) | Candle/line; lighter than full Recharts for finance |
| Tables | **TanStack Table v8** | Sortable, virtualized for long histories |
| Decimals | **decimal.js** | Backend uses `NUMERIC(30,10)`; **NEVER** `parseFloat` financial values |
| Validation | **zod** | Shared schemas with backend tag mirroring |
| UUID | `crypto.randomUUID()` | Native; used for client idempotency keys |
| Icons | **lucide-react** | shadcn/ui ships with it |
| Toast | **sonner** | shadcn/ui standard |
| State | **TanStack Query** + URL state (`nuqs`) | No Redux; server state in Query, UI state in URL |

---

## Personas

### Trader (primary, role=`USER`)
- Signs up + logs in
- Funds account (sees deposit address, watches for balance to update)
- Views markets (Binance feed = price reference)
- Places BUY / SELL orders on a symbol
- Watches local order book + own open orders
- Cancels open orders
- Reviews own trade history
- Withdraws funds

### Admin (role=`ADMIN`)
- All trader actions
- Confirms or rejects pending deposits (mocks chain webhook)
- Lists deposits across all users
- Future: promote users to admin, monitor system

Admin promotion is via raw SQL in MVP. Once promoted, user must re-login for `role=ADMIN` to appear in the JWT.

---

## Pages

### Public (no auth)
| Route | Purpose |
|---|---|
| `/login` | Email + password form. On success, Auth.js stores session; redirect to `/portfolio` |
| `/register` | Calls `POST /users`. On success, auto-login. |

### Trader (auth required)
| Route | Purpose | Primary data source |
|---|---|---|
| `/portfolio` | Balance cards per asset, links to deposit/withdraw, recent orders/trades | `GET /accounts`, `GET /accounts/{id}/balance` per row |
| `/markets` | Symbol list with Binance ticker (price, 24h change, volume) | `GET /markets` (poll 5s) |
| `/markets/[symbol]` | Trade view: book, recent trades, place order, my open orders | mixed (see below) |
| `/orders` | All my orders, filter by status | `GET /users/{userID}/orders` |
| `/trades` | My executed trades | `GET /users/{userID}/trades` |
| `/wallets` | List of deposit addresses across assets, create new | `GET /wallets`, `POST /wallets` |
| `/deposits` | My deposit history (PENDING / CONFIRMED / REJECTED) | `GET /deposits` (poll 10s while any PENDING) |
| `/account/[id]/withdraw` | Withdraw form for a specific account | `POST /accounts/{id}/withdraw` |
| `/me` | Profile details, sign-out | `GET /me` |

### Admin (role=ADMIN)
| Route | Purpose |
|---|---|
| `/admin` | Dashboard: pending deposits count, recent activity |
| `/admin/deposits` | List all pending deposits with confirm/reject buttons |
| `/admin/deposits/intake` | Form to simulate webhook (mock chain deposit) |

---

## Trade view (`/markets/[symbol]`) — the core page

```
┌─────────────────────────────────────────────────────────────────────┐
│  BTC-USDT          $30,012.34  +1.69%  Vol 1.2B (Binance ref price) │
├──────────────────────┬────────────────────┬─────────────────────────┤
│  Local Order Book    │  Place Order       │  Recent Trades (local)  │
│  Asks ──────────     │  ┌─ BUY ─┬─ SELL ┐ │  ──────────────         │
│   30050  0.30        │  Price: [_______]  │  30000  0.5  10:00:01   │
│   30025  0.50        │  Qty:   [_______]  │  30000  0.3  09:59:58   │
│  ──────              │  Total: 0 USDT     │  ...                    │
│   30000  0.50        │  Avail: 100000 USDT│                         │
│   29950  1.20        │  [ Place BUY @ ... ]                         │
│  Bids ──────────     │                    │  Binance Recent (ref)   │
│                      │                    │  30012  0.1  10:00:02   │
├──────────────────────┴────────────────────┴─────────────────────────┤
│  My Open Orders for BTC-USDT                                         │
│  ID    Side  Price    Qty   Filled  Status   Created   [Cancel]     │
│  ...   BUY   30000    0.5   0.0     OPEN     10:00     [✕]          │
└─────────────────────────────────────────────────────────────────────┘
```

Data dependencies:
- **Local book** (`GET /orderbook/{symbol}`) — poll 1s, this is where the user's order actually matches
- **Binance ticker** (`GET /markets/{symbol}/ticker`) — poll 5s, **reference price** shown at top
- **Binance recent trades** (`GET /markets/{symbol}/trades`) — poll 3s, secondary panel
- **Local recent trades** (`GET /marketdata/trades/{symbol}`) — poll 1s, primary panel
- **My open orders** (`GET /users/{userID}/orders` filtered client-side) — poll 2s, refresh on submit
- **Balance** (`GET /accounts/{id}/balance` for reserve asset) — poll 5s, refresh on submit

### UX gotcha: hybrid liquidity

Binance shows BTC @ 30012 but **local book is thin** (only other mynance users). Users placing market-ish orders may not fill. **Communicate this:**

```
ℹ️ Reference price: $30,012 (Binance). Local book depth: 2 levels.
   Your order will rest until matched by another user.
```

Show local book prominently; Binance is the smaller side-panel "for reference".

---

## Auth.js setup

### `auth.config.ts`

```ts
import NextAuth from 'next-auth'
import Credentials from 'next-auth/providers/credentials'
import { z } from 'zod'

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(1),
})

export const { handlers, auth, signIn, signOut } = NextAuth({
  providers: [
    Credentials({
      credentials: {
        email: {}, password: {},
      },
      authorize: async (raw) => {
        const parsed = loginSchema.safeParse(raw)
        if (!parsed.success) return null

        const res = await fetch(`${process.env.API_URL}/login`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(parsed.data),
        })
        if (!res.ok) return null
        const { token, user } = await res.json()
        return {
          id: user.id,
          email: user.email,
          name: user.username,
          role: user.role,
          backendToken: token,
        }
      },
    }),
  ],
  session: { strategy: 'jwt' },
  callbacks: {
    jwt: ({ token, user }) => {
      if (user) {
        token.backendToken = (user as any).backendToken
        token.role = (user as any).role
        token.userId = user.id
      }
      return token
    },
    session: ({ session, token }) => {
      session.backendToken = token.backendToken as string
      session.user.id = token.userId as string
      ;(session.user as any).role = token.role
      return session
    },
  },
  pages: { signIn: '/login' },
})

declare module 'next-auth' {
  interface Session {
    backendToken: string
    user: { id: string; email: string; name: string; role: 'USER' | 'ADMIN' }
  }
}
```

### `middleware.ts` (route protection)

```ts
import { auth } from '@/auth.config'
import { NextResponse } from 'next/server'

const PUBLIC = ['/login', '/register']

export default auth((req) => {
  const isPublic = PUBLIC.some(p => req.nextUrl.pathname.startsWith(p))
  if (!req.auth && !isPublic) {
    return NextResponse.redirect(new URL('/login', req.url))
  }
  if (req.auth && isPublic) {
    return NextResponse.redirect(new URL('/portfolio', req.url))
  }
})

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
}
```

### Admin gate

```tsx
// app/admin/layout.tsx
import { auth } from '@/auth.config'
import { redirect } from 'next/navigation'

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  const session = await auth()
  if (session?.user?.role !== 'ADMIN') redirect('/portfolio')
  return <>{children}</>
}
```

---

## API client pattern

One thin wrapper around `fetch`. Pulls backend token from Auth.js session.

### `lib/api/client.ts`

```ts
import { auth } from '@/auth.config'

export class APIError extends Error {
  constructor(public status: number, public body: any) {
    super(typeof body === 'string' ? body : body?.error ?? `HTTP ${status}`)
  }
}

const BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

// Server-side fetch (RSC, Server Actions) — pulls session via auth()
export async function apiServer(path: string, init: RequestInit = {}): Promise<any> {
  const session = await auth()
  return doFetch(path, init, session?.backendToken)
}

// Client-side fetch — token passed by caller (e.g., from useSession)
export async function apiClient(path: string, init: RequestInit = {}, token?: string): Promise<any> {
  return doFetch(path, init, token)
}

async function doFetch(path: string, init: RequestInit, token?: string) {
  const headers = new Headers(init.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')

  const res = await fetch(`${BASE}${path}`, { ...init, headers })
  const text = await res.text()
  const body = text ? safeJSON(text) : null
  if (!res.ok) throw new APIError(res.status, body ?? text)
  return body
}

function safeJSON(t: string) { try { return JSON.parse(t) } catch { return t } }
```

### Client-side hook

```ts
// lib/api/use-api.ts
import { useSession } from 'next-auth/react'
import { apiClient } from './client'

export function useApi() {
  const { data: session } = useSession()
  return {
    get: (p: string) => apiClient(p, { method: 'GET' }, session?.backendToken),
    post: (p: string, body: any) => apiClient(p, { method: 'POST', body: JSON.stringify(body) }, session?.backendToken),
    del: (p: string) => apiClient(p, { method: 'DELETE' }, session?.backendToken),
  }
}
```

### TanStack Query example

```ts
// hooks/use-balance.ts
import { useQuery } from '@tanstack/react-query'
import { useApi } from '@/lib/api/use-api'

export function useBalance(accountId: string) {
  const api = useApi()
  return useQuery({
    queryKey: ['balance', accountId],
    queryFn: () => api.get(`/accounts/${accountId}/balance`),
    refetchInterval: 5_000,
    staleTime: 1_000,
  })
}
```

---

## Polling cadence

| Resource | Interval | Where |
|---|---|---|
| Local order book | **1s** | trade view |
| Local recent trades | 1s | trade view |
| Open orders for symbol | 2s | trade view |
| Balance (current asset) | 5s | trade view, portfolio |
| Binance ticker (per symbol) | 5s | markets list, trade view header |
| Binance order book | 3s | trade view ref panel |
| Binance recent trades | 3s | trade view ref panel |
| Markets list (all tickers) | 5s | `/markets` page |
| Order history | 10s (manual refresh available) | `/orders` |
| Trade history | 10s | `/trades` |
| Deposits (any PENDING?) | **10s while pending, else 60s** | `/deposits`, `/portfolio` |
| Profile / `GET /me` | on demand | profile page |

**Pause on tab blur.** TanStack Query handles via `refetchOnWindowFocus: true` and `refetchIntervalInBackground: false` (default).

---

## Forms

Pattern: zod schema → `react-hook-form` → mutation → toast.

### Login

```tsx
'use client'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { signIn } from 'next-auth/react'

const schema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
})

export function LoginForm() {
  const form = useForm({ resolver: zodResolver(schema) })
  const onSubmit = async (data: z.infer<typeof schema>) => {
    const res = await signIn('credentials', { ...data, redirect: false })
    if (res?.error) toast.error('Invalid credentials')
    else router.push('/portfolio')
  }
  // ...
}
```

### Place order (idempotency key generated client-side)

```tsx
const placeOrderSchema = z.object({
  symbol: z.string().regex(/^[A-Z]+-[A-Z]+$/),
  side: z.enum(['BUY', 'SELL']),
  price: z.string().regex(/^\d+(\.\d+)?$/),    // strings — never floats
  quantity: z.string().regex(/^\d+(\.\d+)?$/),
})

export function PlaceOrderForm({ symbol, side }: Props) {
  const api = useApi()
  const qc = useQueryClient()
  const idempotencyKey = useRef(crypto.randomUUID())  // stable per mount; regen on success

  const mutation = useMutation({
    mutationFn: (data: z.infer<typeof placeOrderSchema>) =>
      api.post('/orders', { ...data, idempotency_key: idempotencyKey.current }),
    onSuccess: () => {
      toast.success('Order placed')
      idempotencyKey.current = crypto.randomUUID()
      qc.invalidateQueries({ queryKey: ['orders', symbol] })
      qc.invalidateQueries({ queryKey: ['balance'] })
    },
    onError: (err: APIError) => {
      if (err.status === 422) toast.error('Insufficient funds')
      else toast.error(err.message)
    },
  })
  // ...
}
```

---

## Decimal handling

**NEVER use `parseFloat`, `Number()`, or `+x` on price/quantity/balance.** Backend uses `NUMERIC(30,10)`; double-precision floats lose precision past ~15 significant digits.

```ts
import Decimal from 'decimal.js'

// Display only — formatted strings
export const fmt = (v: string, dp = 8) => new Decimal(v).toFixed(dp)

// Arithmetic
const total = new Decimal(price).mul(quantity).toString()  // "15000.0000000000"

// Comparison
new Decimal(amount).lte(balance)  // true / false

// Input → string for API
const cleanPrice = new Decimal(form.price).toString()  // strips trailing zeros normalization
```

Backend echoes back canonical `30000.0000000000` form. Don't expect string equality with what you sent; compare via Decimal.

---

## Idempotency keys

Generate per **logical user action**, not per network request. Reuse across retries.

```ts
const key = useRef(crypto.randomUUID())

// On submit
api.post('/orders', { ...payload, idempotency_key: key.current })

// On success — generate a fresh key for the next form submission
key.current = crypto.randomUUID()

// On error — KEEP the same key so retry-on-network-blip is safe
```

Scopes (backend):
- `POST /orders` → scope `ORDER`
- `POST /accounts/{id}/withdraw` → scope `WITHDRAW`
- `POST /trades` (admin) → scope `TRADE`

Duplicate within a scope → backend returns **200 OK with empty body**. Treat as success on the frontend (the resource already exists; refetch and continue).

---

## Error handling

| Status | Meaning | UX |
|---|---|---|
| `200` | OK (incl. idempotent duplicate) | success path |
| `201` | Created | success path |
| `204` | Deleted | success path |
| `400` | Bad request (validation) | inline form error from `err.body.error` |
| `401` | Unauthenticated / token expired | call `signIn()` / redirect to `/login`; clear React Query cache |
| `403` | Forbidden (resource owner mismatch, admin-only) | toast "Not allowed"; navigate to safe page |
| `404` | Not found | render "Not found" view |
| `409` | Conflict / invalid state transition | toast specific message (`Order already filled`, `Address exists`, etc.) |
| `422` | Insufficient funds / validation | inline form error or balance warning |
| `429` | Rate limit (future) | toast "Slow down"; backoff |
| `500` | Server error | toast "Something went wrong"; log to error tracker |
| `503` | Service unavailable (e.g., marketfeed) | render "Reference price unavailable" inline; degrade gracefully |

Global error boundary catches unhandled APIError to show a generic toast. Per-mutation `onError` handles the specific cases.

---

## Empty / loading / error states

Every async surface needs four states. Standard pattern:

```tsx
const { data, isLoading, isError, error } = useQuery(...)

if (isLoading) return <Skeleton />
if (isError) {
  if (error instanceof APIError && error.status === 503) {
    return <Banner>Reference data temporarily unavailable</Banner>
  }
  return <Banner>Something went wrong. <Button onClick={refetch}>Retry</Button></Banner>
}
if (!data || data.length === 0) return <EmptyState />
return <View data={data} />
```

Skeletons match final shape (rows/cards), not generic spinners.

---

## Component inventory

Reusable across pages:

| Component | Used on | Notes |
|---|---|---|
| `<OrderBook>` | trade view | bids desc / asks asc; click level to autofill price |
| `<RecentTrades>` | trade view | local + Binance variants |
| `<PlaceOrderForm>` | trade view | side toggle, balance display, total auto-calc |
| `<OrderRow>` | trade view, orders page | shows status pill, cancel button when open/partial |
| `<TradeRow>` | trades page, trade view | side color, counterparty, time |
| `<BalanceCard>` | portfolio, deposit, withdraw | asset icon, amount, USD value (future) |
| `<SymbolPicker>` | top nav | dropdown of `BINANCE_SYMBOLS` |
| `<DecimalInput>` | all forms | rejects `e`, `-` for positive amounts; trims spaces |
| `<StatusPill>` | many | color-coded (OPEN green, FILLED blue, CANCELLED gray, PENDING amber, etc.) |
| `<DepositAddressCard>` | wallets, deposit | shows mock address, QR (future), copy button, warning "MOCK — do not send real assets" |
| `<TickerHeader>` | trade view | symbol, last price, 24h change, volume |
| `<EmptyState>` | every list | icon + label + CTA |
| `<Skeleton>` (shadcn) | loading | per-shape variants |
| `<DataTable>` | history pages | TanStack Table powered |
| `<Toast>` | global | `sonner` |

---

## Hybrid market data UX

Two parallel data sources for prices:

| Source | Endpoint | When to display |
|---|---|---|
| **Binance (display)** | `/markets/{symbol}/orderbook`, `/markets/{symbol}/ticker`, `/markets/{symbol}/trades` | Header price, 24h stats, "reference market" panel, chart |
| **Local engine (truth for execution)** | `/orderbook/{symbol}`, `/marketdata/trades/{symbol}` | Primary order book (where your orders match), recent fills on mynance |

Visual hierarchy: local book is the centerpiece. Binance ticker sits in the header as price context. Binance order book / trades go in a smaller side panel labeled "Reference (Binance)".

Show a clear info banner when local book is empty:

> Local order book is thin — your order may rest until another user crosses. Reference price from Binance: $30,012.34.

---

## Data fetching cookbook

### Portfolio page (server-rendered shell + client polling)

```tsx
// app/portfolio/page.tsx
import { apiServer } from '@/lib/api/client'
import { PortfolioClient } from './portfolio-client'

export default async function Portfolio() {
  const accounts = await apiServer('/accounts')
  return <PortfolioClient initialAccounts={accounts} />
}

// portfolio-client.tsx
'use client'
export function PortfolioClient({ initialAccounts }: Props) {
  const { data: accounts } = useQuery({
    queryKey: ['accounts'],
    queryFn: () => api.get('/accounts'),
    initialData: initialAccounts,
    refetchInterval: 10_000,
  })
  // render <BalanceCard> per account
}
```

### Trade view (heavy polling)

```tsx
// app/markets/[symbol]/page.tsx
export default async function TradePage({ params }: { params: Promise<{ symbol: string }> }) {
  const { symbol } = await params
  return <TradeView symbol={symbol} />
}

// trade-view.tsx — all client side
'use client'
export function TradeView({ symbol }: { symbol: string }) {
  const book = useQuery({ queryKey: ['book', symbol], queryFn: () => api.get(`/orderbook/${symbol}`), refetchInterval: 1_000 })
  const trades = useQuery({ queryKey: ['localTrades', symbol], queryFn: () => api.get(`/marketdata/trades/${symbol}`), refetchInterval: 1_000 })
  const ticker = useQuery({ queryKey: ['ticker', symbol], queryFn: () => api.get(`/markets/${symbol}/ticker`), refetchInterval: 5_000, retry: false /* 503 is normal */ })
  // ...
}
```

### Form mutation (place order)

See "Place order" snippet above.

---

## Environment variables

```bash
# .env.local
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=<openssl rand -base64 32>
API_URL=http://localhost:8080                  # server-side (RSC + actions)
NEXT_PUBLIC_API_URL=http://localhost:8080      # client-side
```

In production, `NEXT_PUBLIC_API_URL` should match the public backend hostname (or be relative if same-origin proxied).

---

## Symbol whitelist coupling

Backend currently accepts **any** symbol on `POST /orders`. Frontend should restrict the SymbolPicker to the configured Binance symbols so users only trade pairs that have a reference price.

Source of truth: `GET /markets` returns the configured list. Cache once per session (`staleTime: Infinity` or fetch on app boot).

```ts
const { data: symbols } = useQuery({
  queryKey: ['marketSymbols'],
  queryFn: async () => (await api.get('/markets')).map((m: any) => m.symbol),
  staleTime: Infinity,
})
```

---

## What this spec doesn't decide

These are downstream choices for the design phase:

- Brand / visual identity (logo, color tokens, typography scale)
- Light-mode vs dark-default
- Mobile-first vs desktop-first layout
- Onboarding tour / empty-state CTAs
- Settings page (notification prefs, etc.)
- Internationalization (likely English-only MVP)
- Accessibility audit (target WCAG AA)

---

## Recommended build order

1. **Auth scaffold** — login, register, middleware, signed-in shell with sign-out
2. **API client + TanStack Query setup** — `useApi`, `apiServer`, query client provider
3. **Portfolio + wallets** — view balances, generate deposit address
4. **Markets list** — `/markets` page driven by Binance feed
5. **Trade view** — order book + place order form (the hardest page, do last among trader pages)
6. **Order + trade history** — read-only pages
7. **Withdraw flow** — modal or dedicated page
8. **Admin section** — deposit confirm/reject + intake form

Each phase ships independently. After step 5 the app is usable end-to-end (for a manually-credited test user).
