<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import jsQR from 'jsqr'
import { date, id, integer, list, request, retryKey, useTask } from './api'
import type { Row } from './api'
import type { Batch, Entry, Statement } from './models'
import BatchDetail from './BatchDetail.vue'
import CommissionTables from './CommissionTables.vue'
import SelectField from './SelectField.vue'

const props = defineProps<{ page: string }>()
const { busy, run } = useTask()
const batches = ref<Row[]>([])
const batch = ref<Batch>()
const entries = ref<Entry[]>([])
const statements = ref<Statement[]>([])
const order = ref('')
const code = ref('')
const point = ref('')
const validation = ref('')
const receipt = ref('')
const video = ref<HTMLVideoElement>()
const scanning = ref(false)
const starting = ref(false)
let stream: MediaStream | undefined
let frame = 0
let cameraRequest = 0

const pointOptions = computed(() => {
  const arrived = batches.value.filter(row => row.arrived_at)
  return [...new Map(arrived.map(row => [id(row.pickup_point_id), {
    value: id(row.pickup_point_id), label: id(row.pickup_point_name) + ' #' + id(row.pickup_point_id),
  }])).values()]
})

async function loadBatches() {
  batches.value = (await list('/leader/batches')).reverse()
  if (pointOptions.value.length === 1) point.value = pointOptions.value[0].value
  else if (!pointOptions.value.some(option => option.value === point.value)) point.value = ''
  if (batch.value) batch.value = await request<Batch>('/leader/batches/' + batch.value.campaign_id)
}

async function loadCommission() {
  const [newEntries, newStatements] = await Promise.all([
    list<Entry>('/leader/commission-entries'), list<Statement>('/leader/commission-statements'),
  ])
  entries.value = newEntries
  statements.value = newStatements
}

function stopCamera() {
  cameraRequest++
  cancelAnimationFrame(frame)
  stream?.getTracks().forEach(track => track.stop())
  stream = undefined
  if (video.value) video.value.srcObject = null
  scanning.value = false
  starting.value = false
}

function readScan(value: string) {
  const match = /^half-km:1:([1-9][0-9]*):([1-9][0-9]*):([A-Za-z0-9_-]{43})$/.exec(value)
  if (!match) {
    validation.value = '这张二维码不是半公里提货码，请重新扫码或手动输入。'
    return
  }
  if (!pointOptions.value.some(option => option.value === match[2])) {
    validation.value = '当前没有该自提点的已到点批次，请先刷新。'
    return
  }
  validation.value = ''
  receipt.value = ''
  order.value = match[1]
  point.value = match[2]
  code.value = match[3]
}

async function startCamera() {
  validation.value = ''
  receipt.value = ''
  if (!navigator.mediaDevices?.getUserMedia) {
    validation.value = '当前页面无法使用摄像头，请手动输入提货码。'
    return
  }
  const requestId = ++cameraRequest
  starting.value = true
  try {
    const camera = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } }, audio: false })
    const element = video.value
    if (requestId !== cameraRequest || !element) {
      camera.getTracks().forEach(track => track.stop())
      return
    }
    stream = camera
    element.srcObject = camera
    scanning.value = true
    await element.play()
    if (requestId !== cameraRequest) return
    const canvas = document.createElement('canvas')
    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) throw new Error('Canvas unavailable')
    let lastCapture = 0
    const scan = (now: number) => {
      if (now - lastCapture > 150 && element.videoWidth && element.videoHeight) {
        lastCapture = now
        canvas.width = Math.min(element.videoWidth, 640)
        canvas.height = Math.round(element.videoHeight * canvas.width / element.videoWidth)
        context.drawImage(element, 0, 0, canvas.width, canvas.height)
        const pixels = context.getImageData(0, 0, canvas.width, canvas.height)
        const decoded = jsQR(pixels.data, pixels.width, pixels.height, { inversionAttempts: 'dontInvert' })
        if (decoded) {
          stopCamera()
          readScan(decoded.data)
          return
        }
      }
      frame = requestAnimationFrame(scan)
    }
    frame = requestAnimationFrame(scan)
  } catch {
    if (requestId === cameraRequest) {
      stopCamera()
      validation.value = '无法打开摄像头。请检查权限，或手动输入。'
    }
  } finally {
    if (requestId === cameraRequest) starting.value = false
  }
}

