<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { choices, date, id, integer, list, money, request, title, useTask } from './api'
import type { Integer, Row } from './api'
import type { Batch, Entry, Order, Statement } from './models'
import SelectField from './SelectField.vue'
import OrderDetail from './OrderDetail.vue'
import BatchDetail from './BatchDetail.vue'
import CommissionTables from './CommissionTables.vue'

const props = defineProps<{ page: string; store: Integer }>()
const path = '/stores/' + props.store
const { busy, run } = useTask()
const notice = ref('')
const validation = ref('')
const orders = ref<Row[]>([])
const order = ref<Order>()
const campaigns = ref<Row[]>([])
const batch = ref<Batch>()
const circles = ref<Row[]>([])
const points = ref<Row[]>([])
const leaders = ref<Row[]>([])
const skus = ref<Row[]>([])
const circle = ref('')
const point = ref('')
const leader = ref('')
const cutoff = ref('')
const rate = ref('0')
const offers = ref([{ sku: '', quota: '1' }])
const statements = ref<Statement[]>([])
const entries = ref<Entry[]>([])
const periodLeader = ref('')
const periodStart = ref('')
const periodEnd = ref('')
const statement = ref<Statement>()
const reference = ref('')
const paidAt = ref('')
const utcToday = new Date().toISOString().slice(0, 10)

const settlementLeaders = computed(() => [...new Set(entries.value.map(entry => id(entry.leader_id)))].map(value => ({ value, label: '团长 #' + value })))
const nextDelivery = computed(() => {
  if (!order.value || order.value.canceled_at || order.value.payment_disposition !== 'accepted') return null
  if (order.value.delivery_state === 'delivered') return null
  if (order.value.delivery_state === 'out_for_delivery') return { state: 'delivered', label: '确认送达' }
  if (order.value.delivery_state === 'accepted') return { state: 'out_for_delivery', label: '开始配送' }
  return { state: 'accepted', label: '确认接单' }
})

async function loadSettlement() {
  const [newStatements, newEntries] = await Promise.all([
    list<Statement>(path + '/commission-statements'), list<Entry>(path + '/commission-entries'),
  ])
  statements.value = newStatements
  entries.value = newEntries
}

async function loadCampaigns() {
  campaigns.value = (await list(path + '/campaigns')).reverse()
}

async function loadOrders() {
  orders.value = (await list(path + '/orders')).filter(row => row.fulfillment === 'merchant_delivery').reverse()
}

onMounted(() => void run(async () => {
  if (props.page === 'orders') await loadOrders()
  if (props.page === 'settlement') await loadSettlement()
  if (props.page === 'campaigns') {
    const [allCircles, allSKUs] = await Promise.all([list(path + '/circles'), list(path + '/skus'), loadCampaigns()])
    circles.value = allCircles.filter(row => row.active)
    skus.value = allSKUs
  }
}))

function viewOrder(orderId: Integer) {
  void run(async () => { order.value = await request<Order>('/orders/' + orderId) })
}

function deliver() {
  const selected = order.value
  const next = nextDelivery.value
  if (!selected || !next) return
  void run(async () => {
    const url = '/orders/' + selected.id
    await request(url + '/delivery', 'PUT', { state: next.state })
    notice.value = '订单履约状态已更新。'
    order.value = await request<Order>(url)
  })
}

function changeCircle() {
  point.value = ''
  leader.value = ''
  points.value = []
  leaders.value = []
  if (circle.value) void run(async () => { points.value = await list('/circles/' + circle.value + '/pickup-points') })
}

function changePoint() {
  leader.value = ''
  leaders.value = []
  if (point.value) void run(async () => { leaders.value = (await list('/pickup-points/' + point.value + '/leaders')).filter(row => row.active) })
}

function openCampaign() {
  validation.value = ''
  if (new Date(cutoff.value).getTime() <= Date.now()) {
    validation.value = '截团时间须晚于当前时间。'
    return
  }
  if (new Set(offers.value.map(row => row.sku)).size !== offers.value.length) {
    validation.value = '每个 SKU 只能配置一次。'
    return
  }
  void run(async () => {
    const created = await request<{ id: Integer }>(path + '/campaigns', 'POST', {
      circle_id: integer(circle.value), pickup_point_id: integer(point.value), leader_id: integer(leader.value),
      cutoff_at: new Date(cutoff.value).toISOString(), commission_rate_bps: Number(rate.value),
      offers: offers.value.map(row => ({ sku_id: integer(row.sku), quota: integer(row.quota) })),
    })
    notice.value = '场次 #' + created.id + ' 已发布。'
    cutoff.value = ''
    offers.value = [{ sku: '', quota: '1' }]
    await loadCampaigns()
  })
}

