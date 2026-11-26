<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import QRCode from 'qrcode'
import { choices, date, id, integer, list, money, request, retryKey, title, useTask } from './api'
import type { Integer, Row } from './api'
import type { Order } from './models'
import SelectField from './SelectField.vue'
import OrderDetail from './OrderDetail.vue'

interface Offer { sku_id: Integer; remaining: Integer | null }
interface Campaign { id: Integer; store_id: Integer; pickup_point_id: Integer; cutoff_at: string; offers: Offer[] }
interface SKU extends Row { available: Integer; unit_price_cents: Integer; product_id: Integer }

const props = defineProps<{ page: string }>()
const area = defineModel<{ community: string; circle: string; lastOrder: string }>('area', { required: true })
const { busy, run } = useTask()
const validation = ref('')
const notice = ref('')
const communities = ref<Row[]>([])
const circles = ref<Row[]>([])
const stores = ref<Row[]>([])
const points = ref<Row[]>([])
const buildings = ref<Row[]>([])
const campaigns = ref<Campaign[]>([])
const currentStore = ref<Row>()
const campaign = ref<Campaign>()
const products = ref<Row[]>([])
const skus = ref<SKU[]>([])
const accounts = ref<Row[]>([])
const quantities = ref<Record<string, string>>({})
const building = ref('')
const address = ref('')
const coupon = ref('')
const account = ref('')
const orders = ref<Row[]>([])
const order = ref<Order>()
const pickup = ref<{ code: string; image: string }>()
const cancelPending = ref(false)

const grouped = props.page === 'groups'
const availableSKUs = computed(() => campaign.value
  ? skus.value.filter(sku => campaign.value?.offers.some(offer => id(offer.sku_id) === id(sku.id)))
  : skus.value)
const items = computed(() => availableSKUs.value.flatMap(sku => {
  const quantity = quantities.value[id(sku.id)] || ''
  return /^[1-9][0-9]*$/.test(quantity) ? [{ sku_id: sku.id, quantity: integer(quantity) }] : []
}))
const subtotal = computed(() => items.value.reduce((sum, item) => {
  const sku = availableSKUs.value.find(row => row.id === item.sku_id)
  return sum + item.quantity * BigInt(sku?.unit_price_cents ?? 0)
}, 0n))
const pickupPoint = computed(() => points.value.find(row => id(row.id) === id(campaign.value?.pickup_point_id)))
const canPickup = computed(() => order.value?.payment_disposition === 'accepted' && !!order.value.pickup_point_id && !!order.value.arrived_at && !order.value.redeemed_at && !order.value.canceled_at)
const canCancel = computed(() => order.value && !order.value.payment_disposition && !order.value.canceled_at)

function remaining(sku: SKU) {
  const available = BigInt(sku.available)
  const quota = campaign.value?.offers.find(offer => id(offer.sku_id) === id(sku.id))?.remaining
  return quota == null || BigInt(quota) > available ? available : BigInt(quota)
}

function one(rows: Row[]) { return rows.length === 1 ? id(rows[0].id) : '' }
function storeTitle(storeId: Integer) { return title(stores.value.find(row => id(row.id) === id(storeId)) ?? { id: storeId }) }
function pointTitle(pointId: Integer) { return title(points.value.find(row => id(row.id) === id(pointId)) ?? { id: pointId }) }

function resetCheckout() {
  currentStore.value = undefined
  campaign.value = undefined
  quantities.value = {}
  validation.value = ''
  coupon.value = ''
  address.value = ''
  building.value = ''
  account.value = ''
}

async function loadCircle() {
  stores.value = []
  points.value = []
  buildings.value = []
  campaigns.value = []
  if (!area.value.circle) return
  const path = '/circles/' + area.value.circle
  stores.value = await list(path + '/stores')
  if (grouped) {
    const [newPoints, newCampaigns] = await Promise.all([list(path + '/pickup-points'), list<Campaign>(path + '/campaigns')])
    points.value = newPoints
    campaigns.value = newCampaigns
  } else {
    buildings.value = await list(path + '/buildings')
  }
}

async function loadCommunity() {
  circles.value = area.value.community ? await list('/communities/' + area.value.community + '/circles') : []
  if (!circles.value.some(row => id(row.id) === area.value.circle)) area.value.circle = one(circles.value)
  await loadCircle()
}

async function loadOrder(orderId: string) {
  pickup.value = undefined
  cancelPending.value = false
  const detail = await request<Order>('/orders/' + orderId)
  order.value = detail
  area.value.lastOrder = id(detail.id)
}

async function loadOrders() {
  orders.value = (await list('/orders')).reverse()
  if (area.value.lastOrder) await loadOrder(area.value.lastOrder)
}

