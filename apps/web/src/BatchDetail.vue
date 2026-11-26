<script setup lang="ts">
import { date } from './api'
import type { Batch } from './models'
defineProps<{ batch: Batch }>()
</script>

<template>
  <section class="card">
    <div class="section-heading"><h2>场次 #{{ batch.campaign_id }} 分拣清单</h2><span class="status">{{ batch.arrived_at ? '已到点' : '待到点' }}</span></div>
    <p class="muted">自提点 #{{ batch.pickup_point_id }} · 截团 {{ date(batch.closed_at) }}<template v-if="batch.arrived_at"> · 到点 {{ date(batch.arrived_at) }}</template></p>
    <ul v-if="batch.sorting_lines.length" class="record-list">
      <li v-for="line in batch.sorting_lines" :key="line.sku_id.toString()"><span>{{ line.name }} <small>SKU #{{ line.sku_id }}</small></span><strong>{{ line.quantity }} 件</strong></li>
    </ul>
    <p v-else class="empty">本场次无分拣商品。</p>
    <details v-if="batch.order_statuses.length">
      <summary>入批订单（{{ batch.order_statuses.length }}）</summary>
      <ul class="record-list">
        <li v-for="row in batch.order_statuses" :key="row.order_id.toString()"><span>#{{ row.order_id }}</span><span>{{ row.redeemed_at ? '已核销' : row.canceled_at ? '已取消' : batch.arrived_at ? '待提货' : '待到点' }}</span></li>
      </ul>
    </details>
    <slot />
  </section>
</template>
