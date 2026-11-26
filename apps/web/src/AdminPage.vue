<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { choices, id, integer, list, request, title, useTask } from './api'
import type { Row, Session } from './api'
import ResourceEditor from './ResourceEditor.vue'
import type { Field } from './ResourceEditor.vue'
import SelectField from './SelectField.vue'

const props = defineProps<{ page: string; session: Session }>()
const { busy, run } = useTask()
const notice = ref('')
const community = ref('')
const circle = ref('')
const store = ref(props.session.role === 'merchant' ? id(props.session.id) : '')
const point = ref('')
const communities = ref<Row[]>([])
const circles = ref<Row[]>([])
const buildings = ref<Row[]>([])
const allowedBuildings = ref<Row[]>([])
const stores = ref<Row[]>([])
const storeCircles = ref<Row[]>([])
const points = ref<Row[]>([])
const leaders = ref<Row[]>([])
const pointLeaders = ref<Row[]>([])
const products = ref<Row[]>([])
const skus = ref<Row[]>([])
const stock = ref<Row[]>([])
const coupons = ref<Row[]>([])
const accounts = ref<Row[]>([])

const name: Field = { key: 'name', label: '名称' }
const active: Field = { key: 'active', label: '可用状态', kind: 'checkbox' }
const count = (key: string, label: string) => ({ key, label, kind: 'integer' } as const)
const area = computed<Field>(() => ({ key: 'community_id', label: '所属社区', kind: 'select', options: choices(communities.value.filter(row => id(row.id) === community.value)) }))
const service = computed<Field>(() => ({ key: 'circle_id', label: '所属服务圈', kind: 'select', options: choices(circles.value.filter(row => id(row.id) === circle.value)) }))
const pointRows = computed(() => points.value.map(row => ({ ...row, circle_id: circle.value })))
const skuFields = computed<Field[]>(() => [
  name, { key: 'product_id', label: '商品', kind: 'select', options: choices(products.value) }, count('unit_price_cents', '单价（分）'),
])
const storeFields = [name, count('delivery_fee_cents', '配送费（分）')]
const couponFields: Field[] = [
  { key: 'code', label: '优惠码' }, { ...count('discount_cents', '优惠（分）'), minimum: 1 },
  count('minimum_cents', '门槛（分）'), { key: 'expires_at', label: '到期时间', kind: 'datetime' }, active,
]
const accountFields: Field[] = [{ key: 'provider', label: '支付渠道' }, { key: 'merchant_number', label: '商户号' }, active]
const stockRows = computed(() => stock.value.map(row => ({
  ...row, name: skus.value.find(sku => id(sku.id) === id(row.id))?.name ?? row.id,
})))
const stockFields = [name, count('on_hand', '实物库存'), count('reserved', '已锁定')]
const storePath = computed(() => '/stores/' + store.value)
const picking = computed(() => props.page === 'partners')
const geography = computed(() => props.page === 'community' || picking.value)

async function loadDirectories() {
  if (geography.value) communities.value = await list('/communities')
  if (props.session.role === 'operator' && props.page !== 'community') stores.value = await list('/stores')
  if (picking.value) leaders.value = await list('/leaders')
}

async function loadCommunity() {
  circles.value = []
  buildings.value = []
  if (!community.value) return
  const base = '/communities/' + community.value
  const [newCircles, newBuildings] = await Promise.all([
    list(base + '/circles'), picking.value ? Promise.resolve([]) : list(base + '/buildings'),
  ])
  circles.value = newCircles
  buildings.value = newBuildings
}

async function loadCircle() {
  points.value = []
  allowedBuildings.value = []
  if (!circle.value) return
  const path = '/circles/' + circle.value
  if (picking.value) points.value = await list(path + '/pickup-points')
  else allowedBuildings.value = await list(path + '/buildings')
}

async function loadPoint() {
  pointLeaders.value = point.value ? await list('/pickup-points/' + point.value + '/leaders') : []
}

