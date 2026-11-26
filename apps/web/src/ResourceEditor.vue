<script lang="ts">
export interface Field {
  key: string
  label: string
  kind?: 'integer' | 'select' | 'checkbox' | 'datetime'
  minimum?: number
  options?: { label: string; value: string }[]
}
</script>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { date, id, integer } from './api'
import type { Row } from './api'

const props = defineProps<{
  title: string
  rows: Row[]
  fields: Field[]
  busy: boolean
  updateFields?: Field[]
}>()
const emit = defineEmits<{ save: [rowId: string, body: Record<string, unknown>] }>()
const editing = ref<string>()
const form = ref<Record<string, string | boolean>>({})
const fields = computed(() => editing.value && props.updateFields ? props.updateFields : props.fields)

function open(row?: Row) {
  editing.value = row ? id(row.id) : ''
  form.value = Object.fromEntries(fields.value.map(field => {
    const value = row?.[field.key]
    if (field.kind === 'checkbox') return [field.key, typeof value === 'boolean' ? value : true]
    if (field.kind === 'datetime' && typeof value === 'string') {
      const today = new Date(value)
      today.setMinutes(today.getMinutes() - today.getTimezoneOffset())
      return [field.key, today.toISOString().slice(0, 16)]
    }
    return [field.key, String(value ?? (field.options?.length === 1 ? field.options[0].value : undefined) ?? '')]
  }))
}

watch(() => props.rows, () => { editing.value = undefined })

function save() {
  if (editing.value === undefined) return
  const body = Object.fromEntries(fields.value.map(field => {
    const value = form.value[field.key]
    if (field.kind === 'checkbox') return [field.key, value]
    if (field.kind === 'integer' || field.kind === 'select') return [field.key, integer(String(value))]
    if (field.kind === 'datetime') return [field.key, new Date(String(value)).toISOString()]
    return [field.key, String(value).trim()]
  }))
  emit('save', editing.value, body)
}

function display(row: Row, field: Field) {
  const value = row[field.key]
  if (field.kind === 'checkbox') return value ? '启用' : '停用'
  if (field.kind === 'datetime') return date(value)
  if (field.kind === 'select') return field.options?.find(option => option.value === id(value))?.label ?? id(value)
  return id(value)
}
</script>

<template>
  <section class="card">
    <div class="section-heading"><h2>{{ title }}</h2><button class="secondary" type="button" :disabled="busy" @click="open()">新建</button></div>
    <form v-if="editing !== undefined" class="form-grid editor" @submit.prevent="save">
      <label v-for="field in fields" :key="field.key">
        {{ field.label }}
        <select v-if="field.kind === 'select'" v-model="form[field.key]" :aria-label="field.label" required>
          <option value="" disabled>请选择</option>
          <option v-for="option in field.options" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
        <input v-else-if="field.kind === 'checkbox'" v-model="form[field.key]" type="checkbox">
        <input v-else-if="field.kind === 'integer'" v-model="form[field.key]" inputmode="numeric" :pattern="field.minimum === 1 ? '[1-9][0-9]*' : '[0-9]+'" required>
        <input v-else-if="field.kind === 'datetime'" v-model="form[field.key]" type="datetime-local" required>
        <input v-else v-model="form[field.key]" required pattern=".*\S.*" maxlength="240">
      </label>
      <div class="actions"><button :disabled="busy">保存</button><button class="secondary" type="button" :disabled="busy" @click="editing = undefined">取消</button></div>
    </form>
    <p v-if="!rows.length" class="empty">{{ busy ? '加载中…' : '暂无记录' }}</p>
    <div v-else class="table-scroll">
      <table>
        <thead><tr><th scope="col">编号</th><th v-for="field in props.fields" :key="field.key" scope="col">{{ field.label }}</th><th scope="col">操作</th></tr></thead>
        <tbody><tr v-for="row in rows" :key="id(row.id)"><td>{{ row.id }}</td><td v-for="field in props.fields" :key="field.key">{{ display(row, field) }}</td><td><button class="text-button" type="button" :disabled="busy" @click="open(row)">编辑</button></td></tr></tbody>
      </table>
    </div>
  </section>
</template>
