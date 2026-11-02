"use client"

import {
  useState,
  useRef,
  useEffect,
  useContext,
  createContext,
  ReactNode,
  useMemo,
  useCallback,
} from "react"

import type { Product } from "@/components/product/product-card"
import { cartStorageKey, type CartItem } from "@/lib/cart"

interface CartContextValue {
  items: CartItem[]
  // False until the stored cart has been read - lets pages show a skeleton
  // instead of flashing "your cart is empty" on load.
  ready: boolean
  totalItems: number
  totalPrice: number
  getQuantity: (productId: string) => number
  addItem: (product: Product, quantity?: number) => void
  setQuantity: (productId: string, quantity: number) => void
  removeItem: (productId: string) => void
  // Replace stored product snapshots with fresh copies from the API (prices,
  // names, images). Items missing from `fresh` are left as they are - the
  // caller decides what "unavailable" looks like.
  refreshProducts: (fresh: Product[]) => void
  clear: () => void
  // Cart drawer visibility lives here (not in the header) so anything -
  // e.g. the product page's add-to-cart - can open it.
  isOpen: boolean
  setOpen: (open: boolean) => void
}

const CartContext = createContext<CartContextValue | undefined>(undefined)

function useCart(): CartContextValue {
  const context = useContext(CartContext)
  if (!context) {
    throw new Error("useCart must be used within a CartProvider")
  }
  return context
}

function readStoredCart(storageKey: string): CartItem[] {
  try {
    const raw = sessionStorage.getItem(storageKey)
    return raw ? (JSON.parse(raw) as CartItem[]) : []
  } catch {
    return []
  }
}

interface CartProviderProps {
  branchId: string
  children: ReactNode
}

function CartProvider({ branchId, children }: CartProviderProps) {
  const [items, setItems] = useState<CartItem[]>([])
  const [isOpen, setOpen] = useState(false)
  const [ready, setReady] = useState(false)
  // The key the current `items` were loaded from - null until hydrated.
  // Saving goes through this rather than the branchId prop so a branch
  // switch can't write the old branch's items under the new branch's key.
  const loadedKey = useRef<string | null>(null)

  // Server render (and the client's first render, before hydration) always
  // starts from an empty cart, so there's nothing to mismatch. Once mounted,
  // read sessionStorage and flip the (client-only) real state in.
  useEffect(() => {
    const storageKey = cartStorageKey(branchId)
    loadedKey.current = storageKey
    setItems(readStoredCart(storageKey))
    setReady(true)
  }, [branchId])

  useEffect(() => {
    if (!loadedKey.current) return
    try {
      sessionStorage.setItem(loadedKey.current, JSON.stringify(items))
    } catch {
      // ignore - privacy mode / storage disabled
    }
  }, [items])

  const addItem = useCallback((product: Product, quantity = 1) => {
    setItems((current) => {
      const existing = current.find((item) => item.product.id === product.id)
      if (existing) {
        return current.map((item) =>
          item.product.id === product.id
            ? { ...item, quantity: item.quantity + quantity }
            : item
        )
      }
      return [...current, { product, quantity }]
    })
  }, [])

  const setQuantity = useCallback((productId: string, quantity: number) => {
    setItems((current) => {
      if (quantity <= 0) {
        return current.filter((item) => item.product.id !== productId)
      }
      return current.map((item) =>
        item.product.id === productId ? { ...item, quantity } : item
      )
    })
  }, [])

  const removeItem = useCallback((productId: string) => {
    setItems((current) =>
      current.filter((item) => item.product.id !== productId)
    )
  }, [])

  const refreshProducts = useCallback((fresh: Product[]) => {
    const byId = new Map(fresh.map((p) => [p.id, p]))
    setItems((current) =>
      current.map((item) => {
        const latest = byId.get(item.product.id)
        return latest ? { ...item, product: latest } : item
      })
    )
  }, [])

  const clear = useCallback(() => setItems([]), [])

  const getQuantity = useCallback(
    (productId: string) =>
      items.find((item) => item.product.id === productId)?.quantity ?? 0,
    [items]
  )

  const totalItems = useMemo(
    () => items.reduce((sum, item) => sum + item.quantity, 0),
    [items]
  )

  const totalPrice = useMemo(
    () =>
      items.reduce(
        (sum, item) => sum + (item.product.price ?? 0) * item.quantity,
        0
      ),
    [items]
  )

  const value = useMemo<CartContextValue>(
    () => ({
      items,
      ready,
      totalItems,
      totalPrice,
      getQuantity,
      addItem,
      setQuantity,
      removeItem,
      refreshProducts,
      clear,
      isOpen,
      setOpen,
    }),
    [
      items,
      ready,
      totalItems,
      totalPrice,
      getQuantity,
      addItem,
      setQuantity,
      removeItem,
      refreshProducts,
      clear,
      isOpen,
    ]
  )

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>
}

export { CartProvider, useCart }