onMounted(() => void run(async () => {
  if (props.page === 'orders') {
    await loadOrders()
  } else {
    communities.value = await list('/communities')
    if (!communities.value.some(row => id(row.id) === area.value.community)) area.value.community = one(communities.value)
    await loadCommunity()
  }
}))

function changeCommunity() {
  resetCheckout()
  area.value.circle = ''
  void run(loadCommunity)
}

function changeCircle() {
  resetCheckout()
  void run(loadCircle)
}

function openStore(store: Row, group?: Campaign) {
  void run(async () => {
    const path = '/stores/' + store.id
    const [newProducts, newSKUs, newAccounts] = await Promise.all([list(path + '/products'), list<SKU>(path + '/skus'), list(path + '/checkout-accounts')])
    resetCheckout()
    currentStore.value = store
    campaign.value = group
    products.value = newProducts
    skus.value = newSKUs
    accounts.value = newAccounts
    account.value = one(newAccounts)
    building.value = one(buildings.value)
  })
}

function openCampaign(group: Campaign) {
  const store = stores.value.find(row => id(row.id) === id(group.store_id))
  if (store) openStore(store, group)
}

function checkout() {
  if (!currentStore.value) return
  validation.value = ''
  if (!items.value.length || items.value.length > 100) {
    validation.value = '请选择 1 至 100 种商品并填写数量。'
    return
  }
  if (items.value.some(item => item.quantity > remaining(availableSKUs.value.find(sku => sku.id === item.sku_id)!))) {
    validation.value = '所选数量超过当前可购数量。'
    return
  }
  if (!campaign.value && !address.value.trim()) {
    validation.value = '请填写详细配送地址。'
    return
  }
  const payload = {
    store_id: currentStore.value.id, circle_id: integer(area.value.circle), receiving_account_id: integer(account.value),
    items: items.value, coupon_code: coupon.value.trim(),
    ...(campaign.value ? { campaign_id: campaign.value.id } : { building_id: integer(building.value), address: address.value.trim() }),
  }
  const fingerprint = JSON.stringify(payload, (_, value) => typeof value === 'bigint' ? value.toString() : value)
  const attempt = retryKey('checkout', fingerprint)
  void run(async () => {
    const placed = await request<Order>('/orders', 'POST', { ...payload, checkout_key: attempt.key })
    attempt.complete()
    area.value.lastOrder = id(placed.id)
    location.hash = '/h5/orders'
  })
}

function cancelOrder() {
  const selected = order.value
  if (!selected) return
  void run(async () => {
    await request('/orders/' + selected.id + '/cancel', 'POST')
    notice.value = '订单已取消。'
    await loadOrders()
  })
}

function showPickup() {
  const selected = order.value
  if (!selected) return
  void run(async () => {
    const { code } = await request<{ code: string }>('/orders/' + selected.id + '/pickup-code', 'POST')
    const payload = ['half-km', '1', selected.id, selected.pickup_point_id, code].join(':')
    pickup.value = { code, image: await QRCode.toDataURL(payload, { width: 240, margin: 3, errorCorrectionLevel: 'M' }) }
  })
}
</script>