function campaignAction(row: Row, action: 'batch' | 'close' | 'arrival') {
  void run(async () => {
    const url = path + '/campaigns/' + row.id
    if (action !== 'batch') {
      await request(url + '/' + action, 'POST')
      notice.value = action === 'close' ? '已截团。' : '已登记到点。'
      await loadCampaigns()
    }
    batch.value = await request<Batch>(url + '/batch')
  })
}

function generate() {
  validation.value = ''
  if (periodStart.value >= periodEnd.value) {
    validation.value = '结束日期须晚于开始日期。'
    return
  }
  void run(async () => {
    await request(path + '/commission-statements', 'POST', {
      leader_id: integer(periodLeader.value), period_start: periodStart.value, period_end: periodEnd.value,
    })
    notice.value = '结算单已生成。'
    await loadSettlement()
  })
}

function payout() {
  const selected = statement.value
  if (!selected) return
  validation.value = ''
  if (new Date(paidAt.value).getTime() > Date.now()) {
    validation.value = '付款时间须为当前或过去时间。'
    return
  }
  void run(async () => {
    await request(path + '/commission-statements/' + selected.id + '/payout', 'PUT', {
      offline_payment_reference: reference.value.trim(), offline_paid_at: new Date(paidAt.value).toISOString(), amount_cents: selected.total_cents,
    })
    statement.value = undefined
    reference.value = ''
    paidAt.value = ''
    notice.value = '线下付款已登记。'
    await loadSettlement()
  })
}
</script>

