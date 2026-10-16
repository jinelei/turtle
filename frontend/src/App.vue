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
    <!-- 三面板网格：左列侧栏跨满两行；右上输入源、右下显示器信息。
         位置全部显式指定，避免数据加载后自动摆放导致布局跳动。 -->
    <div class="content">
      <MonitorList
        class="sidebar"
        :monitors="monitors"
        :selected-id="selectedId"
        :loading="loading"
        @select="selectedId = $event"
        @refresh="refresh"
      />

      <section class="panel panel-input">
        <template v-if="selected">
          <InputPanel
            :key="'in-' + selected.id"
            :monitor-id="selected.id"
            @error="showError"
          />
        </template>
        <div v-else class="empty">请先选择一台显示器</div>
      </section>

      <section class="panel panel-info">
        <InfoPanel
          v-if="selected"
          :key="'info-' + selected.id"
          :monitor="selected"
          :monitor-id="selected.id"
          @error="showError"
        />
        <div v-else class="empty">选中显示器后，这里将显示型号、序列号与 VCP 状态</div>
      </section>
    </div>

    <!-- 提示区固定在窗口底部：flex: none 不参与伸缩，
         .content 占满剩余高度，提示出现/消失不会引起整体重排。 -->
    <div v-if="banner" :class="['banner', banner.kind]">
      <div>{{ banner.message }}</div>
      <details v-if="banner.detail"><summary>详情</summary>{{ banner.detail }}</details>
    </div>
  </div>
</template>