function redeem() {
  if (!/^[1-9][0-9]*$/.test(order.value) || !point.value || !/^[A-Za-z0-9_-]{43}$/.test(code.value.trim())) return
  validation.value = ''
  stopCamera()
  const selectedOrder = order.value
  const payload = { pickup_point_id: integer(point.value), code: code.value.trim() }
  const fingerprint = [selectedOrder, point.value, payload.code].join(':')
  const attempt = retryKey('redemption', fingerprint)
  void run(async () => {
    const result = await request<{ redeemed_at: string }>('/orders/' + selectedOrder + '/redeem', 'POST', { ...payload, event_id: attempt.key })
    attempt.complete()
    receipt.value = '订单 #' + selectedOrder + ' 已核销 · ' + date(result.redeemed_at)
    order.value = ''
    code.value = ''
  })
}

onMounted(() => void run(props.page === 'commission' ? loadCommission : loadBatches))
onBeforeUnmount(stopCamera)
</script>

<template>
  <template v-if="page === 'sorting'">
    <section class="card">
      <div class="section-heading"><h2>已截团批次</h2><button class="secondary" :disabled="busy" @click="run(loadBatches)">刷新</button></div>
      <p v-if="!batches.length" class="empty">{{ busy ? '加载中…' : '暂无可查看的分拣批次。' }}</p>
      <ul v-else class="record-list">
        <li v-for="row in batches" :key="id(row.id)">
          <div><strong>{{ row.store_name }} · #{{ row.id }}</strong><small>{{ row.pickup_point_name }} · {{ row.arrived_at ? '已到点' : '待到点' }}</small></div>
          <button class="text-button" :disabled="busy" @click="run(async () => { batch = await request<Batch>('/leader/batches/' + row.id) })">查看分拣</button>
        </li>
      </ul>
    </section>
    <BatchDetail v-if="batch" :batch="batch" />
  </template>
  <section v-else-if="page === 'redeem'" class="card">
    <div class="section-heading"><h2>核对居民提货码</h2><button class="secondary" :disabled="busy" @click="run(loadBatches)">刷新批次</button></div>
    <p v-if="!pointOptions.length" class="empty">{{ busy ? '加载中…' : '暂无已到点且已授权的批次。' }}</p>
    <div class="actions">
      <button v-if="!scanning && !starting" class="secondary" :disabled="busy || !pointOptions.length" @click="startCamera">打开摄像头</button>
      <button v-else class="secondary" @click="stopCamera">关闭摄像头</button>
    </div>
    <video v-show="scanning" ref="video" class="scanner" playsinline muted aria-label="提货码扫描取景"></video>
    <p v-if="validation" class="field-error" role="alert">{{ validation }}</p>
    <p v-if="receipt" class="notice" role="status">{{ receipt }}</p>
    <form class="form-grid" @submit.prevent="redeem">
      <SelectField v-model="point" label="自提点" :options="pointOptions" :disabled="busy" required />
      <label>订单编号<input v-model="order" inputmode="numeric" pattern="[1-9][0-9]*" required :disabled="busy" @input="receipt = ''"></label>
      <label class="full">提货码<input v-model.trim="code" pattern="[A-Za-z0-9_\-]{43}" required :disabled="busy" autocapitalize="off" spellcheck="false" @input="receipt = ''"></label>
      <div class="full"><button :disabled="busy || !pointOptions.length">确认核销</button></div>
    </form>
  </section>
  <template v-else>
    <div class="actions section-tools"><button class="secondary" :disabled="busy" @click="run(loadCommission)">刷新佣金</button></div>
    <CommissionTables :entries="entries" :statements="statements" :busy="busy" />
  </template>
</template>