async function loadStore() {
  storeCircles.value = []
  if (!store.value) return
  const path = storePath.value
  if (picking.value) {
    storeCircles.value = await list(path + '/circles')
  } else {
    const [newProducts, newSKUs, newStock, newCoupons, newAccounts] = await Promise.all(
      ['products', 'skus', 'stock', 'coupons', 'receiving-accounts'].map(resource => list(path + '/' + resource)),
    )
    products.value = newProducts
    skus.value = newSKUs
    stock.value = newStock
    coupons.value = newCoupons
    accounts.value = newAccounts
  }
}

onMounted(() => void run(async () => {
  await loadDirectories()
  if (communities.value.length === 1) community.value = id(communities.value[0].id)
  await loadCommunity()
  if (circles.value.length === 1) circle.value = id(circles.value[0].id)
  await loadCircle()
  if (!store.value && stores.value.length === 1) store.value = id(stores.value[0].id)
  await loadStore()
}))

function changeCommunity() {
  circle.value = ''
  point.value = ''
  void run(async () => {
    await loadCommunity()
    if (circles.value.length === 1) circle.value = id(circles.value[0].id)
    await loadCircle()
    await loadPoint()
  })
}

function changeCircle() {
  point.value = ''
  void run(async () => {
    await loadCircle()
    await loadPoint()
  })
}

function save(path: string, rowId: string, body: Record<string, unknown>, refresh: () => Promise<void>) {
  void run(async () => {
    await request(path + (rowId ? '/' + rowId : ''), rowId ? 'PUT' : 'POST', body)
    notice.value = '已保存。'
    await refresh()
  })
}

function enabled(rows: Row[], row: Row) {
  return rows.some(item => id(item.id) === id(row.id) && item.active === true)
}

function toggle(path: string, value: boolean, refresh: () => Promise<void>) {
  void run(async () => {
    await request(path, 'PUT', { active: value })
    notice.value = value ? '已启用。' : '已停用。'
    await refresh()
  })
}

function setStock(row: Row, event: Event) {
  const form = event.currentTarget as HTMLFormElement
  const onHand = integer(String(new FormData(form).get('on_hand')))
  void run(async () => {
    await request(storePath.value + '/skus/' + row.id + '/stock', 'PUT', { on_hand: onHand })
    notice.value = '库存已更新。'
    await loadStore()
  })
}
</script>

