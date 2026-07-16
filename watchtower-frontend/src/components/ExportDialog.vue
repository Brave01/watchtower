<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal" style="max-width:420px">
      <div class="modal-header">
        <h3 class="modal-title">{{ title }}</h3>
        <button class="modal-close" @click="$emit('close')">&times;</button>
      </div>
      <div class="modal-body">
        <div v-if="error" class="login-error" style="margin-bottom:12px">{{ error }}</div>
        <div class="form-group">
          <label class="form-label">文件名</label>
          <input
            class="form-input"
            v-model="name"
            :placeholder="defaultName"
            ref="inputRef"
            @keyup.enter="confirm"
            @keyup.esc="$emit('close')"
          />
          <p class="form-hint">文件将保存为 <code>{{ (name || defaultName) + todayStr }}.{{ ext }}</code></p>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" @click="$emit('close')">取消</button>
        <button class="btn btn-primary" @click="confirm" :disabled="!name.trim()">确定</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'

const props = defineProps({
  title: { type: String, default: '导出文件' },
  defaultName: { type: String, required: true },
  ext: { type: String, required: true },
})

const emit = defineEmits(['close', 'export'])

const name = ref('')
const error = ref('')
const inputRef = ref(null)

const todayStr = computed(() => {
  const now = new Date()
  const yyyy = now.getFullYear()
  const MM = String(now.getMonth() + 1).padStart(2, '0')
  const DD = String(now.getDate()).padStart(2, '0')
  return `-${yyyy}-${MM}-${DD}`
})

onMounted(() => {
  nextTick(() => inputRef.value?.focus())
})

function confirm() {
  const fileName = name.value.trim()
  if (!fileName) {
    error.value = '文件名不能为空'
    return
  }
  emit('export', fileName + todayStr.value)
}
</script>
