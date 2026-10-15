<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type CapabilitiesView, type MonitorInfo, type MonitorStatus } from '../api'

const props = defineProps<{ monitor: MonitorInfo; monitorId: string }>()
const emit = defineEmits<{ (e: 'error', err: unknown): void }>()

const status = ref<MonitorStatus | null>(null)
const caps = ref<CapabilitiesView | null>(null)
const showRaw = ref(false)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const [s, c] = await Promise.all([
      api.getStatus(props.monitorId),
      api.getCapabilities(props.monitorId),
    ])
    status.value = s
    caps.value = c
  } catch (e) {
    emit('error', e)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="toolbar">
      <h2 style="margin: 0">显示器信息</h2>
      <button class="btn" :disabled="loading" @click="load">
        {{ loading ? '读取中…' : '刷新' }}
      </button>
    </div>

    <div class="section">
      <table class="info">
        <tr><td>名称</td><td>{{ monitor.name }}</td></tr>
        <tr><td>厂商</td><td>{{ monitor.vendor }}</td></tr>
        <tr><td>型号</td><td>{{ monitor.model }}</td></tr>
        <tr><td>序列号</td><td>{{ monitor.serial }}</td></tr>
        <tr v-if="caps?.model"><td>Cap. 型号</td><td>{{ caps.model }}</td></tr>
        <tr v-if="caps?.protocol"><td>协议</td><td>{{ caps.protocol }}</td></tr>
      </table>
    </div>

    <div class="section">
      <h2>常用 VCP 状态</h2>
      <table class="info">
        <tr>
          <td>当前输入源</td>
          <td>{{ status?.input.name ?? '—' }}</td>
        </tr>
        <tr v-for="f in status?.features ?? []" :key="f.code">
          <td>{{ f.name }}</td>
          <td v-if="f.supported">{{ f.current }}<span v-if="f.max"> / {{ f.max }}</span></td>
          <td v-else style="color: var(--muted)">{{ f.error || '不支持' }}</td>
        </tr>
      </table>
    </div>

    <div class="section">
      <h2>支持的控制点（Capabilities / VCP 0xF3）</h2>
      <div v-if="caps?.unsupported" class="empty" style="padding: 10px 0">
        该显示器不支持 VCP 0xF3（Lenovo P27h-20 实测如此），<br />
        输入源候选使用内置映射表。
      </div>
      <template v-else>
      <div v-if="caps?.partial" class="banner notice" style="margin-bottom: 8px">
        能力信息不完整（字符串可能被截断）
      </div>
      <div v-if="caps && caps.supported.length" class="chips">
        <span v-for="c in caps.supported" :key="c.code" class="chip">
          0x{{ c.code.toString(16).toUpperCase().padStart(2, '0') }} {{ c.name }}
        </span>
      </div>
      <div v-else class="empty">未能解析出控制点列表</div>

      <button
        v-if="caps"
        class="btn"
        style="margin-top: 10px"
        @click="showRaw = !showRaw"
      >
        {{ showRaw ? '隐藏' : '查看' }}原始字符串
      </button>
      <pre v-if="caps && showRaw" class="raw">{{ caps.raw }}</pre>
      </template>
    </div>
  </div>
</template>
