<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BrandLogo from '@/components/shell/BrandLogo.vue'
import AppButton from '@/components/ui/AppButton.vue'
import Icon from '@/components/ui/Icon.vue'
import { authApi } from '@/lib/api'
import { useAuth } from '@/lib/useAuth'
import { authRedirectPath } from '@/lib/useAuth'

const route = useRoute()
const router = useRouter()
const { setUser, user, ready } = useAuth()

const username = ref('')
const password = ref('')
const error = ref('')
const rateLimited = ref(false)
const loading = ref(false)
const fieldError = ref(false)

const redirectTarget = computed(() => authRedirectPath(String(route.query.redirect || '')))
/** Brand-only until session probe finishes — avoids flashing Demo credentials to logged-in users. */
const showLoginForm = computed(() => ready.value && !user.value)

if (user.value) {
  router.replace(redirectTarget.value)
}

async function onSubmit() {
  error.value = ''
  rateLimited.value = false
  fieldError.value = false
  loading.value = true
  try {
    const res = await authApi.login(username.value.trim(), password.value, redirectTarget.value)
    setUser({ username: res.username, expiresAt: res.expires_at })
    await router.replace(res.redirect || redirectTarget.value)
  } catch (e: any) {
    const msg = e?.message || '登录失败'
    if (msg.includes('429') || msg.includes('过于频繁')) {
      rateLimited.value = true
    } else {
      error.value = msg.includes('用户名或密码错误') ? msg : '用户名或密码错误'
      fieldError.value = true
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex min-h-full items-center justify-center bg-base px-4 py-8 [background:radial-gradient(ellipse_80%_60%_at_50%_0%,rgba(123,97,255,.08),transparent),rgb(var(--c-base))]">
    <div class="w-full max-w-[400px] border border-line bg-surface p-7 shadow-card">
      <div class="mb-7 flex flex-col items-center text-center">
        <BrandLogo size="lg" align="center" />
      </div>

      <template v-if="showLoginForm">
        <div class="mb-1 text-[15px] font-semibold text-txt">登录</div>
        <div class="mb-6 text-[12px] text-txt3">静态账号登录管理界面</div>

        <div
          v-if="route.query.redirect"
          class="mb-4 flex items-center gap-1.5 border border-info/25 bg-info/10 px-2.5 py-2 text-xs text-info"
        >
          <Icon name="chevron-right" :size="14" class="rotate-[-45deg]" />
          登录后将回跳至 <code class="font-mono text-[11px] text-txt2">{{ redirectTarget }}</code>
        </div>

        <div v-if="error" class="mb-4 border border-err/30 bg-err/10 px-3 py-2.5 text-[13px] text-err">{{ error }}</div>
        <div v-if="rateLimited" class="mb-4 border border-warn/30 bg-warn/10 px-3 py-2.5 text-[13px] text-warn">
          登录尝试过于频繁，请稍后再试（429 Too Many Requests）
        </div>

        <form @submit.prevent="onSubmit">
          <div class="mb-4">
            <label class="mb-1.5 block text-xs font-medium text-txt2" for="username">用户名</label>
            <input
              id="username"
              v-model="username"
              type="text"
              autocomplete="username"
              placeholder="admin"
              class="w-full border border-line bg-base px-3 py-2 text-sm text-txt outline-none transition focus:border-accent focus:shadow-[0_0_0_2px_rgba(123,97,255,.3)]"
              :class="fieldError ? 'border-err' : ''"
            />
          </div>
          <div class="mb-4">
            <label class="mb-1.5 block text-xs font-medium text-txt2" for="password">密码</label>
            <input
              id="password"
              v-model="password"
              type="password"
              autocomplete="current-password"
              placeholder="••••••••"
              class="w-full border border-line bg-base px-3 py-2 text-sm text-txt outline-none transition focus:border-accent focus:shadow-[0_0_0_2px_rgba(123,97,255,.3)]"
              :class="fieldError ? 'border-err' : ''"
            />
          </div>
          <AppButton type="submit" variant="primary" block :disabled="loading">
            {{ loading ? '登录中…' : '登录' }}
          </AppButton>
        </form>

        <!-- Compact hint: avoid a large footer stealing LCP from brand-logo__name -->
        <p class="mt-6 border-t border-line pt-4 text-center text-[10px] text-txt3">
          Demo <kbd class="border border-line bg-elevated px-1 font-mono text-[10px]">admin</kbd>
          /
          <kbd class="border border-line bg-elevated px-1 font-mono text-[10px]">demo1234</kbd>
        </p>
      </template>
    </div>
  </div>
</template>
