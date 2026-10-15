<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type APIError, type MonitorInfo } from './api'
import MonitorList from './components/MonitorList.vue'
import InputPanel from './components/InputPanel.vue'
import InfoPanel from './components/InfoPanel.vue'

const monitors = ref<MonitorInfo[]>([])
const selectedId = ref('')
const loading = ref(false)
const banner = ref<{ kind: 'error' | 'notice'; message: string; detail?: string } | null>(null)

const selected = computed(() => monitors.value.find((m) => m.id === selectedId.value) ?? null)

function showError(e: unknown) {
  const err = e as APIError
  banner.value = { kind: 'error', message: err.message, detail: err.detail }
}

async function refresh() {
  loading.value = true
  banner.value = null
  try {
    monitors.value = await api.refresh()
    if (!monitors.value.some((m) => m.id === selectedId.value)) {
      selectedId.value = monitors.value[0]?.id ?? ''
    }
    if (monitors.value.length === 0) {
      banner.value = {
        kind: 'notice',
        message: '未检测到支持 DDC/CI 的显示器，请确认显示器已在 OSD 中开启 DDC/CI 并直连显卡接口。',
      }
    }
  } catch (e) {
    showError(e)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="layout">
    <div v-if="banner" :class="['banner', banner.kind]">
      <div>{{ banner.message }}</div>
      <details v-if="banner.detail"><summary>详情</summary>{{ banner.detail }}</details>
    </div>

    <MonitorList
      :monitors="monitors"
      :selected-id="selectedId"
      :loading="loading"
      @select="selectedId = $event"
      @refresh="refresh"
    />

    <div class="panel" style="overflow: hidden; display: flex; flex-direction: column">
      <template v-if="selected">
        <InputPanel
          :key="'in-' + selected.id"
          :monitor-id="selected.id"
          @error="showError"
        />
      </template>
      <div v-else class="empty">请先选择一台显示器</div>
    </div>

    <div class="panel" style="grid-column: 2">
      <InfoPanel
        v-if="selected"
        :key="'info-' + selected.id"
        :monitor="selected"
        :monitor-id="selected.id"
        @error="showError"
      />
      <div v-else class="empty">选中显示器后，这里将显示型号、序列号与 VCP 状态</div>
    </div>
  </div>
</template>
