<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, inputName, INPUT_CANDIDATES, type APIError } from '../api'
import type { CapabilitiesView } from '../types'

const props = defineProps<{ monitorId: string }>()
const emit = defineEmits<{ (e: 'error', err: unknown): void }>()

const currentCode = ref<number | null>(null)
const busy = ref(false)
const caps = ref<CapabilitiesView | null>(null)

// capabilities 可用且带可选输入值时只展示该机型实际支持的输入；
// 0xF3 不支持（P27h-20）或未解析出可选值时回退静态候选表。
const candidates = computed(() => {
  const vals = caps.value?.inputValues
  if (vals && vals.length > 0) {
    return vals.map((code) => ({ code, name: inputName(code) }))
  }
  return INPUT_CANDIDATES
})

async function loadCurrent() {
  try {
    const s = await api.getInputSource(props.monitorId)
    currentCode.value = s.code
  } catch (e) {
    emit('error', e)
  }
}

async function loadCapabilities() {
  try {
    caps.value = await api.getCapabilities(props.monitorId)
  } catch {
    // capabilities 加载失败不影响切换主流程
    caps.value = null
  }
}

async function switchTo(code: number, name: string) {
  if (code === currentCode.value) {
    emit('error', { kind: 'already_active', message: '当前已在该输入源', detail: '' } satisfies APIError)
    return
  }
  if (!window.confirm(`画面将立即切换到「${name}」，本机画面会消失。确定切换吗？`)) {
    return
  }
  busy.value = true
  try {
    await api.setInputSource(props.monitorId, code)
    currentCode.value = code
  } catch (e) {
    emit('error', e)
  } finally {
    busy.value = false
    await loadCurrent()
  }
}

onMounted(() => {
  void loadCapabilities()
  void loadCurrent()
})
</script>

<template>
  <div class="section">
    <div class="toolbar">
      <h2 style="margin: 0">输入源切换（VCP 0x60）</h2>
      <button class="btn" :disabled="busy" @click="loadCurrent">重新读取</button>
    </div>

    <div class="input-grid">
      <button
        v-for="c in candidates"
        :key="c.code"
        :class="['input-btn', { current: c.code === currentCode }]"
        :disabled="busy"
        @click="switchTo(c.code, c.name)"
      >
        {{ c.name }}
        <span class="badge">0x{{ c.code.toString(16).toUpperCase().padStart(2, '0') }}</span>
        <span v-if="c.code === currentCode" class="badge">（当前）</span>
      </button>
    </div>

    <p v-if="currentCode !== null" style="font-size: 12px; color: var(--muted); margin-top: 10px">
      当前输入源值：0x{{ currentCode.toString(16).toUpperCase().padStart(2, '0') }}
      （{{ inputName(currentCode) }}）
    </p>
  </div>
</template>
