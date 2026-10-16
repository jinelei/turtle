<script setup lang="ts">
import type { MonitorInfo } from '../api'

defineProps<{
  monitors: MonitorInfo[]
  selectedId: string
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'refresh'): void
}>()
</script>

<template>
  <div class="panel">
    <div class="toolbar">
      <h2 style="margin: 0">显示器</h2>
      <button class="btn" :disabled="loading" @click="emit('refresh')">
        {{ loading ? '刷新中…' : '刷新' }}
      </button>
    </div>

    <div class="monitor-items">
      <div v-if="monitors.length === 0 && !loading" class="empty">
        未检测到支持 DDC/CI 的显示器
      </div>

      <button
        v-for="m in monitors"
        :key="m.id"
        :class="['monitor-item', { active: m.id === selectedId }]"
        @click="emit('select', m.id)"
      >
        {{ m.name }}
        <span class="sub">{{ m.vendor }} · {{ m.model }}</span>
      </button>
    </div>
  </div>
</template>
