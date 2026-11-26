<script setup lang="ts">
import { date, money } from './api'
import { orderStatus } from './models'
import type { Order } from './models'

defineProps<{ order: Order }>()
</script>

<template>
  <section class="card order-detail">
    <div class="section-heading"><h2>订单 #{{ order.id }}</h2><span class="status">{{ orderStatus(order) }}</span></div>
    <dl>
      <div><dt>下单时间</dt><dd>{{ date(order.created_at) }}</dd></div>
      <div><dt>收款渠道</dt><dd>{{ order.receiving_account.provider }}</dd></div>
      <div v-if="order.address_snapshot"><dt>配送地址</dt><dd>{{ order.address_snapshot }}</dd></div>
      <div v-else><dt>团购场次</dt><dd>#{{ order.campaign_id }}</dd></div>
      <div v-if="order.arrived_at"><dt>到点时间</dt><dd>{{ date(order.arrived_at) }}</dd></div>
      <div v-if="order.redeemed_at"><dt>核销时间</dt><dd>{{ date(order.redeemed_at) }}</dd></div>
      <div v-if="order.hold_until && !order.payment_disposition && !order.canceled_at"><dt>库存保留至</dt><dd>{{ date(order.hold_until) }}</dd></div>
    </dl>
    <ul class="record-list"><li v-for="line in order.items" :key="line.sku_id.toString()">
      <span>{{ line.sku_name_snapshot }} × {{ line.quantity }}</span>
      <span>{{ money(line.unit_price_cents) }} / 件<span v-if="line.discount_cents"> · 已减 {{ money(line.discount_cents) }}</span></span>
    </li></ul>
    <p class="total">应付 <strong>{{ money(order.payable_cents) }}</strong><small v-if="order.refunded_cents">已退 {{ money(order.refunded_cents) }}</small></p>
    <slot />
  </section>
</template>