<template>
  <template v-if="page === 'orders'">
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <OrderDetail v-if="order" :order="order">
      <p v-if="canCancel" class="muted">请使用商户提供的方式付款，登记成功后可在此刷新状态。</p>
      <div class="actions">
        <button class="secondary" :disabled="busy" @click="run(() => loadOrder(id(order?.id)))">刷新状态</button>
        <button v-if="canPickup && !pickup" :disabled="busy" @click="showPickup">显示提货码</button>
        <button v-if="canCancel && !cancelPending" class="secondary" :disabled="busy" @click="cancelPending = true">取消订单</button>
      </div>
      <div v-if="canCancel && cancelPending" class="actions">
        <p>确定取消这笔待付款订单？</p><button :disabled="busy" @click="cancelOrder">确认取消</button><button class="secondary" :disabled="busy" @click="cancelPending = false">返回</button>
      </div>
      <div v-if="pickup" class="pickup-code">
        <img :src="pickup.image" width="240" height="240" alt="团购订单提货二维码">
        <p>自提点 #{{ order.pickup_point_id }} · 订单 #{{ order.id }}</p>
        <p>手输提货码<code>{{ pickup.code }}</code></p>
      </div>
    </OrderDetail>
    <section class="card">
      <div class="section-heading"><h2>订单列表</h2><button class="secondary" :disabled="busy" @click="run(loadOrders)">刷新</button></div>
      <p v-if="!orders.length" class="empty">{{ busy ? '加载中…' : '暂无订单' }}</p>
      <ul v-else class="record-list">
        <li v-for="row in orders" :key="id(row.id)">
          <div><strong>{{ row.fulfillment === 'group_pickup' ? '团购自提' : '即时配送' }} #{{ row.id }}</strong><small>{{ date(row.created_at) }} · {{ money(row.payable_cents) }}</small></div>
          <button class="text-button" :disabled="busy" @click="run(() => loadOrder(id(row.id)))">查看</button>
        </li>
      </ul>
    </section>
  </template>
  <template v-else>
    <div class="toolbar">
      <SelectField v-model="area.community" label="社区" :options="choices(communities)" :disabled="busy" @change="changeCommunity" />
      <SelectField v-model="area.circle" label="服务圈" :options="choices(circles)" :disabled="busy || !area.community" @change="changeCircle" />
    </div>
    <form v-if="currentStore" class="card checkout" @submit.prevent="checkout">
      <div class="section-heading">
        <h2>{{ title(currentStore) }}</h2>
        <button class="secondary" type="button" :disabled="busy" @click="resetCheckout">返回列表</button>
      </div>
      <p v-if="campaign" class="muted">{{ pickupPoint ? title(pickupPoint) + ' · ' + pickupPoint.address : '自提点 #' + campaign.pickup_point_id }} · {{ date(campaign.cutoff_at) }} 截团</p>
      <p v-if="!availableSKUs.length" class="empty">暂无可购商品。</p>
      <ul v-else class="record-list">
        <li v-for="sku in availableSKUs" :key="id(sku.id)">
          <div><small>{{ title(products.find(row => id(row.id) === id(sku.product_id)) ?? { id: sku.product_id }) }}</small><strong>{{ title(sku) }}</strong><small>{{ money(sku.unit_price_cents) }} · 可购 {{ remaining(sku) }}</small></div>
          <label class="quantity">数量<input v-model="quantities[id(sku.id)]" :aria-label="title(sku) + '数量'" type="text" inputmode="numeric" pattern="[0-9]+" :disabled="busy || !remaining(sku)" placeholder="0"></label>
        </li>
      </ul>
      <div class="form-grid">
        <SelectField v-if="!campaign" v-model="building" label="配送楼栋" :options="choices(buildings)" :disabled="busy" required />
        <label v-if="!campaign">详细地址<input v-model="address" required maxlength="160" autocomplete="street-address"></label>
        <SelectField v-model="account" label="收款渠道" :options="choices(accounts, 'provider')" :disabled="busy" required />
        <label>优惠码（选填）<input v-model="coupon" maxlength="80"></label>
      </div>
      <p v-if="!accounts.length" class="field-error">商户尚未配置启用的收款渠道。</p>
      <p v-if="!campaign && !buildings.length" class="field-error">这个服务圈尚无可配送楼栋。</p>
      <p v-if="validation" class="field-error" role="alert">{{ validation }}</p>
      <div class="checkout-total">
        <div><strong>{{ money(subtotal) }}</strong><small>商品小计<template v-if="!campaign"> · 配送费 {{ money(currentStore.delivery_fee_cents) }}</template> · 实付以订单为准</small></div>
        <button :disabled="busy || !account || !availableSKUs.length || (!campaign && !building)">提交{{ campaign ? '团购' : '配送' }}订单</button>
      </div>
    </form>
    <p v-else-if="!area.circle" class="empty">选择社区和服务圈后查看附近好物。</p>
    <template v-else-if="grouped">
      <p v-if="!campaigns.length" class="empty">{{ busy ? '加载中…' : '当前服务圈暂无进行中的团购。' }}</p>
      <div v-else class="discovery">
        <article v-for="group in campaigns" :key="id(group.id)" class="card listing">
          <span class="status">社区自提</span>
          <h2>{{ storeTitle(group.store_id) }}</h2>
          <p>{{ pointTitle(group.pickup_point_id) }} · {{ date(group.cutoff_at) }} 截团</p>
          <p class="muted">{{ group.offers.length }} 种参团商品</p>
          <button :disabled="busy" @click="openCampaign(group)">查看团购</button>
        </article>
      </div>
    </template>
    <template v-else>
      <p v-if="!stores.length" class="empty">{{ busy ? '加载中…' : '当前服务圈暂无配送商户。' }}</p>
      <div v-else class="discovery">
        <article v-for="row in stores" :key="id(row.id)" class="card listing">
          <span class="status">商户配送</span>
          <h2>{{ title(row) }}</h2>
          <p class="muted">配送费 {{ money(row.delivery_fee_cents) }}</p>
          <button :disabled="busy" @click="openStore(row)">选购商品</button>
        </article>
      </div>
    </template>
  </template>
</template>
