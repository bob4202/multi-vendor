export interface Product {
  id: string // UUID
  plu: string | null
  name: string | null
  barcode: string | null
  category_id: string | null // the inventory system's department id, e.g. "dept-chicken"
  category_name: string | null // resolved from the categories table, null if uncategorized
  price: number | null // minor units (pence), e.g. 650 = £6.50
  description: string | null // branch override if set, else the shared product description
  image_urls: string[] | null // branch override if set, else the shared product image_urls
  is_upsell: boolean
  can_be_sold: boolean | null
  deleted: boolean
}

// GET /v1/store/products?sort= - the API's default is name_asc.
export type ProductSort = "name_asc" | "price_asc" | "price_desc"
