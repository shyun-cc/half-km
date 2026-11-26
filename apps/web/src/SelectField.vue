<script setup lang="ts">
defineProps<{ label: string; options: { label: string; value: string }[]; disabled?: boolean; required?: boolean }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <label>{{ label }}
    <select v-model="model" :aria-label="label" :disabled="disabled" :required="required">
      <option value="">请选择</option>
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  </label>
</template>