<template>
  <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  <div v-if="geography || session.role === 'operator'" class="toolbar">
    <SelectField v-if="geography" v-model="community" label="当前社区" :options="choices(communities)" :disabled="busy" @change="changeCommunity" />
    <SelectField v-if="geography" v-model="circle" label="当前服务圈" :options="choices(circles)" :disabled="busy" @change="changeCircle" />
    <SelectField v-if="page !== 'community'" v-model="store" label="当前商户" :options="choices(stores)" :disabled="busy" @change="run(async () => { products = []; skus = []; stock = []; coupons = []; accounts = []; await loadStore() })" />
  </div>
  <template v-if="page === 'community'">
    <ResourceEditor title="社区" :rows="communities" :fields="[name]" :busy="busy" @save="(row, body) => save('/communities', row, body, loadDirectories)" />
    <template v-if="community">
      <ResourceEditor :key="'circle-' + community" title="服务圈" :rows="circles" :fields="[name, area]" :busy="busy" @save="(row, body) => save('/circles', row, body, loadCommunity)" />
      <ResourceEditor :key="'building-' + community" title="楼栋" :rows="buildings" :fields="[name, area]" :busy="busy" @save="(row, body) => save('/buildings', row, body, loadCommunity)" />
      <section v-if="circle" class="card">
        <h2>楼栋白名单</h2>
        <p v-if="!buildings.length" class="empty">请先在当前社区添加楼栋。</p>
        <ul v-else class="record-list"><li v-for="row in buildings" :key="id(row.id)">
          <span>{{ title(row) }}</span>
          <button type="button" class="secondary" :disabled="busy" @click="toggle('/circles/' + circle + '/buildings/' + row.id, !enabled(allowedBuildings, row), loadCircle)">{{ enabled(allowedBuildings, row) ? '已启用 · 移出' : '未启用 · 加入' }}</button>
        </li></ul>
      </section>
    </template>
  </template>
  <template v-else-if="picking">
    <ResourceEditor title="商户" :rows="stores" :fields="storeFields" :busy="busy" @save="(row, body) => save('/stores', row, body, loadDirectories)" />
    <ResourceEditor title="团长" :rows="leaders" :fields="[{ key: 'display_name', label: '团长姓名' }]" :busy="busy" @save="(row, body) => save('/leaders', row, body, loadDirectories)" />
    <section v-if="store && community" class="card">
      <h2>商户服务范围</h2>
      <p v-if="!circles.length" class="empty">当前社区暂无服务圈。</p>
      <ul v-else class="record-list"><li v-for="row in circles" :key="id(row.id)">
        <span>{{ title(row) }}</span>
        <button type="button" class="secondary" :disabled="busy" @click="toggle(storePath + '/circles/' + row.id, !enabled(storeCircles, row), loadStore)">{{ enabled(storeCircles, row) ? '已服务 · 停用' : '未服务 · 启用' }}</button>
      </li></ul>
    </section>
    <template v-if="circle">
      <ResourceEditor :key="'point-' + circle" title="自提点" :rows="pointRows" :fields="[name, { key: 'address', label: '地址' }, service]" :busy="busy" @save="(row, body) => save('/pickup-points', row, body, loadCircle)" />
      <section class="card">
        <h2>自提点团长授权</h2>
        <SelectField v-model="point" label="自提点" :options="choices(points)" :disabled="busy" @change="run(loadPoint)" />
        <ul v-if="point" class="record-list"><li v-for="row in leaders" :key="id(row.id)">
          <span>{{ title(row, 'display_name') }}</span>
          <button type="button" class="secondary" :disabled="busy" @click="toggle('/pickup-points/' + point + '/leaders/' + row.id, !enabled(pointLeaders, row), loadPoint)">{{ enabled(pointLeaders, row) ? '已授权 · 停用' : '未授权 · 启用' }}</button>
        </li></ul>
      </section>
    </template>
  </template>
  <template v-else-if="store">
    <ResourceEditor :key="'product-' + store" title="商品" :rows="products" :fields="[name]" :busy="busy" @save="(row, body) => save(storePath + '/products', row, body, loadStore)" />
    <ResourceEditor :key="'sku-' + store" title="SKU" :rows="skus" :fields="skuFields" :busy="busy" @save="(row, body) => save(storePath + '/skus', row, body, loadStore)" />
    <section class="card">
      <h2>实物库存</h2>
      <p v-if="!stockRows.length" class="empty">添加 SKU 后即可维护库存。</p>
      <div v-else class="table-scroll">
        <table>
          <thead><tr><th v-for="field in stockFields" :key="field.key">{{ field.label }}</th><th>调整实物库存</th></tr></thead>
          <tbody><tr v-for="row in stockRows" :key="id(row.id)">
            <td v-for="field in stockFields" :key="field.key">{{ id((row as Row)[field.key]) }}</td>
            <td><form class="actions" @submit.prevent="setStock(row, $event)">
              <input :key="id((row as Row).on_hand)" name="on_hand" :aria-label="row.name + '实物库存'" :value="id((row as Row).on_hand)" inputmode="numeric" pattern="[0-9]+" required size="8">
              <button class="secondary" :disabled="busy">更新</button>
            </form></td>
          </tr></tbody>
        </table>
      </div>
    </section>
    <ResourceEditor :key="'coupon-' + store" title="优惠券" :rows="coupons" :fields="couponFields" :busy="busy" @save="(row, body) => save(storePath + '/coupons', row, body, loadStore)" />
    <ResourceEditor :key="'account-' + store" title="收款渠道" :rows="accounts" :fields="accountFields" :update-fields="[active]" :busy="busy" @save="(row, body) => save(storePath + '/receiving-accounts', row, body, loadStore)" />
  </template>
  <p v-else class="empty">选择商户后查看商品、库存和收款渠道。</p>
</template>
