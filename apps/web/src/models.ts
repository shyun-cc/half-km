import type { Integer } from './api'

export interface Order {
  id: Integer
  store_id: Integer
  fulfillment: 'merchant_delivery' | 'group_pickup'
  payable_cents: Integer
  created_at: string
  canceled_at: string | null
  hold_until: string | null
  campaign_id: Integer | null
  pickup_point_id: Integer | null
  arrived_at: string | null
  address_snapshot: string | null
  receiving_account: { id: Integer; provider: string; merchant_number: string }
  refunded_cents: Integer
  payment_disposition: 'accepted' | 'refund_required' | null
  delivery_state: 'accepted' | 'out_for_delivery' | 'delivered' | null
  redeemed_at: string | null
  items: { sku_id: Integer; sku_name_snapshot: string; quantity: Integer; unit_price_cents: Integer; discount_cents: Integer }[]
}

export interface Batch {
  campaign_id: Integer
  pickup_point_id: Integer
  closed_at: string
  arrived_at: string | null
  order_statuses: { order_id: Integer; canceled_at: string | null; redeemed_at: string | null }[]
  sorting_lines: { sku_id: Integer; name: string; quantity: Integer }[]
}

export interface Entry {
  id: Integer
  order_id: Integer
  leader_id?: Integer
  store_name?: string
  statement_id: Integer | null
  entry_kind: 'earned' | 'refund_adjustment'
  amount_cents: Integer
  eligible_at: string
}

export interface Statement {
  id: Integer
  leader_id?: Integer
  store_name?: string
  period_start: string
  period_end: string
  total_cents: Integer
  offline_payment_reference?: string
  offline_paid_at: string | null
}

export function orderStatus(order: Order) {
  if (order.payment_disposition === 'refund_required') return order.refunded_cents >= order.payable_cents ? '已退款' : '待退款'
  if (order.canceled_at) return '已取消'
  if (order.redeemed_at) return '已核销'
  if (order.delivery_state === 'delivered') return '已送达'
  if (order.delivery_state === 'out_for_delivery') return '配送中'
  if (order.delivery_state === 'accepted') return '已接单'
  if (!order.payment_disposition) return '待付款'
  if (order.fulfillment === 'merchant_delivery') return '待接单'
  return order.arrived_at ? '可提货' : '待到点'
}