<template>
  <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  <template v-if="page === 'orders'">
    <section class="card">
      <div class="section-heading"><h2>即时配送订单</h2><button class="secondary" :disabled="busy" @click="run(loadOrders)">刷新</button></div>
      <p v-if="!orders.length" class="empty">{{ busy ? '加载中…' : '暂无即时配送订单' }}</p>
      <div v-else class="table-scroll"><table>
        <thead><tr><th>订单编号</th><th>下单时间</th><th>应付</th><th>收款状态</th><th>操作</th></tr></thead>
        <tbody><tr v-for="row in orders" :key="id(row.id)">
          <td>#{{ row.id }}</td><td>{{ date(row.created_at) }}</td><td>{{ money(row.payable_cents) }}</td>
          <td>{{ row.disposition === 'refund_required' ? '待退款' : row.canceled_at ? '已取消' : row.disposition === 'accepted' ? '已收款' : '待付款' }}</td>
          <td><button class="text-button" :disabled="busy" @click="viewOrder(row.id)">查看订单</button></td>
        </tr></tbody>
      </table></div>
    </section>
    <OrderDetail v-if="order" :order="order">
      <button v-if="nextDelivery" :disabled="busy" @click="deliver">{{ nextDelivery.label }}</button>
    </OrderDetail>
  </template>
  <template v-else-if="page === 'campaigns'">
    <section class="card">
      <h2>发布团购</h2>
      <form class="form-grid" @submit.prevent="openCampaign">
        <SelectField v-model="circle" label="服务圈" :options="choices(circles)" required :disabled="busy" @change="changeCircle" />
        <SelectField v-model="point" label="自提点" :options="choices(points)" required :disabled="busy || !circle" @change="changePoint" />
        <SelectField v-model="leader" label="授权团长" :options="choices(leaders, 'display_name')" required :disabled="busy || !point" />
        <label>截团时间<input v-model="cutoff" type="datetime-local" required></label>
        <label>佣金比例（基点，100 = 1%）<input v-model="rate" type="number" min="0" max="10000" step="1" required></label>
        <fieldset class="full">
          <legend>参团 SKU 与配额</legend>
          <div v-for="(offer, index) in offers" :key="index" class="form-grid form-row">
            <SelectField v-model="offer.sku" label="SKU" :options="choices(skus)" required :disabled="busy" />
            <label>付款配额<input v-model="offer.quota" inputmode="numeric" pattern="[1-9][0-9]*" required></label>
            <button v-if="offers.length > 1" class="secondary" type="button" :disabled="busy" @click="offers.splice(index, 1)">移除</button>
          </div>
          <button class="secondary" type="button" :disabled="busy || offers.length >= Math.min(skus.length, 100)" @click="offers.push({ sku: '', quota: '1' })">添加 SKU</button>
        </fieldset>
        <p v-if="validation" class="field-error full" role="alert">{{ validation }}</p>
        <div class="full"><button :disabled="busy || !leader || !skus.length">发布场次</button></div>
      </form>
    </section>
    <section class="card">
      <div class="section-heading"><h2>团购场次</h2><button class="secondary" :disabled="busy" @click="run(loadCampaigns)">刷新</button></div>
      <p v-if="!campaigns.length" class="empty">{{ busy ? '加载中…' : '暂无团购场次' }}</p>
      <div v-else class="table-scroll"><table>
        <thead><tr><th>场次</th><th>服务圈</th><th>截团时间</th><th>状态</th><th>操作</th></tr></thead>
        <tbody><tr v-for="row in campaigns" :key="id(row.id)">
          <td>#{{ row.id }}</td><td>{{ title(circles.find(item => id(item.id) === id(row.circle_id)) ?? { id: row.circle_id as Integer }) }}</td>
          <td>{{ date(row.cutoff_at) }}</td>
          <td>{{ row.arrived_at ? '已到点' : row.closed_at ? '已截团' : Date.parse(id(row.cutoff_at)) <= Date.now() ? '待截团' : '进行中' }}</td>
          <td class="actions">
            <button v-if="row.closed_at" class="text-button" :disabled="busy" @click="campaignAction(row, 'batch')">查看分拣</button>
            <button v-if="row.closed_at && !row.arrived_at" class="secondary" :disabled="busy" @click="campaignAction(row, 'arrival')">登记到点</button>
            <button v-if="!row.closed_at && Date.parse(id(row.cutoff_at)) <= Date.now()" class="secondary" :disabled="busy" @click="campaignAction(row, 'close')">补跑截团</button>
          </td>
        </tr></tbody>
      </table></div>
    </section>
    <BatchDetail v-if="batch" :batch="batch" />
  </template>
  <template v-else>
    <section class="card">
      <div class="section-heading"><h2>生成结算单</h2><button class="secondary" :disabled="busy" @click="run(loadSettlement)">刷新</button></div>
      <form class="form-grid" @submit.prevent="generate">
        <SelectField v-model="periodLeader" label="团长" :options="settlementLeaders" :disabled="busy" required />
        <label>开始日期（UTC）<input v-model="periodStart" type="date" :max="utcToday" required></label>
        <label>结束日期（UTC，不含当日）<input v-model="periodEnd" type="date" :max="utcToday" required></label>
        <p v-if="validation && !statement" class="field-error full" role="alert">{{ validation }}</p>
        <div class="full"><button :disabled="busy || !periodLeader">生成结算单</button></div>
      </form>
    </section>
    <section v-if="statement" class="card">
      <h2>线下付款登记 · 结算单 #{{ statement.id }}</h2>
      <p>已付金额 {{ money(statement.total_cents) }}。请在实际完成付款后登记凭证。</p>
      <form class="form-grid" @submit.prevent="payout">
        <label>线下付款凭证<input v-model="reference" required maxlength="128" pattern=".*\S.*"></label>
        <label>实际付款时间<input v-model="paidAt" type="datetime-local" required></label>
        <p v-if="validation" class="field-error full" role="alert">{{ validation }}</p>
        <div class="actions full"><button :disabled="busy">登记已付款</button><button class="secondary" type="button" @click="statement = undefined; validation = ''">取消</button></div>
      </form>
    </section>
    <CommissionTables :entries="entries" :statements="statements" :busy="busy">
      <template #payout="{ statement: current }"><button class="text-button" :disabled="busy" @click="statement = current; reference = ''; paidAt = ''; validation = ''">登记付款</button></template>
    </CommissionTables>
  </template>
</template>
