<script setup lang="ts">
import { date, money } from './api'
import type { Entry, Statement } from './models'
defineProps<{ entries: Entry[]; statements: Statement[]; busy: boolean }>()
</script>

<template>
  <section class="card">
    <h2>结算单</h2>
    <p v-if="!statements.length" class="empty">{{ busy ? '加载中…' : '暂无结算单' }}</p>
    <ul v-else class="record-list ledger">
      <li v-for="row in statements" :key="row.id.toString()">
        <div><strong>结算单 #{{ row.id }} · {{ money(row.total_cents) }}</strong><small>{{ row.store_name || '团长 #' + row.leader_id }} · {{ row.period_start }} 至 {{ row.period_end }}（UTC）</small></div>
        <div>
          <span class="status">{{ row.offline_paid_at ? '已付款' : '待付款' }}</span>
          <small v-if="row.offline_paid_at">{{ date(row.offline_paid_at) }}{{ row.offline_payment_reference ? ' · ' + row.offline_payment_reference : '' }}</small>
          <slot v-else name="payout" :statement="row" />
        </div>
      </li>
    </ul>
  </section>
  <section class="card">
    <h2>佣金流水</h2>
    <p v-if="!entries.length" class="empty">{{ busy ? '加载中…' : '暂无佣金流水' }}</p>
    <ul v-else class="record-list ledger">
      <li v-for="row in entries" :key="row.id.toString()">
        <div><strong>{{ row.entry_kind === 'refund_adjustment' ? '退款冲减' : '提货佣金' }} · {{ money(row.amount_cents) }}</strong><small>{{ row.store_name || '团长 #' + row.leader_id }} · 订单 #{{ row.order_id }}</small></div>
        <div><span class="status">{{ row.statement_id ? '结算单 #' + row.statement_id : '待结算' }}</span><small>可结算 {{ date(row.eligible_at) }}</small></div>
      </li>
    </ul>
  </section>
</template>
